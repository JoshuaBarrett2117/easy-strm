package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestSQLParameterSelectionSyncEnqueue 验证空串与指定作品始终提交变更入队，入队失败时回滚。
// sqlmock 只验证实际提交的 SQL、参数、顺序和事务终态，不执行函数或证明 PostgreSQL 类型推断。
func TestSQLParameterSelectionSyncEnqueue(t *testing.T) {
	if len(shareSelectionSyncSQL) == 0 {
		t.Fatal("selection sync SQL is empty")
	}
	for _, test := range []struct {
		name string
		work string
		fail bool
	}{
		{"all_works", "", false},
		{"specific_work", "movie:tmdb:550", false},
		{"all_works_enqueue_failure", "", true},
		{"specific_work_enqueue_failure", "movie:tmdb:550", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectExec(shareSelectionLockSQL).WithArgs().WillReturnResult(sqlmock.NewResult(0, 1))
			for _, query := range shareSelectionSyncSQL[:len(shareSelectionSyncSQL)-1] {
				expectation := mock.ExpectExec(query)
				if strings.Contains(query, "$1") {
					expectation.WithArgs(test.work)
				} else {
					expectation.WithArgs()
				}
				expectation.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			enqueue := mock.ExpectExec(`SELECT share_export_enqueue(ARRAY(SELECT work_key FROM selection_changed ORDER BY work_key),'selection-sync') WHERE $1::text IS NOT NULL`).WithArgs(test.work)
			enqueueErr := errors.New("enqueue failed")
			if test.fail {
				enqueue.WillReturnError(enqueueErr)
				mock.ExpectRollback()
			} else {
				enqueue.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = NewShareRecordDAO(database).SyncSelections(context.Background(), test.work)
			if test.fail {
				if !errors.Is(err, enqueueErr) || !strings.Contains(err.Error(), "刷新来源选择") {
					t.Errorf("expected wrapped enqueue failure, got %v", err)
				}
			} else if err != nil {
				t.Error(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
