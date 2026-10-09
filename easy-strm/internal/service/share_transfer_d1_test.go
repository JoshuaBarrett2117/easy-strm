package service

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

const d1LogInsertSQL = `INSERT INTO t_share_transfer_log
 (task_id, share_code, share_folder_name, file_name, file_pick_code,
 file_size, file_sha1, cloud115_id, target_directory, status, error_message,
 is_second_transfer) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`

const d1LogSelectSQL = `SELECT id, task_id, share_code, share_folder_name, file_name,
 file_pick_code, file_size, file_sha1, cloud115_id,
 target_directory, status, error_message, is_second_transfer,
 COALESCE(create_time::text, ''), COALESCE(update_time::text, '')
 FROM t_share_transfer_log WHERE task_id = $1 ORDER BY id ASC`

const d1LogUpdateSQL = `UPDATE t_share_transfer_log
 SET status = $1, error_message = $2, update_time = CURRENT_TIMESTAMP
 WHERE task_id = $3 AND file_pick_code = $4`

type d1TransferCall struct {
	code, password, fid, cid string
	account                  int
}

type d1TransferClient struct {
	fakeShareCloud115Client
	calls chan d1TransferCall
}

func (client *d1TransferClient) MkdirAll115(path string, account int, cookie string) (string, error) {
	if path != "/测试目标" || account != 7 {
		return "", errors.New("unexpected target")
	}
	return "target-cid", nil
}

func (client *d1TransferClient) GetCIDByPath(path string, account int, cookie string) (string, error) {
	return client.MkdirAll115(path, account, cookie)
}

func (client *d1TransferClient) ReceiveShare(code, password, fid, cid string, account int, cookie string) error {
	client.calls <- d1TransferCall{code: code, password: password, fid: fid, cid: cid, account: account}
	return nil
}

type d1TaskStore struct {
	*dao.TaskRedisDAO
	done        chan error
	metadataErr error
}

func (store *d1TaskStore) UpdateMetadata(taskID string, metadata map[string]interface{}) error {
	if store.metadataErr != nil {
		return store.metadataErr
	}
	return store.TaskRedisDAO.UpdateMetadata(taskID, metadata)
}

// 终态的最后一次进度保存后发信号，不以 sleep 或轮询猜测异步执行是否结束。
func (store *d1TaskStore) UpdateProgress(taskID string, total, processed, success, failed int) error {
	err := store.TaskRedisDAO.UpdateProgress(taskID, total, processed, success, failed)
	task, readErr := store.TaskRedisDAO.Get(taskID)
	if task != nil && task["status"] == transferStatusCompleted {
		if err == nil {
			err = readErr
		}
		store.done <- err
	}
	return err
}

func newD1TransferTestService(t *testing.T) (*ShareTransferService, *d1TaskStore, *d1TransferClient, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	previousDB := dao.DB
	dao.DB = database
	t.Cleanup(func() { dao.DB = previousDB; database.Close() })
	redisClient := setupTaskRedisMock(t)
	t.Cleanup(func() { redisClient.Close() })
	store := &d1TaskStore{TaskRedisDAO: dao.NewTaskRedisDAO(redisClient), done: make(chan error, 2)}
	client := &d1TransferClient{calls: make(chan d1TransferCall, 2)}
	svc := &ShareTransferService{
		client: client, taskDAO: store, logDAO: dao.NewShareTransferLogDAO(database),
		cloud115DAO: dao.NewCloud115DAO(), redisClient: redisClient,
	}
	return svc, store, client, mock
}

