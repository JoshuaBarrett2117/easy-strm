package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transientOperationStore struct {
	*operationMemory
	readErrors, finishErrors, publishErrors atomic.Int32
}

type queuedSyncParser struct{ started, resume chan struct{} }

func (p *queuedSyncParser) ParseShareLink(ctx context.Context, _, _ string) (*domain.ParseShareResponse, error) {
	close(p.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.resume:
	}
	return &domain.ParseShareResponse{Files: []domain.ShareFileInfo{{Name: "removed.mkv", Path: "removed.mkv", Fid: "remote", Type: "video"}}}, nil
}

func TestQueuedFileDeleteDuringScanPreventsOldSyncRecreation(t *testing.T) {
	s, m, _ := operationFixture(t)
	p := &queuedSyncParser{started: make(chan struct{}), resume: make(chan struct{})}
	s.parser = p
	expectSyncRecord(m)
	syncID, err := s.StartRecordSync(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("同步没有开始")
	}
	m.ExpectQuery("SELECT share_id FROM t_share_media_file").WithArgs(11).WillReturnRows(sqlmock.NewRows([]string{"share_id"}).AddRow(7))
	expectOperationTarget(m, 7)
	m.ExpectQuery("SELECT share_id,file_name,file_id FROM t_share_media_file").WithArgs(11).WillReturnRows(sqlmock.NewRows([]string{"share_id", "file_name", "file_id"}).AddRow(7, "removed.mkv", "remote"))
	m.ExpectBegin()
	m.ExpectExec("DELETE FROM t_share_media_file WHERE id").WithArgs(11).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectExec("UPDATE t_share_operation_queue SET status='completed'").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	deleteID, err := s.StartDeleteMedia(context.Background(), 7, 11)
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, deleteID, "completed")
	// 删除无需等待当前扫描；旧扫描落库时过滤候选，不再次INSERT。
	m.ExpectQuery("SELECT id FROM t_share_media_file WHERE share_id").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	m.ExpectBegin()
	m.ExpectQuery("SELECT id FROM t_share_record").WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	m.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectCommit()
	close(p.resume)
	waitOperationStatus(t, s, syncID, "completed")
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func (m *transientOperationStore) Get(ctx context.Context, id string) (domain.ShareOperation, error) {
	if m.readErrors.Swap(0) > 0 {
		return domain.ShareOperation{}, errors.New("temporary read failure")
	}
	return m.operationMemory.Get(ctx, id)
}
func (m *transientOperationStore) Finish(ctx context.Context, id, status, message string, result map[string]interface{}) error {
	if m.finishErrors.Swap(0) > 0 {
		return errors.New("temporary finish failure")
	}
	return m.operationMemory.Finish(ctx, id, status, message, result)
}
func (m *transientOperationStore) Published(ctx context.Context, id string) error {
	if m.publishErrors.Swap(0) > 0 {
		return errors.New("temporary publication failure")
	}
	return m.operationMemory.Published(ctx, id)
}

