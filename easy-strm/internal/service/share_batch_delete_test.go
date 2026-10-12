package service

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func expectBatchDeleteWithoutExports(mock sqlmock.Sqlmock, id int) {
	expectOperationTarget(mock, id)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(id, "example").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT id FROM t_share_strm").WithArgs("example").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_record").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE t_share_operation_queue SET result=jsonb_set").WithArgs(sqlmock.AnyArg(), id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// TestBatchDeleteContinuesAfterFailureAndMissing 验证中间失败不终止批量清理，缺失项跳过且仅成功目标失效。
func TestBatchDeleteContinuesAfterFailureAndMissing(t *testing.T) {
	s, mock, store := operationFixture(t)
	for _, id := range []int{1, 2, 3, 4} {
		expectOperationTarget(mock, id)
	}
	expectBatchDeleteWithoutExports(mock, 1)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(2).WillReturnError(errors.New("读取失败"))
	expectBatchDeleteWithoutExports(mock, 3)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(4).WillReturnError(sql.ErrNoRows)
	id, err := s.StartBatchDelete(context.Background(), []int{4, 2, 3, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, s, id, "failed")
	v, _ := store.Get(context.Background(), id)
	if !reflect.DeepEqual(v.ShareIDs, []int{1, 2, 3, 4}) || v.Result["deleted"] != int64(2) ||
		!reflect.DeepEqual(v.Result["deleted_ids"], []int{1, 3}) || !reflect.DeepEqual(v.Result["failed_ids"], []int{2}) ||
		!reflect.DeepEqual(v.Result["skipped_ids"], []int{4}) || !strings.Contains(v.Error, "2: 读取失败") {
		t.Fatal(v)
	}
	s.Coordinator().mu.Lock()
	validEpochs := s.Coordinator().epochs[1] == 1 && s.Coordinator().epochs[3] == 1 && s.Coordinator().epochs[2] == 0
	s.Coordinator().mu.Unlock()
	if !validEpochs {
		t.Fatal("部分失败未仅失效成功分享")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchDeleteRestartCheckpoint 验证重启兼容JSON数字与整数检查点，仅跳过已成功目标并重试失败项。
func TestBatchDeleteRestartCheckpoint(t *testing.T) {
	for _, checkpoint := range []interface{}{[]int{1, 1}, []interface{}{float64(1), float64(1)}} {
		t.Run(reflect.TypeOf(checkpoint).String(), func(t *testing.T) {
			s, mock, store := operationFixture(t)
			store.rows["restart"] = domain.ShareOperation{TaskID: "restart", Kind: "share_delete", ShareIDs: []int{1, 2, 3}, Status: "running", Result: map[string]interface{}{"deleted_ids": checkpoint, "failed_ids": []int{2}}}
			expectBatchDeleteWithoutExports(mock, 2)
			expectBatchDeleteWithoutExports(mock, 3)
			if err := s.RecoverOperations(context.Background()); err != nil {
				t.Fatal(err)
			}
			waitOperationStatus(t, s, "restart", "completed")
			v, _ := store.Get(context.Background(), "restart")
			if v.Result["deleted"] != int64(3) || !reflect.DeepEqual(v.Result["deleted_ids"], []int{1, 2, 3}) || len(v.Result["failed_ids"].([]int)) != 0 {
				t.Fatal(v)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