func expectD1TargetAccount(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, name, cookie, COALESCE(cookie_source, ''), refresh_token, access_token, expires_in,
 COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
 COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
 COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
 COALESCE(alist_url, ''), COALESCE(alist_token, ''),
 create_time, update_time FROM t_cloud_115 WHERE id = $1`).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "cookie", "cookie_source", "refresh_token", "access_token", "expires_in",
			"transfer_account_id", "transfer_directory", "account_type", "quota_used", "priority", "status", "cooling_start_time",
			"transfer_method", "alist_url", "alist_token", "create_time", "update_time"}).
			AddRow(7, "测试账号", "unit-test-cookie", "", "", "", 0, 0, "", "resource", 0, 5, "active", nil,
				"", "", "", time.Unix(0, 0), time.Unix(0, 0)))
}

func expectD1RetryLogs(mock sqlmock.Sqlmock, taskID string) {
	mock.ExpectQuery(d1LogSelectSQL).WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "share_code", "share_folder_name", "file_name",
			"file_pick_code", "file_size", "file_sha1", "cloud115_id", "target_directory", "status",
			"error_message", "is_second_transfer", "create_time", "update_time"}).
			AddRow(1, taskID, "share-test", "测试分享", "电影.mkv", "fid-original", 321, "", 7, "/测试目标",
				"failed", "测试失败", false, "mock-created", "mock-updated")).RowsWillBeClosed()
}

func waitD1Transfer(t *testing.T, store *d1TaskStore, client *d1TransferClient, password string) {
	t.Helper()
	select {
	case err := <-store.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("本地转存测试未完成")
	}
	select {
	case call := <-client.calls:
		if call.password != password {
			t.Fatal("转存调用没有保持原分享密码")
		}
		if call.code != "share-test" || call.fid != "fid-original" || call.cid != "target-cid" || call.account != 7 {
			t.Fatal("转存调用的分享、Fid 或目标参数错误")
		}
	default:
		t.Fatal("未调用本地转存钩子")
	}
}

// TestD1SubmitAndRetryTransfer 通过真实提交/重试链路验证日志与密码；失败列表由本地夹具注入，避免自动重试的等待。
func TestD1SubmitAndRetryTransfer(t *testing.T) {
	svc, store, client, mock := newD1TransferTestService(t)
	const password = "unit-test-password-D1"
	cache, err := json.Marshal(domain.ParseShareResponse{FolderName: "测试分享"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.redisClient.Set(context.Background(), shareCacheKeyPrefix+"share-test", cache, 0).Err(); err != nil {
		t.Fatal(err)
	}
	expectD1TargetAccount(mock)
	mock.ExpectExec(d1LogInsertSQL).WithArgs(sqlmock.AnyArg(), "share-test", "测试分享", "电影.mkv",
		"fid-original", int64(321), "", 7, "/测试目标", "pending", "", false).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(d1LogUpdateSQL).WithArgs("success", "", sqlmock.AnyArg(), "fid-original").
		WillReturnResult(sqlmock.NewResult(0, 1))
	response, err := svc.SubmitTransfer(context.Background(), domain.TransferRequest{
		ShareCode: "share-test", Password: password, TargetCloud115Id: 7, TargetDirectory: "/测试目标",
		ConflictStrategy: "skip", Files: []domain.ShareTransferFileItem{{Fid: "fid-original", PickCode: "different-pickcode", Name: "电影.mkv", Size: 321}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalFiles != 1 || response.EstimatedSize != 321 {
		t.Fatal("提交统计错误")
	}
	waitD1Transfer(t, store, client, password)
	progress, err := svc.GetProgress(context.Background(), response.TaskId)
	if err != nil {
		t.Fatal(err)
	}
	progressJSON, err := json.Marshal(progress)
	if err != nil || strings.Contains(string(progressJSON), password) || strings.Contains(string(progressJSON), "share_transfer_password") {
		t.Fatal("转存进度响应不能暴露分享密码")
	}
	task, err := store.Get(response.TaskId)
	if err != nil {
		t.Fatal(err)
	}
	metadata := task["metadata"].(map[string]interface{})
	if metadata["share_transfer_password"] != password {
		t.Fatal("原分享密码未保存到任务元数据")
	}
	metadata["failed_items"] = []domain.FailedItem{{Name: "电影.mkv", Retryable: true}, {Name: "不可重试.mkv", Retryable: false}}
	if err := store.UpdateMetadata(response.TaskId, metadata); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateStatus(response.TaskId, transferStatusFailed); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCancelFlag(response.TaskId); err != nil {
		t.Fatal(err)
	}
	expectD1RetryLogs(mock, response.TaskId)
	expectD1TargetAccount(mock)
	mock.ExpectExec(d1LogUpdateSQL).WithArgs("success", "", response.TaskId, "fid-original").
		WillReturnResult(sqlmock.NewResult(0, 1))
	retry, err := svc.RetryTransfer(context.Background(), response.TaskId)
	if err != nil || retry.RetriedFiles != 1 || retry.TaskId != response.TaskId {
		t.Fatalf("重试未复用原任务: %v", err)
	}
	waitD1Transfer(t, store, client, password)
	if store.IsCancelled(response.TaskId) {
		t.Fatal("重试未清除取消标记")
	}
	retriedTask, err := store.Get(response.TaskId)
	if err != nil || retriedTask["metadata"].(map[string]interface{})["share_transfer_password"] != password {
		t.Fatal("执行与重试更新元数据时不能丢失原密码")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestD1RetryDoesNotInsertLogs 配合实际重试的 UPDATE 期望，防止被忽略的 sqlmock INSERT 错误掩盖重复写日志。
func TestD1RetryDoesNotInsertLogs(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "share_transfer_service.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "RetryTransfer" {
			continue
		}
		found = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && (selector.Sel.Name == "BatchInsert" || selector.Sel.Name == "SubmitTransfer") {
				t.Error("重试必须复用既有日志，不能重新提交或批量插入")
			}
			return true
		})
	}
	if !found {
		t.Fatal("重试入口不存在")
	}
}

// TestD1RetryHistoricalTaskWithoutPassword 验证历史缺失密码保持空值，不猜测或伪造密码。
func TestD1RetryHistoricalTaskWithoutPassword(t *testing.T) {
	svc, store, client, mock := newD1TransferTestService(t)
	const taskID = "historical-transfer"
	if err := store.Create(taskID, "share_transfer", "测试历史任务"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMetadata(taskID, map[string]interface{}{
		"share_code": "share-test", "target_directory": "/测试目标", "target_account_id": 7,
		"conflict_strategy": "skip", "failed_items": []domain.FailedItem{{Name: "电影.mkv", Retryable: true}},
	}); err != nil {
		t.Fatal(err)
	}
	expectD1RetryLogs(mock, taskID)
	expectD1TargetAccount(mock)
	mock.ExpectExec(d1LogUpdateSQL).WithArgs("success", "", taskID, "fid-original").WillReturnResult(sqlmock.NewResult(0, 1))
	response, err := svc.RetryTransfer(context.Background(), taskID)
	if err != nil || response.RetriedFiles != 1 {
		t.Fatalf("历史任务重试失败: %v", err)
	}
	waitD1Transfer(t, store, client, "")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestD1SubmitMetadataFailureStopsExecution 验证重试参数保存失败后不写日志、不启动转存且错误可追溯。
func TestD1SubmitMetadataFailureStopsExecution(t *testing.T) {
	svc, store, client, mock := newD1TransferTestService(t)
	store.metadataErr = errors.New("metadata unavailable")
	expectD1TargetAccount(mock)
	response, err := svc.SubmitTransfer(context.Background(), domain.TransferRequest{
		ShareCode: "share-test", Password: "unit-test-password-D1", TargetCloud115Id: 7,
		Files: []domain.ShareTransferFileItem{{Fid: "fid-original", Name: "电影.mkv"}},
	})
	if err == nil || response != nil || !errors.Is(err, store.metadataErr) {
		t.Fatal("保存重试参数失败必须停止提交并返回原错误")
	}
	if len(client.calls) != 0 {
		t.Fatal("元数据保存失败后不能执行转存")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestD1TaskPasswordNotExposed 验证所有公开任务投影不泄露密码，且过滤不修改内部元数据或已存原值。
func TestD1TaskPasswordNotExposed(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()
	store := dao.NewTaskRedisDAO(client)
	if err := store.Create("private-transfer", "share_transfer", "测试任务"); err != nil {
		t.Fatal(err)
	}
	const password = "unit-test-password-D1"
	if err := store.UpdateMetadata("private-transfer", map[string]interface{}{
		"share_transfer_password": password, "share_code": "share-test", "failed_items": []interface{}{},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewTaskService(store)
	assertPrivate := func(value interface{}, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), password) || strings.Contains(string(payload), "share_transfer_password") {
			t.Fatal("任务响应泄露了分享密码")
		}
		if !strings.Contains(string(payload), "share-test") {
			t.Fatal("过滤密码不应删除公开元数据")
		}
	}
	assertPrivate(svc.Get("private-transfer"))
	assertPrivate(svc.GetAll())
	assertPrivate(svc.GetUnified())
	raw, err := store.Get("private-transfer")
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(svc.TaskStatusToDomain(raw), nil)
	assertPrivate((&DashboardService{}).limitTasks([]map[string]interface{}{raw}, 8), nil)
	if raw["metadata"].(map[string]interface{})["share_transfer_password"] != password {
		t.Fatal("响应过滤不应修改内部任务元数据")
	}
	reread, err := store.Get("private-transfer")
	if err != nil || reread["metadata"].(map[string]interface{})["share_transfer_password"] != password {
		t.Fatal("响应过滤不应清除已保存的重试密码")
	}
}
