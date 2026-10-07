package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sort"
	"sync"
	"time"
)

// ShareOperationStore 持久化已确认的清理操作，支持本地测试替换。
type ShareOperationStore interface {
	Enqueue(context.Context, domain.ShareOperation) (domain.ShareOperation, error)
	Recoverable(context.Context) ([]domain.ShareOperation, error)
	Get(context.Context, string) (domain.ShareOperation, error)
	Start(context.Context, string) (bool, error)
	Cancel(context.Context, string) error
	Finish(context.Context, string, string, string, map[string]interface{}) error
	Published(context.Context, string) error
}

type shareOperationQueue struct {
	store  ShareOperationStore
	submit sync.Mutex // 只串行化清理请求的持久化顺序，正常业务不使用。
	mu     sync.Mutex
	active map[string]bool
	s      *ShareRecordService
}

// SetOperationStore 注入持久化队列，须在恢复队列及开放路由前调用。
func (s *ShareRecordService) SetOperationStore(store ShareOperationStore) {
	s.operations = &shareOperationQueue{store: store, active: map[string]bool{}, s: s}
}

// Coordinator 返回分享服务共用的资源协调器。
func (s *ShareRecordService) Coordinator() *ShareOperationCoordinator {
	s.coordOnce.Do(func() {
		if s.coordinator == nil {
			s.coordinator = NewShareOperationCoordinator()
		}
	})
	return s.coordinator
}

func operationResources(v domain.ShareOperation) []shareResource {
	resources := []shareResource{}
	for _, id := range v.ShareIDs {
		resources = append(resources, shareResource{key: shareKey(id), exclusive: v.Kind != "share_media_delete"})
	}
	if v.FileID > 0 {
		resources = append(resources, shareResource{key: fileKey(v.FileID), exclusive: true})
		for _, id := range v.ShareIDs {
			resources = append(resources, shareResource{key: fmt.Sprintf("sync-commit:%d", id)})
		}
	}
	return resources
}

// StartDelete 将分享删除持久化并排队，返回可追踪的任务ID。
func (s *ShareRecordService) StartDelete(ctx context.Context, id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("分享ID无效")
	}
	return s.enqueueOperation(ctx, "share_delete", []int{id}, 0)
}

// StartDeleteMedia 校验文件属于该分享后，将单文件删除持久化并排队。
func (s *ShareRecordService) StartDeleteMedia(ctx context.Context, shareID, id int) (string, error) {
	if id <= 0 || shareID <= 0 {
		return "", fmt.Errorf("分享或文件ID无效")
	}
	parent, err := s.dao.FileShareID(ctx, id)
	if err != nil {
		return "", err
	}
	if parent != shareID {
		return "", fmt.Errorf("文件不属于该分享")
	}
	return s.enqueueOperation(ctx, "share_media_delete", []int{shareID}, id)
}

// StartClearMedia 将选中的分享清空操作持久化，批量目标在同一事务内执行。
func (s *ShareRecordService) StartClearMedia(ctx context.Context, ids []int) (string, error) {
	return s.enqueueOperation(ctx, "share_clear", ids, 0)
}

// StartClearAllMedia 固定提交时的分享集合，新增分享不受旧操作影响。
func (s *ShareRecordService) StartClearAllMedia(ctx context.Context) (string, error) {
	ids, err := s.dao.ShareIDs(ctx)
	if err != nil {
		return "", err
	}
	return s.enqueueOperation(ctx, "share_clear", ids, 0)
}

