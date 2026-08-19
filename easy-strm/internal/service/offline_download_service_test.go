package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	driver "github.com/SheltonZhu/115driver/pkg/driver"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// fakeOfflineAccountStore 内存115账号桩
type fakeOfflineAccountStore struct {
	accounts map[int]*domain.Cloud115
}

func (f *fakeOfflineAccountStore) GetByID(id int) (*domain.Cloud115, error) {
	if acc, ok := f.accounts[id]; ok {
		return acc, nil
	}
	return nil, nil
}

// fakeOfflineClient 内存115离线下载客户端桩
type fakeOfflineClient struct {
	mu           sync.Mutex
	mkdirDirID   string
	mkdirPath    string
	mkdirErr     error
	addHashes    []string
	addErr       error
	addFunc      func(uris []string) ([]string, error) // 可选：按调用动态返回，覆盖 addHashes/addErr
	addedUrls    []string
	addedBatches [][]string
	addedDir     string
	addCalls     int
	listResp     *driver.OfflineTaskResp
	listErr      error
	listCalled   chan struct{}
	deleteErr    error
	deleted      []string
	deleteFiles  bool
}

func (f *fakeOfflineClient) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mkdirPath = path
	return f.mkdirDirID, f.mkdirErr
}

func (f *fakeOfflineClient) AddOfflineTasks(uris []string, saveDirID string, cloud115ID int, cookie string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls++
	f.addedUrls = append([]string{}, uris...)
	f.addedBatches = append(f.addedBatches, append([]string(nil), uris...))
	f.addedDir = saveDirID
	if f.addFunc != nil {
		return f.addFunc(uris)
	}
	return f.addHashes, f.addErr
}

func (f *fakeOfflineClient) ListOfflineTasks(page int64, cloud115ID int, cookie string) (*driver.OfflineTaskResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listCalled != nil {
		select {
		case <-f.listCalled:
		default:
			close(f.listCalled)
		}
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listResp != nil {
		return f.listResp, nil
	}
	return &driver.OfflineTaskResp{PageCount: 1}, nil
}

func (f *fakeOfflineClient) DeleteOfflineTasks(hashes []string, deleteFiles bool, cloud115ID int, cookie string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, hashes...)
	f.deleteFiles = deleteFiles
	return f.deleteErr
}

// newOfflineDownloadTestEnv 组装服务测试环境（sqlmock记录库 + 内存任务/账号/客户端桩）
func newOfflineDownloadTestEnv(t *testing.T) (*OfflineDownloadService, *fakeOfflineClient, *fakeTaskDAO, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New 失败: %v", err)
	}

	client := &fakeOfflineClient{mkdirDirID: "dir-1"}
	taskStore := newFakeTaskDAO()
	accounts := &fakeOfflineAccountStore{accounts: map[int]*domain.Cloud115{
		1: {ID: 1, Name: "测试账号", Cookie: "UID=1; CID=2; SEID=3"},
	}}
	svc := NewOfflineDownloadService(client, taskStore, dao.NewOfflineDownloadTaskDAO(db), accounts)

	return svc, client, taskStore, mock, func() { _ = db.Close() }
}

func TestNormalizeOfflineUrls(t *testing.T) {
	valid, invalid := normalizeOfflineUrls([]string{
		"ed2k://|file|a.mkv|1|HASH|/",
		"  magnet:?xt=urn:btih:ABCDEF  ",
		"",
		"   ",
		"ed2k://|file|a.mkv|1|HASH|/", // 重复项应去重
		"https://example.com/a.mp4",
		"thunder://xxx",
		"not-a-link",
	})

	if len(valid) != 3 {
		t.Fatalf("有效链接数不符合预期: got %d want 3, valid=%v", len(valid), valid)
	}
	if valid[1] != "magnet:?xt=urn:btih:ABCDEF" {
		t.Fatalf("链接未修剪空白: %q", valid[1])
	}
	if len(invalid) != 2 {
		t.Fatalf("无效链接数不符合预期: got %d want 2, invalid=%v", len(invalid), invalid)
	}
}

