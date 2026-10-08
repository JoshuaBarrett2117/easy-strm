package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareExportCheckpointMissingSchemaFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(false))
	if err = NewShareExportCheckpointDAO(db).CheckSchema(context.Background()); !errors.Is(err, ErrShareExportSchemaMissing) {
		t.Fatalf("missing schema: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareExportCheckpointPrepareFanoutAndFingerprintAtomic(t *testing.T) {
	for _, failure := range []string{"none", "revision", "fanout", "fingerprint", "commit", "resume"} {
		t.Run(failure, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			revision, prepared := int64(2), int64(0)
			fingerprint, state := "old", "required"
			if failure == "revision" {
				revision = 3
			}
			if failure == "resume" {
				prepared = 2
				fingerprint = "new"
				state = "building"
			}
			mock.ExpectQuery("SELECT config_revision,prepared_revision.*legacy_outputs_reconciled.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision", "prepared", "fingerprint", "state", "protocol", "reconciled"}).AddRow(revision, prepared, fingerprint, state, 1, true))
			if failure != "revision" && failure != "resume" {
				fanout := mock.ExpectExec(regexp.QuoteMeta(shareExportFanoutSQL))
				if failure == "fanout" {
					fanout.WillReturnError(errors.New("fanout failure"))
				} else {
					fanout.WillReturnResult(sqlmock.NewResult(0, 1))
				}
				if failure != "fanout" {
					update := mock.ExpectExec("UPDATE t_share_export_consumer SET config_fingerprint").WithArgs("new")
					if failure == "fingerprint" {
						update.WillReturnError(errors.New("fingerprint failure"))
					} else {
						update.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if failure == "none" || failure == "resume" {
				mock.ExpectCommit()
			} else if failure == "commit" {
				mock.ExpectCommit().WillReturnError(errors.New("commit failure"))
			} else {
				mock.ExpectRollback()
			}
			err = NewShareExportCheckpointDAO(db).Prepare(context.Background(), domain.ShareExportInput{ConfigRevision: 2}, "new")
			if (failure == "none" || failure == "resume") != (err == nil) {
				t.Fatalf("%s: %v", failure, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareExportCheckpointCompleteCASRollbackAndProtectedStale(t *testing.T) {
	for _, scenario := range []string{"config-change", "work-bump", "already-acked", "ack-zero", "stale-race", "protected", "success", "commit-failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			config := int64(1)
			if scenario == "config-change" {
				config = 2
			}
			mock.ExpectQuery("SELECT config_revision.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(config))
			if scenario != "config-change" {
				revision, acked := int64(7), int64(6)
				if scenario == "work-bump" {
					revision = 8
				}
				if scenario == "already-acked" {
					acked = 7
				}
				mock.ExpectQuery("SELECT revision,acked_revision.*FOR UPDATE").WithArgs("work").WillReturnRows(sqlmock.NewRows([]string{"revision", "acked"}).AddRow(revision, acked))
				if scenario != "work-bump" && scenario != "already-acked" {
					mock.ExpectExec("UPDATE t_share_export_source_state SET state='stale'").WithArgs("work", "{}", "run").WillReturnResult(sqlmock.NewResult(0, 1))
					count := int64(1)
					if scenario == "protected" || scenario == "stale-race" {
						count = 0
					}
					mock.ExpectExec(regexp.QuoteMeta(shareExportStaleSQL)).WithArgs("work", "old-key", "old-run").WillReturnResult(sqlmock.NewResult(0, count))
					if count == 0 {
						mock.ExpectQuery("SELECT EXISTS.*export_keys @>").WithArgs("work", "old-key").WillReturnRows(sqlmock.NewRows([]string{"protected"}).AddRow(scenario == "protected"))
					}
					if scenario != "stale-race" {
						ackCount := int64(1)
						if scenario == "ack-zero" {
							ackCount = 0
						}
						mock.ExpectExec(regexp.QuoteMeta(shareExportAckSQL)).WithArgs("work", int64(7), "run").WillReturnResult(sqlmock.NewResult(0, ackCount))
					}
				}
			}
			if scenario == "success" || scenario == "protected" {
				mock.ExpectCommit()
			} else if scenario == "commit-failure" {
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			} else {
				mock.ExpectRollback()
			}
			_, err = NewShareExportCheckpointDAO(db).CompleteWork(context.Background(), domain.ShareExportDirty{WorkKey: "work", Revision: 7}, 1, "run", nil, ExportSnapshot{"old-key": "old-run"})
			if (scenario == "success" || scenario == "protected") != (err == nil) {
				t.Fatalf("%s: %v", scenario, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareExportCheckpointDirtyRowsAndObservationErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dao := NewShareExportCheckpointDAO(db)
	mock.ExpectQuery(regexp.QuoteMeta(shareExportDirtySQL)).WithArgs("{}").WillReturnRows(sqlmock.NewRows([]string{"work", "revision", "pending"}).AddRow("work", 7, `["old","planned"]`))
	rows, err := dao.DirtyBatch(context.Background(), []string{})
	if err != nil || len(rows) != 1 || len(rows[0].PendingKeys) != 2 {
		t.Fatalf("%+v %v", rows, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(shareExportObservedSQL)).WithArgs("work").WillReturnRows(sqlmock.NewRows([]string{"file", "share", "media", "work", "fileversion", "shareversion", "available", "cancelled", "episodes", "state"}).AddRow(1, 2, 3, "work", 4, 5, false, true, "1:1,2:2", "stale"))
	states, err := dao.ObserveWork(context.Background(), "work")
	if err != nil || len(states) != 1 || states[0].State != "stale" || !states[0].Cancelled {
		t.Fatalf("%+v %v", states, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(shareExportDirtySQL)).WithArgs("{}").WillReturnRows(sqlmock.NewRows([]string{"work", "revision", "pending"}).AddRow("work", 7, `[42]`))
	if _, err = dao.DirtyBatch(context.Background(), []string{}); err == nil {
		t.Fatal("非字符串 pending 未被拒绝")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareExportCheckpointPlannedKeysCASAndUnion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dao := NewShareExportCheckpointDAO(db)
	for _, changed := range []int64{1, 0} {
		mock.ExpectExec(regexp.QuoteMeta(shareExportPlannedSQL)).WithArgs("work", int64(7), `["a","b"]`).WillReturnResult(sqlmock.NewResult(0, changed))
		err = dao.RecordPlannedKeys(context.Background(), "work", 7, []string{"b", "a", "b"})
		if changed == 0 && !errors.Is(err, ErrShareExportChanged) {
			t.Fatalf("CAS: %v", err)
		}
		if changed == 1 && err != nil {
			t.Fatal(err)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareExportTargetedStrmSourcesAndLockedFileRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewShareRecordDAO(db)
	columns := []string{"id", "share", "name", "work", "url", "password", "file", "remote", "result", "episodes", "remaining"}
	mock.ExpectQuery(regexp.QuoteMeta(shareStrmWorkSourcesSQL)).WithArgs("work", 0).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, 2, "name", "work", "url", "", "a.mkv", "remote", `{"_media_id":3,"_file_version":4,"_share_version":5}`, `[]`, 1))
	rows, err := store.StrmWorkSources(context.Background(), "work", 0)
	if err != nil || len(rows) != 1 || rows[0].FileVersion != 4 {
		t.Fatalf("%+v %v", rows, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(shareStrmFileSourceSQL)).WithArgs(1).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, 2, "name", "work", "url", "", "a.mkv", "remote", `{"_media_id":3}`, `[]`, 1))
	if _, err = store.GetStrmSource(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