func (s *ShareRecordService) enqueueOperation(ctx context.Context, kind string, ids []int, fileID int) (string, error) {
	if s.operations == nil || s.tasks == nil {
		return "", fmt.Errorf("分享操作队列未初始化")
	}
	ids = append([]int{}, ids...)
	sort.Ints(ids)
	unique := ids[:0]
	for _, id := range ids {
		if id <= 0 {
			return "", fmt.Errorf("分享ID无效")
		}
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	ids = unique
	if len(ids) > 0 {
		for _, id := range ids {
			if _, err := s.dao.ShareDeleteURL(ctx, id); err != nil {
				return "", err
			}
		}
	}
	v := domain.ShareOperation{TaskID: kind + "_" + uuid.NewString(), Kind: kind, Key: fmt.Sprintf("%s:%v:%d", kind, ids, fileID), ShareIDs: ids, FileID: fileID}
	q := s.operations
	q.submit.Lock()
	defer q.submit.Unlock()
	v, err := q.store.Enqueue(ctx, v)
	if err != nil {
		return "", err
	}
	if err = q.launch(v); err != nil { // 操作已经持久化，后台及启动恢复均以原ID继续同步。
		logger.Errorf("ShareOperation[enqueue] task=%s 任务中心同步失败: %v", v.TaskID, err)
	}
	return v.TaskID, nil
}

// RecoverOperations 先恢复所有目标等待标记，再启动执行协程，必须早于业务路由开放。
func (s *ShareRecordService) RecoverOperations(ctx context.Context) error {
	if s.operations == nil {
		return fmt.Errorf("分享操作队列未初始化")
	}
	q := s.operations
	rows, err := q.store.Recoverable(ctx)
	if err != nil {
		return err
	}
	tickets := map[string]*shareTicket{}
	for _, v := range rows {
		if v.Status == "pending" || v.Status == "running" {
			tickets[v.TaskID] = s.Coordinator().reserveCleanup(v.TaskID, operationResources(v))
		}
	}
	for _, v := range rows {
		if t := tickets[v.TaskID]; t != nil {
			q.mu.Lock()
			q.active[v.TaskID] = true
			q.mu.Unlock()
			q.s.tasks.RegisterCancelGuard(v.TaskID, func() error { return q.store.Cancel(context.Background(), v.TaskID) })
			go q.run(v, t)
		} else {
			// 终态只需补发展示，不应因Redis暂不可用阻止业务入口开放。
			q.mu.Lock()
			q.active[v.TaskID] = true
			q.mu.Unlock()
			go func(v domain.ShareOperation) {
				defer func() { q.mu.Lock(); delete(q.active, v.TaskID); q.mu.Unlock() }()
				_ = q.retry(context.Background(), func() error { return q.publish(v) })
			}(v)
		}
	}
	return nil
}

func (q *shareOperationQueue) launch(v domain.ShareOperation) error {
	q.mu.Lock()
	if q.active[v.TaskID] {
		q.mu.Unlock()
		return nil
	}
	q.active[v.TaskID] = true
	q.mu.Unlock()
	t := q.s.Coordinator().reserveCleanup(v.TaskID, operationResources(v))
	q.s.tasks.RegisterCancelGuard(v.TaskID, func() error { return q.store.Cancel(context.Background(), v.TaskID) })
	err := q.publish(v)
	go q.run(v, t)
	return err
}

func operationTitle(kind string) string {
	switch kind {
	case "share_delete":
		return "删除分享"
	case "share_media_delete":
		return "删除分享文件记录"
	default:
		return "清空分享文件记录"
	}
}

func (q *shareOperationQueue) publish(v domain.ShareOperation) error {
	tasks := q.s.tasks
	if old, err := tasks.Get(v.TaskID); err != nil || old == nil || old["task_type"] != v.Kind || old["task_id"] != v.TaskID {
		if err = tasks.Create(v.TaskID, v.Kind, operationTitle(v.Kind)); err != nil {
			return err
		}
	}
	meta := map[string]interface{}{"operation": v.Kind, "record_ids": v.ShareIDs, "file_id": v.FileID, "phase": v.Phase, "result": v.Result, "cancellable": v.Status == "pending", "recoverable_operation": true}
	if v.Status != "pending" {
		meta["blocking_task_ids"] = []string{}
	}
	if err := tasks.UpdateMetadata(v.TaskID, meta); err != nil {
		return err
	}
	if v.Status == "failed" {
		if err := tasks.SetError(v.TaskID, v.Error); err != nil {
			return err
		}
	} else {
		status := v.Status
		if status == "" {
			status = "pending"
		}
		if err := tasks.UpdateStatus(v.TaskID, status); err != nil {
			return err
		}
	}
	if v.Status == "completed" {
		if err := tasks.UpdateProgressPercent(v.TaskID, 100); err != nil {
			return err
		}
	}
	if v.Status != "pending" && v.Status != "running" {
		return q.store.Published(context.Background(), v.TaskID)
	}
	return nil
}

func (q *shareOperationQueue) run(v domain.ShareOperation, t *shareTicket) {
	taskID := v.TaskID
	coord := q.s.Coordinator()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		q.mu.Lock()
		delete(q.active, v.TaskID)
		q.mu.Unlock()
		q.s.tasks.RemoveCancel(v.TaskID)
		q.s.tasks.RemoveCancelGuard(v.TaskID)
		coord.mu.Lock()
		coord.remove(t)
		coord.signal()
		coord.mu.Unlock()
	}()
	q.s.tasks.RegisterCancel(v.TaskID, cancel)
	q.s.tasks.RegisterCancelGuard(taskID, func() error { return q.store.Cancel(context.Background(), taskID) })
	// 数据库状态读取失败时继续等待，不能凭提交时的旧状态执行清理。
	if err := q.retry(ctx, func() error {
		stored, e := q.store.Get(ctx, v.TaskID)
		if e == nil {
			v = stored
		}
		return e
	}); err != nil {
		q.publishCancelled(v, t)
		return
	}
	if v.Status != "pending" && v.Status != "running" {
		coord.mu.Lock()
		coord.remove(t)
		coord.signal()
		coord.mu.Unlock()
		_ = q.retry(context.Background(), func() error { return q.publish(v) })
		return
	}
	if err := q.retry(ctx, func() error { return q.publish(v) }); err != nil {
		q.publishCancelled(v, t)
		return
	}
	release, err := coord.waitReported(ctx, t, nil, func() {
		_ = q.s.tasks.UpdateMetadata(v.TaskID, map[string]interface{}{"blocking_task_ids": coord.blockers(t), "phase": "等待目标资源"})
	})
	if err != nil {
		q.publishCancelled(v, t)
		return
	}
	defer release()
	var started bool
	err = q.retry(context.Background(), func() error { var e error; started, e = q.store.Start(context.Background(), v.TaskID); return e })
	if !started {
		release()
		q.publishCancelled(v, t)
		return
	}
	v.Status = "running"
	v.Phase = "执行清理"
	_ = q.publish(v)
	var deletedFile *domain.ShareMedia
	completedEffect := func() {
		if v.Kind == "share_media_delete" {
			if deletedFile != nil {
				coord.removedFile(*deletedFile)
			}
		} else {
			coord.invalidate(v.ShareIDs)
		}
	}
	execCtx := dao.WithShareOperationTask(context.WithValue(context.Background(), shareOwnerContext{}, v.TaskID), v.TaskID)
	defer func() {
		if p := recover(); p != nil {
			q.fail(v, fmt.Errorf("清理异常: %v", p), release, completedEffect)
		}
	}()
	var count int64
	switch v.Kind {
	case "share_delete":
		err = q.s.deleteShare(execCtx, v.ShareIDs[0])
		count = 1
	case "share_media_delete":
		var file domain.ShareMedia
		file, err = q.s.dao.FileIdentity(execCtx, v.FileID)
		if err == nil {
			deletedFile = &file
			err = q.s.dao.DeleteMedia(execCtx, v.FileID)
		}
		count = 1
	case "share_clear":
		count, err = q.s.dao.ClearSelectedMedia(execCtx, v.ShareIDs)
	default:
		err = fmt.Errorf("不支持的分享操作: %s", v.Kind)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		count = 0
	}
	if err != nil {
		q.fail(v, err, release, completedEffect)
		return
	}
	stored := q.finish(v, "completed", "", map[string]interface{}{"deleted": count})
	if stored.Status == "completed" {
		completedEffect()
	}
	release()
	_ = q.retry(context.Background(), func() error { return q.publish(stored) })
}