func TestOfflineDownloadSubmitHappyPathTracksToCompleted(t *testing.T) {
	svc, client, taskStore, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	client.addHashes = []string{"hash-1"}
	client.listCalled = make(chan struct{})
	client.listResp = &driver.OfflineTaskResp{
		PageCount: 1,
		Tasks: []*driver.OfflineTask{
			{InfoHash: "hash-1", Name: "SSIS-472-C.mkv", Size: 10839656214, Status: 2, Percent: 100},
		},
	}

	recordColumns := []string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}

	// Submit 同步阶段：写入一条 pending 记录
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO t_offline_download_task
		(task_id, cloud115_id, url, info_hash, name, size, status, percent, error_message, save_dir_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`)).
		WithArgs(sqlmock.AnyArg(), 1, "ed2k://|file|SSIS-472-C.mkv|10839656214|HASH|/", "hash-1", "", int64(0), domain.OfflineStatusPending, 0.0, "", "dir-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 跟踪协程：同步账号状态（活跃记录 -> 115已完成 -> 回写）
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE cloud115_id = $1 AND status IN ($2, $3) ORDER BY id ASC`)).
		WithArgs(1, domain.OfflineStatusPending, domain.OfflineStatusDownloading).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(1, "offline-x", 1, "ed2k://|file|SSIS-472-C.mkv|10839656214|HASH|/", "hash-1", "", int64(0), domain.OfflineStatusPending, 0.0, "", "dir-1", "", ""))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_offline_download_task
		SET name = $1, size = $2, status = $3, percent = $4, error_message = $5, update_time = CURRENT_TIMESTAMP
		WHERE cloud115_id = $6 AND info_hash = $7`)).
		WithArgs("SSIS-472-C.mkv", int64(10839656214), domain.OfflineStatusCompleted, 100.0, "", 1, "hash-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE task_id = $1 ORDER BY id ASC`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(1, "offline-x", 1, "ed2k://|file|SSIS-472-C.mkv|10839656214|HASH|/", "hash-1", "SSIS-472-C.mkv", int64(10839656214), domain.OfflineStatusCompleted, 100.0, "", "dir-1", "", ""))

	resp, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 1,
		Urls:       []string{"ed2k://|file|SSIS-472-C.mkv|10839656214|HASH|/"},
	})
	if err != nil {
		t.Fatalf("提交云下载失败: %v", err)
	}
	if resp.Accepted != 1 || resp.Rejected != 0 || !strings.HasPrefix(resp.TaskId, offlineTaskIDPrefix) {
		t.Fatalf("提交结果不符合预期: %+v", resp)
	}
	if client.mkdirPath != offlineDefaultDir || client.addedDir != "dir-1" {
		t.Fatalf("默认目录处理不符合预期: mkdirPath=%q addedDir=%q", client.mkdirPath, client.addedDir)
	}

	// 等待跟踪协程把任务推进到终态
	select {
	case <-client.listCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("跟踪协程未启动状态同步")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, _ := taskStore.Get(resp.TaskId)
		if task != nil && task["status"] == domain.TaskStatusCompleted {
			if task["task_type"] != string(domain.TaskTypeOfflineDownload) {
				t.Fatalf("任务类型不符合预期: %v", task["task_type"])
			}
			if task["success_files"] != 1 || task["total_files"] != 1 {
				t.Fatalf("任务进度不符合预期: %+v", task)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("任务未在预期时间内到达终态: %+v", task)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadSubmitRejectsUnsupportedLinks(t *testing.T) {
	svc, client, _, _, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	_, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 1,
		Urls:       []string{"thunder://xxx", "  "},
	})
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if client.addCalls != 0 {
		t.Fatalf("全部链接非法时不应调用115接口, addCalls=%d", client.addCalls)
	}
}

func TestOfflineDownloadSubmitAccountCookieMissing(t *testing.T) {
	svc, _, _, _, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	_, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 404,
		Urls:       []string{"ed2k://|file|a.mkv|1|HASH|/"},
	})
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("期望账号不存在错误, got: %v", err)
	}
}