func TestShareOperationRetriesDurableStateWithoutRepeatingDeletion(t *testing.T) {
	s, m, memory := operationFixture(t)
	store := &transientOperationStore{operationMemory: memory}
	store.readErrors.Store(1)
	store.finishErrors.Store(1)
	store.publishErrors.Store(1)
	s.SetOperationStore(store)
	expectOperationTarget(m, 1)
	expectOperationClear(m, 2)
	id, err := s.StartClearMedia(context.Background(), []int{1})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "completed")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := s.Coordinator().acquire(ctx, nil, shareResource{key: shareKey(1), exclusive: true})
	if err != nil {
		t.Fatal("结果展示重试仍占用目标", err)
	}
	release()
	deadline := time.Now().Add(2 * time.Second)
	published := false
	for time.Now().Before(deadline) {
		memory.mu.Lock()
		published = memory.published[id]
		memory.mu.Unlock()
		if published {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !published {
		t.Fatal("同一进程没有补发完成结果")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareOperationRestoresRunningDeleteAfterDiskRemoval(t *testing.T) {
	s, m, store := operationFixture(t)
	id := "same-task-after-disk-removal"
	store.rows[id] = domain.ShareOperation{TaskID: id, Kind: "share_delete", ShareIDs: []int{7}, Status: "running", Phase: "执行清理"}
	p := filepath.Join(t.TempDir(), "already-removed.strm")
	m.ExpectQuery("SELECT url FROM t_share_record").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/abc"))
	m.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	m.ExpectQuery("SELECT id FROM t_share_strm").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("entry-1"))
	m.ExpectQuery("SELECT output_path").WillReturnRows(sqlmock.NewRows([]string{"path", "linked"}).AddRow(p, true))
	expectShareDeletePathLock(m, p)
	m.ExpectExec("INSERT INTO t_strm_export_history").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectBegin()
	for _, table := range []string{"t_strm_file", "t_strm_export_history", "t_strm_export_state", "t_share_strm", "t_share_record", "t_share_media"} {
		m.ExpectExec("DELETE FROM " + table).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	m.ExpectExec("UPDATE t_share_operation_queue SET status='completed'").WithArgs(id, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	m.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := s.RecoverOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "completed")
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareOperationCancelStartRaceHasOneWinner(t *testing.T) {
	for i := 0; i < 100; i++ {
		store := newOperationMemory()
		v, _ := store.Enqueue(context.Background(), domain.ShareOperation{TaskID: "race", Key: "race"})
		var wg sync.WaitGroup
		wg.Add(2)
		var started bool
		var cancelled error
		go func() { defer wg.Done(); started, _ = store.Start(context.Background(), v.TaskID) }()
		go func() { defer wg.Done(); cancelled = store.Cancel(context.Background(), v.TaskID) }()
		wg.Wait()
		if started == (cancelled == nil) {
			t.Fatal("开始与取消不能同时成功或同时失败", started, cancelled)
		}
	}
}

func TestShareCleanupCannotResumeThroughGenericTaskAPI(t *testing.T) {
	s, _, _ := operationFixture(t)
	if err := s.tasks.Create("failed-cleanup", "share_clear", "清空"); err != nil {
		t.Fatal(err)
	}
	if err := s.tasks.SetError("failed-cleanup", "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.tasks.Resume("failed-cleanup"); err == nil {
		t.Fatal("通用恢复产生了没有队列协程的活动任务")
	}
	v, _ := s.tasks.Get("failed-cleanup")
	if v["status"] != "failed" {
		t.Fatal(v)
	}
}

func TestShareOperationRecoveryDoesNotFailOnTransientResultPublication(t *testing.T) {
	s, m, memory := operationFixture(t)
	memory.rows["unpublished"] = domain.ShareOperation{TaskID: "unpublished", Kind: "share_clear", ShareIDs: []int{1}, Status: "completed", Result: map[string]interface{}{"deleted": 2}}
	store := &transientOperationStore{operationMemory: memory}
	store.publishErrors.Store(1)
	s.SetOperationStore(store)
	if err := s.RecoverOperations(context.Background()); err != nil {
		t.Fatal("任务展示暂不可用阻止了启动", err)
	}
	if s.Coordinator().pendingShare(1) {
		t.Fatal("已完成任务恢复了资源等待标记")
	}
	waitOperationStatus(t, s, "unpublished", "completed")
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type operationMemory struct {
	mu        sync.Mutex
	rows      map[string]domain.ShareOperation
	next      int64
	published map[string]bool
	start     chan struct{}
	finish    chan struct{}
}

func newOperationMemory() *operationMemory {
	return &operationMemory{rows: map[string]domain.ShareOperation{}, published: map[string]bool{}}
}
func (m *operationMemory) Enqueue(_ context.Context, v domain.ShareOperation) (domain.ShareOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, old := range m.rows {
		if old.Key == v.Key && (old.Status == "pending" || old.Status == "running") {
			return old, nil
		}
	}
	m.next++
	v.Sequence = m.next
	v.Status = "pending"
	v.Phase = "等待目标资源"
	v.Result = map[string]interface{}{}
	m.rows[v.TaskID] = v
	return v, nil
}
func (m *operationMemory) Recoverable(context.Context) ([]domain.ShareOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.ShareOperation{}
	for _, v := range m.rows {
		if v.Status == "pending" || v.Status == "running" || !m.published[v.TaskID] {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *operationMemory) Get(_ context.Context, id string) (domain.ShareOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.rows[id]
	if !ok {
		return v, sql.ErrNoRows
	}
	return v, nil
}
func (m *operationMemory) Start(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.rows[id]
	if v.Status != "pending" && v.Status != "running" {
		return false, nil
	}
	v.Status = "running"
	m.rows[id] = v
	if m.start != nil {
		close(m.start)
		m.start = nil
	}
	return true, nil
}
func (m *operationMemory) Cancel(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.rows[id]
	if v.Status != "pending" {
		return fmt.Errorf("已执行，不能取消")
	}
	v.Status = "cancelled"
	m.rows[id] = v
	return nil
}
func (m *operationMemory) Finish(_ context.Context, id, status, message string, result map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.rows[id]
	if v.Status == "pending" || v.Status == "running" {
		v.Status = status
		v.Error = message
		v.Result = result
		m.rows[id] = v
	}
	if m.finish != nil {
		close(m.finish)
		m.finish = nil
	}
	return nil
}
func (m *operationMemory) Published(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published[id] = true
	return nil
}

func operationFixture(t *testing.T) (*ShareRecordService, sqlmock.Sqlmock, *operationMemory) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, NewTaskService(dao.NewTaskRedisDAO(client)), nil)
	store := newOperationMemory()
	s.SetOperationStore(store)
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			s.operations.mu.Lock()
			n := len(s.operations.active)
			s.operations.mu.Unlock()
			if n == 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		client.Close()
		db.Close()
	})
	return s, mock, store
}
func expectOperationTarget(mock sqlmock.Sqlmock, id int) {
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/example"))
}
func expectOperationClear(mock sqlmock.Sqlmock, count int64) {
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_media_file WHERE share_id = ANY").WillReturnResult(sqlmock.NewResult(0, count))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE t_share_operation_queue SET status='completed'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
func waitOperationStatus(t *testing.T, s *ShareRecordService, id, status string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		task, e := s.tasks.Get(id)
		if e == nil && task != nil && task["status"] == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	task, _ := s.tasks.Get(id)
	t.Fatalf("task %s 未进入 %s: %+v", id, status, task)
}

func TestShareOperationQueueWaitsOnlyForTargetAndDeduplicates(t *testing.T) {
	s, mock, store := operationFixture(t)
	active := acquireFixture(t, s.Coordinator(), shareResource{key: shareKey(1)}, shareResource{key: fileKey(11), exclusive: true})
	defer active()
	expectOperationTarget(mock, 1)
	id, e := s.StartClearMedia(context.Background(), []int{1})
	if e != nil {
		t.Fatal(e)
	}
	expectOperationTarget(mock, 1)
	same, e := s.StartClearMedia(context.Background(), []int{1, 1})
	if e != nil || same != id {
		t.Fatal(same, e)
	}
	expectOperationTarget(mock, 2)
	expectOperationClear(mock, 4)
	other, e := s.StartClearMedia(context.Background(), []int{2})
	if e != nil {
		t.Fatal(e)
	}
	waitOperationStatus(t, s, other, "completed")
	v, _ := store.Get(context.Background(), id)
	if v.Status != "pending" {
		t.Fatal("目标未结束时执行了清空", v.Status)
	}
	expectOperationClear(mock, 3)
	active()
	waitOperationStatus(t, s, id, "completed")
	if e = mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestShareOperationQueueCancelDoesNotDeleteAndUnblocksWork(t *testing.T) {
	s, mock, store := operationFixture(t)
	active := acquireFixture(t, s.Coordinator(), shareResource{key: shareKey(1)})
	defer active()
	expectOperationTarget(mock, 1)
	id, e := s.StartClearMedia(context.Background(), []int{1})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.tasks.Cancel(id); e != nil {
		t.Fatal(e)
	}
	active()
	waitOperationStatus(t, s, id, "cancelled")
	deadline := time.Now().Add(time.Second)
	for s.Coordinator().pendingShare(1) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.Coordinator().pendingShare(1) {
		t.Fatal("取消后未撤销等待标记")
	}
	v, _ := store.Get(context.Background(), id)
	if v.Status != "cancelled" {
		t.Fatal(v)
	}
	if e = mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestShareOperationQueueExecutingCannotCancel(t *testing.T) {
	s, mock, _ := operationFixture(t)
	expectOperationTarget(mock, 1)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_media_file").WillDelayFor(150 * time.Millisecond).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE t_share_operation_queue SET status='completed'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	id, e := s.StartClearMedia(context.Background(), []int{1})
	if e != nil {
		t.Fatal(e)
	}
	waitOperationStatus(t, s, id, "running")
	if e = s.tasks.Cancel(id); e == nil {
		t.Fatal("执行阶段不允许取消")
	}
	waitOperationStatus(t, s, id, "completed")
	if e = mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestShareOperationQueueFailureReleasesReservation(t *testing.T) {
	s, mock, _ := operationFixture(t)
	expectOperationTarget(mock, 1)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_media_file").WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	id, e := s.StartClearMedia(context.Background(), []int{1})
	if e != nil {
		t.Fatal(e)
	}
	waitOperationStatus(t, s, id, "failed")
	deadline := time.Now().Add(time.Second)
	for s.Coordinator().pendingShare(1) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.Coordinator().pendingShare(1) {
		t.Fatal("失败后残留资源")
	}
	if e = mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestShareOperationQueueRecoveryPreservesIDsAndCommittedResult(t *testing.T) {
	s, mock, store := operationFixture(t)
	store.rows["queued-before-restart"] = domain.ShareOperation{TaskID: "queued-before-restart", Kind: "share_clear", ShareIDs: []int{1}, Status: "pending"}
	store.rows["committed-before-restart"] = domain.ShareOperation{TaskID: "committed-before-restart", Kind: "share_clear", ShareIDs: []int{2}, Status: "completed", Result: map[string]interface{}{"deleted": int64(8)}}
	// 已提交的目标2不得再次清空；只有未执行的目标1有数据库变更。
	expectOperationClear(mock, 2)
	if e := s.RecoverOperations(context.Background()); e != nil {
		t.Fatal(e)
	}
	waitOperationStatus(t, s, "queued-before-restart", "completed")
	waitOperationStatus(t, s, "committed-before-restart", "completed")
	task, _ := s.tasks.Get("committed-before-restart")
	meta := task["metadata"].(map[string]interface{})
	result := meta["result"].(map[string]interface{})
	if result["deleted"] != float64(8) {
		t.Fatal(task)
	}
	if e := mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestShareOperationQueueClearAllCapturesSubmissionScope(t *testing.T) {
	s, mock, _ := operationFixture(t)
	mock.ExpectQuery("SELECT id FROM t_share_record ORDER BY id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
	expectOperationTarget(mock, 1)
	expectOperationTarget(mock, 2)
	expectOperationClear(mock, 0)
	id, e := s.StartClearAllMedia(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	waitOperationStatus(t, s, id, "completed")
	if e = mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
