package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type batchDeleteFinishAudit struct {
	*operationMemory
	mock         sqlmock.Sqlmock
	beforeFinish func(error)
	calls        atomic.Int32
}

func (store *batchDeleteFinishAudit) Finish(ctx context.Context, id, status, message string, result map[string]interface{}) error {
	store.calls.Add(1)
	store.beforeFinish(store.mock.ExpectationsWereMet())
	return store.operationMemory.Finish(ctx, id, status, message, result)
}

// TestBatchDeleteFinishesAfterAllCascadeAttempts 验证真实文件清理在中间失败后继续，全部尝试后仅写入一次终态。
func TestBatchDeleteFinishesAfterAllCascadeAttempts(t *testing.T) {
	s, mock, store := operationFixture(t)
	audit := &batchDeleteFinishAudit{operationMemory: store, mock: mock, beforeFinish: func(err error) {
		if err != nil {
			t.Error("所有删除尝试结束前写入终态", err)
		}
	}}
	s.SetOperationStore(audit)
	paths := []string{filepath.Join(t.TempDir(), "first.strm"), filepath.Join(t.TempDir(), "last.strm")}
	for index, path := range paths {
		code := []string{"first", "last"}[index]
		if err := os.WriteFile(path, []byte("http://localhost/share-strm/"+code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int{1, 2, 3} {
		expectOperationTarget(mock, id)
	}
	expectBatchDeleteCascade(t, mock, 1, "first", paths[0], false)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(2).WillReturnError(errors.New("分享读取失败"))
	expectBatchDeleteCascade(t, mock, 3, "last", paths[1], false)
	id, err := s.StartBatchDelete(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "failed")
	v, _ := store.Get(context.Background(), id)
	if audit.calls.Load() != 1 || v.Result["deleted"] != int64(2) || !reflect.DeepEqual(v.Result["failed_ids"], []int{2}) {
		t.Fatal(v, audit.calls.Load())
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("失败阻止后续STRM清理", path, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchDeleteSingleTargetKeepsOriginalCompletionSQL 验证去重后仅一个目标时仍使用原单条完成事务。
func TestBatchDeleteSingleTargetKeepsOriginalCompletionSQL(t *testing.T) {
	s, mock, _ := operationFixture(t)
	expectOperationTarget(mock, 1)
	expectOperationTarget(mock, 1)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_record").WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE t_share_operation_queue SET status='completed'").WithArgs(sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	id, err := s.StartBatchDelete(context.Background(), []int{1, 1})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "completed")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestStartDeleteRuntimeMissingCompletesWithoutDeletion 验证单条删除执行时目标缺失仍完成，且实际删除数为零。
func TestStartDeleteRuntimeMissingCompletesWithoutDeletion(t *testing.T) {
	s, mock, store := operationFixture(t)
	expectOperationTarget(mock, 1)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(1).WillReturnError(sql.ErrNoRows)
	id, err := s.StartDelete(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "completed")
	v, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "completed" || v.Error != "" || v.Result["deleted"] != int64(0) ||
		!reflect.DeepEqual(v.Result["deleted_ids"], []int{}) || !reflect.DeepEqual(v.Result["failed_ids"], []int{}) ||
		!reflect.DeepEqual(v.Result["skipped_ids"], []int{1}) {
		t.Fatal(v)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchDeleteSingleFailureRetainsResult 验证单目标失败保留完整结果，不失效分享且不改变原单条删除事务。
func TestBatchDeleteSingleFailureRetainsResult(t *testing.T) {
	s, mock, store := operationFixture(t)
	expectOperationTarget(mock, 1)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(1).WillReturnError(errors.New("读取失败"))
	id, err := s.StartBatchDelete(context.Background(), []int{1})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "failed")
	v, _ := store.Get(context.Background(), id)
	if v.Result["deleted"] != int64(0) || !reflect.DeepEqual(v.Result["deleted_ids"], []int{}) ||
		!reflect.DeepEqual(v.Result["failed_ids"], []int{1}) || !reflect.DeepEqual(v.Result["skipped_ids"], []int{}) {
		t.Fatal(v)
	}
	s.Coordinator().mu.Lock()
	epoch := s.Coordinator().epochs[1]
	s.Coordinator().mu.Unlock()
	if epoch != 0 {
		t.Fatal("单目标删除失败不应失效分享", epoch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
