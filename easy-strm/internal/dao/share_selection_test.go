package dao

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestSelectionCASMembershipAndAtomicDirty(t *testing.T) {
	for _, test := range []struct {
		name      string
		revision  int64
		valid     bool
		dirtyFail bool
	}{
		{"success", 3, true, false}, {"stale", 4, true, false}, {"foreign", 3, false, false}, {"dirty_failure", 3, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			store := NewShareRecordDAO(database)
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT selection_revision,work_key").WithArgs("work:1:2").WillReturnRows(sqlmock.NewRows([]string{"revision", "work"}).AddRow(test.revision, "work"))
			if test.revision == 3 {
				mock.ExpectQuery("SELECT EXISTS").WithArgs("work:1:2", int64(22)).WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(test.valid))
				if test.valid {
					mock.ExpectExec("UPDATE t_share_media_selection").WithArgs("work:1:2", int64(22), int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
					expect := mock.ExpectExec("SELECT share_export_enqueue").WithArgs("work")
					if test.dirtyFail {
						expect.WillReturnError(errors.New("dirty failed"))
					} else {
						expect.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if test.revision == 3 && test.valid && !test.dirtyFail {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = store.ChangeSelection(context.Background(), domain.ShareSelectionChange{ItemKey: "work:1:2", CandidateID: 22, ExpectedRevision: 3})
			if (err == nil) != (test.revision == 3 && test.valid && !test.dirtyFail) {
				t.Fatalf("err=%v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelectionSyncTransactionAndFailure(t *testing.T) {
	for _, failure := range []int{-1, 0, 5, len(shareSelectionSyncSQL) - 1} {
		t.Run(string(rune('a'+failure+1)), func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			for index, query := range shareSelectionSyncSQL {
				expect := mock.ExpectExec(regexp.QuoteMeta(query))
				if strings.Contains(query, "$1") {
					expect.WithArgs("old")
				}
				if index == failure {
					expect.WillReturnError(errors.New("sync failed"))
					break
				}
				expect.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if failure < 0 {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err := NewShareRecordDAO(database).SyncSelections(context.Background(), "old")
			if (err == nil) != (failure < 0) {
				t.Fatalf("err=%v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelectionReadsDoNotDiscover(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count").WithArgs("film").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT s.media_item_key").WithArgs("film", 20, 20).WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "title", "valid"}).AddRow("w:0:0", "w", 0, 0, 9, "manual", 3, 2, "movie/name.strm", "film", false))
	mock.ExpectCommit()
	rows, total, err := NewShareRecordDAO(database).ListSelections(context.Background(), domain.ShareSelectionQuery{Keyword: "film", Page: 2, PageSize: 20})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Valid {
		t.Fatalf("rows=%v total=%d err=%v", rows, total, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionMissingDoesNotModify(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT selection_revision").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	err := NewShareRecordDAO(database).ChangeSelection(context.Background(), domain.ShareSelectionChange{ItemKey: "missing", CandidateID: 1, ExpectedRevision: 1})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