func (q *shareOperationQueue) retry(ctx context.Context, action func() error) error {
	attempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := action(); err == nil {
			return nil
		} else if attempts%30 == 0 {
			logger.Warnf("ShareOperation 状态读取或同步失败，将继续重试: %v", err)
		}
		attempts++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
func (q *shareOperationQueue) finish(v domain.ShareOperation, status, message string, result map[string]interface{}) domain.ShareOperation {
	_ = q.retry(context.Background(), func() error { return q.store.Finish(context.Background(), v.TaskID, status, message, result) })
	_ = q.retry(context.Background(), func() error {
		stored, err := q.store.Get(context.Background(), v.TaskID)
		if err == nil {
			v = stored
		}
		return err
	})
	return v
}
func (q *shareOperationQueue) publishCancelled(v domain.ShareOperation, t *shareTicket) {
	c := q.s.Coordinator()
	c.mu.Lock()
	c.remove(t)
	c.signal()
	c.mu.Unlock()
	_ = q.retry(context.Background(), func() error {
		stored, err := q.store.Get(context.Background(), v.TaskID)
		if err == nil {
			v = stored
		}
		return err
	})
	_ = q.retry(context.Background(), func() error { return q.publish(v) })
}
func (q *shareOperationQueue) fail(v domain.ShareOperation, err error, release func(), completedEffect func()) {
	logger.Errorf("ShareOperation[fail] task=%s error=%v", v.TaskID, err)
	stored := q.finish(v, "failed", err.Error(), nil)
	if stored.Status == "completed" {
		completedEffect()
	}
	release()
	_ = q.retry(context.Background(), func() error { return q.publish(stored) })
}
