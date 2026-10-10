package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSelectionResolveStickyAndFallback(t *testing.T) {
	for _, scenario := range []string{"valid-auto", "valid-manual", "invalid-manual", "fallback", "zero-candidates"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			mode := "auto"
			valid := scenario == "valid-auto" || scenario == "valid-manual"
			if scenario == "valid-manual" || scenario == "invalid-manual" {
				mode = "manual"
			}
			mock.ExpectQuery("SELECT s.media_item_key").WithArgs("work:0:0").WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "title", "valid"}).AddRow("work:0:0", "work", 0, 0, 7, mode, 3, 2, "fixed.strm", "film", valid))
			if !valid && mode == "auto" {
				rows := sqlmock.NewRows([]string{"candidate"})
				if scenario == "fallback" {
					rows.AddRow(9)
				}
				mock.ExpectQuery("SELECT c.candidate_id.*ORDER BY c.file_size DESC,c.share_id,c.source_file_id,c.candidate_id").WithArgs("work", 0, 0).WillReturnRows(rows)
				if scenario == "fallback" {
					mock.ExpectExec("UPDATE t_share_media_selection").WithArgs("work:0:0", int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("SELECT share_export_enqueue").WithArgs("work").WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if valid || scenario == "fallback" {
				candidate := int64(7)
				if scenario == "fallback" {
					candidate = 9
				}
				mock.ExpectQuery("SELECT source_file_id").WithArgs(candidate).WillReturnRows(sqlmock.NewRows([]string{"source"}).AddRow(21))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			selection, source, err := NewShareRecordDAO(database).ResolveSelection(context.Background(), "work:0:0")
			if valid || scenario == "fallback" {
				if err != nil || source != 21 || selection.RelativePath != "fixed.strm" {
					t.Fatalf("selection=%+v source=%d err=%v", selection, source, err)
				}
			} else if !errors.Is(err, ErrShareSelectionUnavailable) {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelectionReceiptStaleCASPreservesChoiceAndPath(t *testing.T) {
	for _, scenario := range []string{"success", "stale", "path-conflict", "failure"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			connection, err := database.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			mock.ExpectBegin()
			revision := int64(2)
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			path := ""
			if scenario == "stale" {
				revision = 3
			}
			if scenario == "path-conflict" {
				path = "other.strm"
			}
			mock.ExpectQuery("SELECT selection_revision,stable_relative_path").WithArgs("w:0:0").WillReturnRows(sqlmock.NewRows([]string{"revision", "path"}).AddRow(revision, path))
			if scenario != "path-conflict" {
				mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 1))
				expect := mock.ExpectExec("INSERT INTO t_strm_export_state")
				if scenario == "failure" {
					expect.WillReturnError(errors.New("receipt failed"))
				} else {
					expect.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("UPDATE t_share_media_selection SET stable_relative_path").WithArgs("w:0:0", "fixed.strm", int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if scenario == "success" || scenario == "stale" {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = (&StrmExportDAO{Conn: connection}).SaveSelectionExport(context.Background(), ExportState{Owner: "share:default", Key: "w:0:0"}, domain.ShareSelection{ItemKey: "w:0:0", Revision: 2}, "fixed.strm")
			if (scenario == "success") != (err == nil) {
				t.Fatalf("err=%v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
