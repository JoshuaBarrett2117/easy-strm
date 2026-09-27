package dao

import (
	"context"
	"fmt"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"testing"
)

func TestDeleteWithStrmAtomicCleanup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			paths := []string{"/export/movie.strm"}
			entries := []string{"mapping"}
			mock.ExpectBegin()
			mock.ExpectExec("DELETE FROM t_strm_file WHERE strm_config_id=-1").WithArgs(pq.Array(paths)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM t_strm_export_history WHERE owner_key='share:default'").WithArgs(pq.Array(paths)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM t_strm_export_state WHERE owner_key='share:default'").WithArgs(pq.Array(paths)).WillReturnResult(sqlmock.NewResult(0, 1))
			q := mock.ExpectExec("DELETE FROM t_share_strm WHERE id=ANY").WithArgs(pq.Array(entries))
			if fail {
				q.WillReturnError(fmt.Errorf("db failure"))
				mock.ExpectRollback()
			} else {
				q.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM t_share_record").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM t_share_media m WHERE NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err := NewShareRecordDAO(db).DeleteWithStrm(context.Background(), 7, entries, paths)
			if fail != (err != nil) {
				t.Fatalf("err=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
