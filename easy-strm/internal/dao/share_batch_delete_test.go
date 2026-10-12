package dao

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestBatchDeleteCheckpointAtomicAndScoped 验证批量上下文优先、精确检查点SQL及更新行数异常时的事务回滚。
func TestBatchDeleteCheckpointAtomicAndScoped(t *testing.T) {
	query := `UPDATE t_share_operation_queue SET result=jsonb_set(
 COALESCE(NULLIF(result,'null'::jsonb),'{}'::jsonb),'{deleted_ids}',
 CASE WHEN COALESCE(NULLIF(result->'deleted_ids','null'::jsonb),'[]'::jsonb) @> to_jsonb($2::int)
 THEN COALESCE(NULLIF(result->'deleted_ids','null'::jsonb),'[]'::jsonb)
 ELSE COALESCE(NULLIF(result->'deleted_ids','null'::jsonb),'[]'::jsonb) || to_jsonb($2::int) END),
 published=FALSE,updated_at=now() WHERE task_id=$1 AND status='running'`
	for _, test := range []struct {
		name        string
		rows        int64
		failure     error
		rowsFailure bool
	}{
		{"one_row", 1, nil, false}, {"missing_task", 0, nil, false}, {"multiple_rows", 2, nil, false},
		{"exec_failure", 0, errors.New("检查点不可用"), false}, {"rows_failure", 0, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectExec("DELETE FROM t_share_record").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
			checkpoint := mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs("batch-task", 7)
			if test.failure != nil {
				checkpoint.WillReturnError(test.failure)
			} else if test.rowsFailure {
				checkpoint.WillReturnResult(sqlmock.NewErrorResult(errors.New("无法读取行数")))
			} else {
				checkpoint.WillReturnResult(sqlmock.NewResult(0, test.rows))
			}
			success := test.rows == 1 && test.failure == nil && !test.rowsFailure
			if success {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			ctx := WithShareBatchDelete(WithShareOperationTask(context.Background(), "single-task"), "batch-task", 7)
			err = NewShareRecordDAO(db).DeleteWithStrm(ctx, 7, nil, nil)
			if success != (err == nil) {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
