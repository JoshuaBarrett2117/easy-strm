package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectBatchDeleteCascade(t *testing.T, mock sqlmock.Sqlmock, id int, code, path string, shared bool) {
	t.Helper()
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/" + code))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(id, code).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(shared))
	if !shared {
		mock.ExpectQuery("SELECT id FROM t_share_strm").WithArgs(code).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(code))
		mock.ExpectQuery("SELECT output_path").WillReturnRows(sqlmock.NewRows([]string{"path", "linked"}).AddRow(path, true))
		expectShareDeletePathLock(mock, path)
		mock.ExpectExec("INSERT INTO t_strm_export_history").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectBegin()
	if !shared {
		for _, table := range []string{"t_strm_file", "t_strm_export_history", "t_strm_export_state", "t_share_media_selection", "t_share_strm"} {
			mock.ExpectExec("DELETE FROM " + table).WillReturnResult(sqlmock.NewResult(0, 1))
		}
	}
	mock.ExpectExec("DELETE FROM t_share_record").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_share_operation_queue SET result=jsonb_set").WithArgs(sqlmock.AnyArg(), id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if !shared {
		mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

// TestBatchDeleteRealStrmCascadeAndSharedCode 使用临时STRM验证多目标级联清理及共享分享码最后引用清理语义。
func TestBatchDeleteRealStrmCascadeAndSharedCode(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "multiple_cascades", true: "shared_code"}[shared], func(t *testing.T) {
			s, mock, store := operationFixture(t)
			dir := t.TempDir()
			paths := []string{filepath.Join(dir, "first.strm"), filepath.Join(dir, "second.strm")}
			for index, path := range paths {
				code := []string{"first", "second"}[index]
				if shared {
					code = "second"
				}
				if err := os.WriteFile(path, []byte("http://localhost/share-strm/"+code), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []int{1, 2} {
				expectOperationTarget(mock, id)
			}
			if shared {
				expectBatchDeleteCascade(t, mock, 1, "second", paths[1], true)
				expectBatchDeleteCascade(t, mock, 2, "second", paths[1], false)
			} else {
				expectBatchDeleteCascade(t, mock, 1, "first", paths[0], false)
				expectBatchDeleteCascade(t, mock, 2, "second", paths[1], false)
			}
			id, err := s.StartBatchDelete(context.Background(), []int{1, 2})
			if err != nil {
				t.Fatal(err)
			}
			waitOperationStatus(t, s, id, "completed")
			v, _ := store.Get(context.Background(), id)
			if v.Result["deleted"] != int64(2) {
				t.Fatal(v)
			}
			if shared {
				if _, err := os.Stat(paths[0]); err != nil {
					t.Fatal("未列入清单的文件被删除", err)
				}
				paths = paths[1:]
			}
			for _, path := range paths {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("STRM未删除 %s: %v", path, err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestBatchDeleteAdmissionValidation 验证空集合、非正数及不存在的分享不会创建队列任务。
func TestBatchDeleteAdmissionValidation(t *testing.T) {
	s, mock, store := operationFixture(t)
	for _, ids := range [][]int{nil, {}, {0}, {-1}, {1, -2}} {
		if _, err := s.StartBatchDelete(context.Background(), ids); err == nil {
			t.Fatal("接受无效ID", ids)
		}
	}
	expectOperationTarget(mock, 1)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(2).WillReturnError(sql.ErrNoRows)
	if _, err := s.StartBatchDelete(context.Background(), []int{2, 1}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if len(store.rows) != 0 {
		t.Fatal("校验失败仍创建任务")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchDeletePendingCancellationAndTargetLocks 验证等待任务可取消且仅占用所选目标，同请求复用任务。
func TestBatchDeletePendingCancellationAndTargetLocks(t *testing.T) {
	s, mock, _ := operationFixture(t)
	release := acquireFixture(t, s.Coordinator(), shareResource{key: shareKey(2)})
	defer release()
	for _, id := range []int{1, 2} {
		expectOperationTarget(mock, id)
	}
	id, err := s.StartBatchDelete(context.Background(), []int{2, 1})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Coordinator().pendingShare(1) || !s.Coordinator().pendingShare(2) || s.Coordinator().pendingShare(3) {
		t.Fatal("等待锁范围错误")
	}
	for _, target := range []int{1, 2} {
		expectOperationTarget(mock, target)
	}
	same, err := s.StartBatchDelete(context.Background(), []int{1, 2, 2})
	if err != nil || same != id {
		t.Fatal("未复用活动任务", same, err)
	}
	other := acquireFixture(t, s.Coordinator(), shareResource{key: shareKey(3), exclusive: true})
	other()
	if err := s.tasks.Cancel(id); err != nil {
		t.Fatal(err)
	}
	release()
	waitOperationStatus(t, s, id, "cancelled")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchDeleteRunningNoncancellableAndNoPrematureCompletion 验证执行阶段不可取消，终态包含后续目标结果。
func TestBatchDeleteRunningNoncancellableAndNoPrematureCompletion(t *testing.T) {
	s, mock, store := operationFixture(t)
	for _, id := range []int{1, 2} {
		expectOperationTarget(mock, id)
	}
	expectBatchDeleteWithoutExports(mock, 1)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(2).WillDelayFor(200 * time.Millisecond).WillReturnError(sql.ErrNoRows)
	id, err := s.StartBatchDelete(context.Background(), []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "running")
	if err := s.tasks.Cancel(id); err == nil {
		t.Fatal("执行阶段被取消")
	}
	waitOperationStatus(t, s, id, "completed")
	v, _ := store.Get(context.Background(), id)
	if v.Result["deleted"] != int64(1) || !reflect.DeepEqual(v.Result["skipped_ids"], []int{2}) {
		t.Fatal(v)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchDeleteCheckpointRollbackContinues 验证检查点未写入时回滚当前删除，但仍执行后续目标。
func TestBatchDeleteCheckpointRollbackContinues(t *testing.T) {
	s, mock, store := operationFixture(t)
	for _, id := range []int{1, 2} {
		expectOperationTarget(mock, id)
	}
	expectOperationTarget(mock, 1)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_record").WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE t_share_operation_queue SET result=jsonb_set").WithArgs(sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	expectBatchDeleteWithoutExports(mock, 2)
	id, err := s.StartBatchDelete(context.Background(), []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "failed")
	v, _ := store.Get(context.Background(), id)
	if v.Result["deleted"] != int64(1) || !reflect.DeepEqual(v.Result["deleted_ids"], []int{2}) || !reflect.DeepEqual(v.Result["failed_ids"], []int{1}) {
		t.Fatal(v)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
