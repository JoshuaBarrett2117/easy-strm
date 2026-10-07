package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStrmDirectorySharedAndLeafExclusive(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	for _, p := range []string{"root", "root/library"} {
		m.ExpectQuery("SELECT pg_try_advisory_lock_shared").WithArgs(p).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	}
	d, err := LockStrmOutputShared(context.Background(), db, []string{"root", "root/library"})
	if err != nil {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT pg_try_advisory_lock\\(").WithArgs("root/library/a.strm").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	m.ExpectExec("SELECT pg_advisory_unlock\\(").WithArgs("root/library/a.strm").WillReturnResult(sqlmock.NewResult(0, 1))
	release, err := d.LockPath(context.Background(), "root/library/a.strm")
	if err != nil {
		t.Fatal(err)
	}
	release()
	m.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	d.Close()
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFinishStrmSnapshotDoesNotInvalidateConcurrentWrites(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	c, _ := db.Conn(context.Background())
	defer c.Close()
	d := &StrmExportDAO{Conn: c}
	m.ExpectExec("WHERE owner_key=\\$1 AND export_key=\\$2 AND last_seen_run_id=\\$3").WithArgs("share:default", "old-unseen", "snapshot-run").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := d.FinishSnapshot(context.Background(), "share:default", ExportSnapshot{"old-unseen": "snapshot-run", "seen": "other-run"}, map[string]bool{"seen": true}); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
