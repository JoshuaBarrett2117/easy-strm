package dao

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestStrmOutputLockRejectsOverlappingDirectory(t *testing.T) {
	db, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	m.ExpectQuery("SELECT pg_try_advisory_lock_shared").WithArgs("root").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	m.ExpectQuery("SELECT pg_try_advisory_lock\\(").WithArgs("root/child").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(false))
	m.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	if _, e = LockStrmOutput(context.Background(), db, []string{"root", "root/child"}); e == nil {
		t.Fatal("重叠目录未拒绝")
	}
	if e = m.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
