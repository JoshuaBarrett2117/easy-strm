package dao

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCleanupReadOnlyScopesAndApplyGuard(t *testing.T) {
	for _, mode := range []string{"REBUILD"} {
		options := ShareCleanupOptions{Mode: mode, Scope: "all-share"}
		script, err := BuildShareCleanupSQL(options)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(script, "READ ONLY") || strings.Contains(script, "DELETE FROM") || strings.Contains(script, "t_share_export_recovery") {
			t.Fatal("unsafe dry run")
		}
		if strings.Contains(script, "FROM t_share_record WHERE") {
			t.Fatal("share configs wrong scope")
		}
		for _, guard := range []string{"owner_key='share:default'", "strm_config_id=-1", "operation='share_clear'", "manual_selections_to_destroy"} {
			if !strings.Contains(script, guard) {
				t.Fatal(guard)
			}
		}
		options.Apply = true
		if _, err = BuildShareCleanupSQL(options); err == nil {
			t.Fatal("unguarded apply")
		}
		options.Confirm = "DESTROY_SHARE_SELECTIONS_AND_DERIVED_RECORDS"
		options.ConfirmSelectionsDestroyed = true
		options.BackupAttestation = "fixture-backup"
		options.Quiesced = true

		script, err = BuildShareCleanupSQL(options)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(script, "pg_try_advisory_xact_lock") || !strings.Contains(script, "t_cron_task_run") || !strings.Contains(script, "IN EXCLUSIVE MODE") || !strings.HasSuffix(script, "COMMIT;\n") {
			t.Fatal("missing transaction or active-job guard")
		}
	}
	if _, err := BuildShareCleanupSQL(ShareCleanupOptions{Mode: "REBUILD"}); err == nil {
		t.Fatal("missing scope accepted")
	}
}

func TestCleanupMockCountsAndFailureAreReadOnly(t *testing.T) {
	for _, mode := range []string{"REBUILD"} {
		for _, failure := range []bool{false, true} {
			database, mock, _ := sqlmock.New()
			mock.ExpectBegin()
			for index, table := range ShareCleanupTables(mode) {
				expect := mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM " + table.Table + " WHERE " + table.Predicate))
				if failure && index == 2 {
					expect.WillReturnError(errors.New("count failed"))
					break
				}
				expect.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(index + 1))
			}
			if failure {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			counts, err := ReadShareCleanupCounts(context.Background(), database, ShareCleanupOptions{Mode: mode, Scope: "all-share"})
			if (err != nil) != failure {
				t.Fatalf("err=%v", err)
			}
			if !failure && len(counts) != len(ShareCleanupTables(mode)) {
				t.Fatal(counts)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			database.Close()
		}
	}
}
