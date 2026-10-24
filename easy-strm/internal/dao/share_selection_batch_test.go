package dao

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func expectSelectionPreparation(mock sqlmock.Sqlmock, work string) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
	for _, query := range shareSelectionPrepareSQL {
		expect := mock.ExpectExec(regexp.QuoteMeta(query))
		if strings.Contains(query, "$1") {
			expect.WithArgs(work)
		}
		expect.WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery(regexp.QuoteMeta(selection115PreflightSQL)).WithArgs(work).WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "ref", "kind"}))
}

func TestSelectionBatchReportsAllInvalidAndNeverUpdates(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	expectSelectionPreparation(mock, "")
	rows := sqlmock.NewRows([]string{"key", "mode"})
	for _, key := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		rows.AddRow(key, "manual")
	}
	rows.AddRow("z", "auto")
	mock.ExpectQuery(regexp.QuoteMeta(shareSelectionPrepareErrorsSQL)).WillReturnRows(rows)
	mock.ExpectRollback()
	count, err := NewShareRecordDAO(database).ResolveSelections(context.Background(), "")
	var report *ShareSelectionPreparationError
	if count != 0 || !errors.As(err, &report) || !errors.Is(err, ErrShareSelectionUnavailable) || len(report.ManualKeys) != 7 || len(report.AutoKeys) != 1 || report.ManualKeys[6] != "g" || !strings.Contains(err.Error(), "手选失效 7 项") {
		t.Fatalf("count=%d report=%+v err=%v", count, report, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionBatchValidUnchangedEmptyAndSingleSortedEnqueue(t *testing.T) {
	for _, scenario := range []string{"empty", "valid-auto-manual", "changed", "enqueue-error", "update-error", "read-error", "commit-error"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			expectSelectionPreparation(mock, "work")
			invalid := mock.ExpectQuery(regexp.QuoteMeta(shareSelectionPrepareErrorsSQL))
			if scenario == "read-error" {
				invalid.WillReturnError(errors.New("read failed"))
			} else {
				invalid.WillReturnRows(sqlmock.NewRows([]string{"key", "mode"}))
				update := mock.ExpectQuery(regexp.QuoteMeta(shareSelectionPrepareUpdateSQL))
				if scenario == "update-error" {
					update.WillReturnError(errors.New("update failed"))
				} else {
					rows := sqlmock.NewRows([]string{"work"})
					if scenario == "changed" || scenario == "enqueue-error" || scenario == "commit-error" {
						rows.AddRow("z").AddRow("a").AddRow("z")
					}
					update.WillReturnRows(rows)
					if scenario == "changed" || scenario == "enqueue-error" || scenario == "commit-error" {
						enqueue := mock.ExpectExec("SELECT share_export_enqueue").WithArgs(`{"a","z"}`)
						if scenario == "enqueue-error" {
							enqueue.WillReturnError(errors.New("enqueue failed"))
						} else {
							enqueue.WillReturnResult(sqlmock.NewResult(0, 1))
						}
					}
				}
			}
			success := scenario == "empty" || scenario == "valid-auto-manual" || scenario == "changed"
			if success {
				mock.ExpectCommit()
			} else if scenario == "commit-error" {
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			} else {
				mock.ExpectRollback()
			}
			count, err := NewShareRecordDAO(database).ResolveSelections(context.Background(), "work")
			if (err == nil) != success || (scenario == "changed" && count != 3) || (scenario != "changed" && count != 0) {
				t.Fatalf("count=%d err=%v", count, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelectionBatchEveryMaterializationFailureRollsBack(t *testing.T) {
	for failure := range shareSelectionPrepareSQL {
		database, mock, _ := sqlmock.New()
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
		for index, query := range shareSelectionPrepareSQL {
			expect := mock.ExpectExec(regexp.QuoteMeta(query))
			if index == failure {
				expect.WillReturnError(errors.New("stage failed"))
				break
			}
			expect.WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectRollback()
		if _, err := NewShareRecordDAO(database).ResolveSelections(context.Background(), ""); err == nil {
			t.Fatal("failure committed")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}

func TestSelectionBatchBlockedSQLCancellation(t *testing.T) {
	for _, blocked := range []string{shareSelectionLockSQL, shareSelectionPrepareSQL[0]} {
		database, mock, _ := sqlmock.New()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		mock.ExpectBegin()
		if blocked != shareSelectionLockSQL {
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectExec(regexp.QuoteMeta(blocked)).WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()
		started := time.Now()
		if _, err := NewShareRecordDAO(database).ResolveSelections(ctx, ""); err == nil || time.Since(started) > 500*time.Millisecond {
			t.Fatalf("blocked cancellation: %v", err)
		}
		cancel()
		deadline := time.Now().Add(time.Second)
		for mock.ExpectationsWereMet() != nil && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}

func TestSelectionBatchSQLValidityAndDeterminism(t *testing.T) {
	if !strings.Contains(shareSelectionColumns, shareSelectionLivePredicate) || !strings.Contains(shareSelectionPrepareSQL[0], shareSelectionLivePredicate) {
		t.Fatal("read/write validity drift")
	}
	joined := strings.Join(shareSelectionPrepareSQL, "\n")
	for _, required := range []string{"live.media_type='movie'", "live.media_type='tv'", "NOT i.revoked", "live.work_key=i.work_key", "DISTINCT ON(work_key,season_number,episode_number)", "file_size DESC,share_id,source_file_id,candidate_id", "ANALYZE selection_valid_member", "LEFT JOIN selection_valid_member"} {
		if !strings.Contains(joined, required) {
			t.Fatal(required)
		}
	}
	if strings.Contains(joined, "EXISTS(SELECT 1 FROM selection_valid_member") || strings.Contains(shareSelectionPrepareUpdateSQL, "exported_revision=") || !strings.Contains(shareSelectionPrepareUpdateSQL, "s.selection_revision+1") || !strings.Contains(shareSelectionPrepareUpdateSQL, "s.selection_mode='auto'") {
		t.Fatal("quadratic scan or selection invariants broken")
	}
}

func TestSelectionInternalCursorSkipsCountUIRetainsTotal(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(expectedSelectionListSQL(true))).WithArgs("", 100, "work", 1, 2).WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "kind", "ref", "title", "valid"}))
	mock.ExpectCommit()
	rows, total, err := NewShareRecordDAO(database).ListSelections(context.Background(), domain.ShareSelectionQuery{Cursor: true, SkipCount: true, PageSize: 100, AfterWork: "work", AfterSeason: 1, AfterEpisode: 2})
	if err != nil || len(rows) != 0 || total != 0 {
		t.Fatalf("rows=%v total=%d err=%v", rows, total, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionBatchAutoWithoutFallbackRollsBack(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	expectSelectionPreparation(mock, "work")
	mock.ExpectQuery(regexp.QuoteMeta(shareSelectionPrepareErrorsSQL)).WillReturnRows(sqlmock.NewRows([]string{"key", "mode"}).AddRow("w:1:2", "auto").AddRow("w:1:3", "auto"))
	mock.ExpectRollback()
	_, err := NewShareRecordDAO(database).ResolveSelections(context.Background(), "work")
	var invalid *ShareSelectionPreparationError
	if !errors.As(err, &invalid) || len(invalid.AutoKeys) != 2 || len(invalid.ManualKeys) != 0 {
		t.Fatalf("invalid=%+v err=%v", invalid, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
