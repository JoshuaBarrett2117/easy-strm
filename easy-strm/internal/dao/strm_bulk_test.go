package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStrmMarkMissingBatchAtomic(t *testing.T) {
	for _, failure := range []int{-1, 0, 1, 2} {
		database, mock, _ := sqlmock.New()
		connection, _ := database.Conn(context.Background())
		mock.ExpectBegin()
		for index, query := range []string{"INSERT INTO t_strm_export_state.*ANY", "UPDATE t_strm_export_state.*ANY", "DELETE FROM t_strm_export_history.*ANY"} {
			expect := mock.ExpectExec(query).WithArgs(`{"one","two"}`)
			if index == failure {
				expect.WillReturnError(errors.New("batch failed"))
				break
			}
			expect.WillReturnResult(sqlmock.NewResult(0, 2))
		}
		if failure < 0 {
			mock.ExpectExec("UPDATE t_strm_clear_run").WithArgs("run").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		} else {
			mock.ExpectRollback()
		}
		err := (&StrmExportDAO{Conn: connection}).MarkMissing(context.Background(), []string{"one", "two"}, "run")
		if (err == nil) != (failure < 0) {
			t.Fatal(err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		connection.Close()
		database.Close()
	}
}