func TestOfflineDownloadSubmitAllRejectedBy115(t *testing.T) {
	svc, client, taskStore, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	client.addHashes = []string{""}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO t_offline_download_task
		(task_id, cloud115_id, url, info_hash, name, size, status, percent, error_message, save_dir_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`)).
		WithArgs(sqlmock.AnyArg(), 1, "magnet:?xt=urn:btih:XYZ", "", "", int64(0), domain.OfflineStatusFailed, 0.0, "115未接受该链接（链接无效或任务已存在）", "dir-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	_, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 1,
		Urls:       []string{"magnet:?xt=urn:btih:XYZ"},
	})
	if err == nil || !strings.Contains(err.Error(), "未接受任何链接") {
		t.Fatalf("期望115全部拒绝错误, got: %v", err)
	}
	if len(taskStore.tasks) != 0 {
		t.Fatal("全部拒绝时不应创建任务中心任务")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadSyncMarksRemovedRecords(t *testing.T) {
	svc, client, _, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	// 115离线列表为空（已完整拉取），本地仍有活跃记录 => 标记removed
	client.listResp = &driver.OfflineTaskResp{PageCount: 1}

	recordColumns := []string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE cloud115_id = $1 AND status IN ($2, $3) ORDER BY id ASC`)).
		WithArgs(1, domain.OfflineStatusPending, domain.OfflineStatusDownloading).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(7, "offline-x", 1, "u", "hash-gone", "", int64(0), domain.OfflineStatusPending, 0.0, "", "dir-1", "", ""))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_offline_download_task
		SET status = $1, error_message = $2, update_time = CURRENT_TIMESTAMP
		WHERE id = $3`)).
		WithArgs(domain.OfflineStatusRemoved, "任务已不在115离线列表中", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ran, err := svc.SyncAccountOfflineTasks(context.Background(), 1, "", true)
	if err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	if !ran {
		t.Fatal("force=true 时应实际执行同步")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadListFillsAccountNamesWithoutActiveSync(t *testing.T) {
	svc, client, _, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	// 当前页无未完成任务 => 不触发115状态同步
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM t_offline_download_task WHERE 1=1`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	recordColumns := []string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE 1=1 ORDER BY id DESC LIMIT $1 OFFSET $2`)).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(1, "offline-x", 1, "u", "hash-1", "a.mkv", int64(1), domain.OfflineStatusCompleted, 100.0, "", "dir-1", "", ""))

	resp, err := svc.List(context.Background(), 0, "", 1, 20)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("列表结果不符合预期: %+v", resp)
	}
	if resp.Data[0].AccountName != "测试账号" {
		t.Fatalf("账号名称未回填: %q", resp.Data[0].AccountName)
	}
	if len(client.deleted) != 0 && client.addCalls != 0 {
		t.Fatal("无活跃任务时不应产生115写调用")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadDeleteRecordRemoves115Task(t *testing.T) {
	svc, client, _, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	recordColumns := []string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE id = $1`)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(5, "offline-x", 1, "u", "hash-1", "a.mkv", int64(1), domain.OfflineStatusDownloading, 30.0, "", "dir-1", "", ""))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM t_offline_download_task WHERE id = $1`)).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.DeleteRecord(context.Background(), 5, true); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "hash-1" || !client.deleteFiles {
		t.Fatalf("115侧任务删除不符合预期: deleted=%v deleteFiles=%v", client.deleted, client.deleteFiles)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadDeleteRecordSkipsRemovedTask(t *testing.T) {
	svc, client, _, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	recordColumns := []string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE id = $1`)).
		WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(6, "offline-x", 1, "u", "hash-2", "a.mkv", int64(1), domain.OfflineStatusRemoved, 0.0, "任务已不在115离线列表中", "dir-1", "", ""))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM t_offline_download_task WHERE id = $1`)).
		WithArgs(int64(6)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.DeleteRecord(context.Background(), 6, false); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(client.deleted) != 0 {
		t.Fatalf("removed 状态记录不应再调用115删除: %v", client.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestOfflineDownloadSubmitAssociatesExistingTaskOnDuplicate 验证：批量提交因重复被拒后降级逐条提交，
// 并把重复链接关联到115已有的离线任务（可继续跟踪进度）。
func TestOfflineDownloadSubmitAssociatesExistingTaskOnDuplicate(t *testing.T) {
	svc, client, taskStore, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	dupUrl := "ed2k://|file|SSIS-472-C.mkv|10839656214|HASH|/"

	// 批量与逐条提交都返回“任务已存在”（模拟driver退化后的真实错误形态），迫使服务走关联现有任务分支
	productionDupErr := fmt.Errorf(`{"data":"encrypted-blob","errcode":10008,"error_msg":"任务已存在，请勿输入重复的链接地址","state":false}: %w`, driver.ErrUnexpected)
	client.addFunc = func(uris []string) ([]string, error) {
		return nil, productionDupErr
	}
	client.listCalled = make(chan struct{})
	client.listResp = &driver.OfflineTaskResp{
		PageCount: 1,
		Tasks: []*driver.OfflineTask{
			{InfoHash: "duphash", Url: dupUrl, Name: "SSIS-472-C.mkv", Size: 10839656214, Status: 2, Percent: 100},
		},
	}

	recordColumns := []string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}

	// 关联后写入的记录应为已完成态，并携带现有任务的hash/名称/大小/进度
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO t_offline_download_task
		(task_id, cloud115_id, url, info_hash, name, size, status, percent, error_message, save_dir_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`)).
		WithArgs(sqlmock.AnyArg(), 1, dupUrl, "duphash", "SSIS-472-C.mkv", int64(10839656214), domain.OfflineStatusCompleted, 100.0, "", "dir-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 跟踪协程：关联记录已是completed，不属于活跃记录 => 活跃查询为空
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE cloud115_id = $1 AND status IN ($2, $3) ORDER BY id ASC`)).
		WithArgs(1, domain.OfflineStatusPending, domain.OfflineStatusDownloading).
		WillReturnRows(sqlmock.NewRows(recordColumns))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE task_id = $1 ORDER BY id ASC`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(recordColumns).
			AddRow(1, "offline-x", 1, dupUrl, "duphash", "SSIS-472-C.mkv", int64(10839656214), domain.OfflineStatusCompleted, 100.0, "", "dir-1", "", ""))

	resp, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 1,
		Urls:       []string{dupUrl},
	})
	if err != nil {
		t.Fatalf("重复链接应通过关联现有任务受理, got err: %v", err)
	}
	if resp.Accepted != 1 || resp.Rejected != 0 {
		t.Fatalf("受理数量不符合预期: %+v", resp)
	}
	if len(resp.Results) != 1 || !resp.Results[0].Accepted || resp.Results[0].InfoHash != "duphash" {
		t.Fatalf("逐链接结果不符合预期: %+v", resp.Results)
	}
	if client.addCalls < 2 {
		t.Fatalf("期望先批量后逐条降级提交, addCalls=%d", client.addCalls)
	}

	select {
	case <-client.listCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("跟踪协程未启动状态同步")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, _ := taskStore.Get(resp.TaskId)
		if task != nil && task["status"] == domain.TaskStatusCompleted {
			if task["success_files"] != 1 || task["total_files"] != 1 {
				t.Fatalf("任务进度不符合预期: %+v", task)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("任务未在预期时间内到达终态: %+v", task)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestOfflineDownloadSubmitBatchFatalErrorNormalized 验证：批量提交遇到配额用尽等致命错误时，
// 返回归一化的中文错误且不触发逐条降级提交。
func TestOfflineDownloadSubmitBatchFatalErrorNormalized(t *testing.T) {
	svc, client, taskStore, _, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	client.addFunc = func(uris []string) ([]string, error) {
		return nil, driver.ErrOfflineNoTimes
	}

	_, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 1,
		Urls:       []string{"ed2k://|file|a.mkv|1|HASH|/"},
	})
	if err == nil || !strings.Contains(err.Error(), "次数已用完") {
		t.Fatalf("期望配额用尽的归一化错误, got: %v", err)
	}
	if client.addCalls != 1 {
		t.Fatalf("致命错误不应触发逐条降级提交, addCalls=%d", client.addCalls)
	}
	if len(taskStore.tasks) != 0 {
		t.Fatal("致命错误时不应创建任务中心任务")
	}
}

// TestOfflineDownloadSubmitLargeBatchQueuesAndChunks 验证超过100条时入口立即返回，
// 后台单worker再按100条切分并顺序调用115，而不是把大数组直接发送给115。
func TestOfflineDownloadSubmitLargeBatchQueuesAndChunks(t *testing.T) {
	svc, client, taskStore, mock, cleanup := newOfflineDownloadTestEnv(t)
	defer cleanup()

	urls := make([]string, 201)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/file-%03d.mkv", i)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	client.addFunc = func(uris []string) ([]string, error) {
		startOnce.Do(func() { close(started) })
		<-release
		return make([]string, len(uris)), nil
	}

	insertPrefix := regexp.QuoteMeta(`INSERT INTO t_offline_download_task
		(task_id, cloud115_id, url, info_hash, name, size, status, percent, error_message, save_dir_id)
		VALUES `)
	mock.ExpectExec(insertPrefix).WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(insertPrefix).WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectExec(insertPrefix).WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := svc.Submit(context.Background(), domain.OfflineDownloadSubmitRequest{
		Cloud115ID: 1,
		Urls:       urls,
	})
	if err != nil {
		t.Fatalf("大批量任务入队失败: %v", err)
	}
	if !resp.Queued || resp.QueuedCount != 201 || resp.Accepted != 0 || resp.Total != 201 {
		t.Fatalf("大批量入队响应不符合预期: %+v", resp)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("后台队列未开始发送第一批请求")
	}
	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for {
		task, _ := taskStore.Get(resp.TaskId)
		if task != nil && task["status"] == domain.TaskStatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("后台队列未在预期时间内处理完成: %+v", task)
		}
		time.Sleep(20 * time.Millisecond)
	}

	client.mu.Lock()
	batchSizes := make([]int, len(client.addedBatches))
	for i, batch := range client.addedBatches {
		batchSizes[i] = len(batch)
	}
	client.mu.Unlock()
	if fmt.Sprint(batchSizes) != "[100 100 1]" {
		t.Fatalf("后台分批大小不符合预期: %v", batchSizes)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
