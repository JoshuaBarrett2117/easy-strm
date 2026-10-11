package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestShareDestructiveWritesFencePartialScans(test *testing.T) {
	for _, operation := range []string{"delete_file", "clear_share", "clear_selected", "clear_all"} {
		for _, fail := range []bool{false, true} {
			test.Run(operation+map[bool]string{false: "/commit", true: "/rollback"}[fail], func(test *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					test.Fatal(err)
				}
				defer db.Close()
				mock.ExpectBegin()
				if operation == "clear_share" {
					mock.ExpectQuery("SELECT id FROM t_share_record WHERE id=\\$1 FOR UPDATE").WithArgs(1).
						WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				}
				fence := mock.ExpectExec("UPDATE t_share_record SET version=version\\+1,updated_at=now\\(\\)")
				switch operation {
				case "delete_file", "clear_share":
					fence.WithArgs(1)
				case "clear_selected":
					fence.WithArgs(pq.Array([]int{1, 2}))
				}
				failure := errors.New("fence unavailable")
				if fail {
					fence.WillReturnError(failure)
					mock.ExpectRollback()
				} else {
					fence.WillReturnResult(sqlmock.NewResult(0, 1))
					deletion := mock.ExpectExec("DELETE FROM t_share_media_file")
					switch operation {
					case "delete_file", "clear_share":
						deletion.WithArgs(1)
					case "clear_selected":
						deletion.WithArgs(pq.Array([]int{1, 2}))
					}
					deletion.WillReturnResult(sqlmock.NewResult(0, 3))
					mock.ExpectExec("DELETE FROM t_share_media").WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectCommit()
				}
				dao := NewShareRecordDAO(db)
				switch operation {
				case "delete_file":
					err = dao.DeleteMedia(context.Background(), 1)
				case "clear_share":
					_, err = dao.ClearMedia(context.Background(), 1)
				case "clear_selected":
					_, err = dao.ClearSelectedMedia(context.Background(), []int{1, 2})
				case "clear_all":
					_, err = dao.ClearAllMedia(context.Background())
				}
				if fail && !errors.Is(err, failure) || !fail && err != nil {
					test.Fatal(err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					test.Fatal(err)
				}
			})
		}
	}
}
