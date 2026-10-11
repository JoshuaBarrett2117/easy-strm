package dao

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func adoptionRequest() domain.ShareBaselineAdoptionRequest {
	return domain.ShareBaselineAdoptionRequest{ManualConfirmation: "人工确认采纳", Reason: "独立核验后确认", DBAAttestation: "DBA-review-42", ExpectedOutputPath: "/isolated/output", ActorUserID: 1, Expected: map[string]int64{
		"config_revision": 3, "prepared_revision": 3, "completed_revision": 0,
		"selection_count": 2, "output_count": 2, "pending_count": 1,
		"dirty_revision_sum": 2, "selection_revision_sum": 2,
	}}
}

func TestShareBaselineAdoptionAtomicRollbackAndReadyNoAck(test *testing.T) {
	for _, scenario := range []string{"success", "ready-new-pending", "counts-changed", "inconsistent", "mapping-mismatch", "mapping-read-failed", "seed-failed", "ack-failed", "finish-failed", "audit-failed", "commit-failed", "producer-busy", "owner-busy", "selection-busy", "revision-changed", "ack-count-changed", "trust-zero", "finish-zero", "mirror-zero", "audit-zero", "fingerprint-changed", "protocol-changed", "required", "completed-changed", "ready-untrusted", "commit-guard-failed"} {
		test.Run(scenario, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			request := adoptionRequest()
			failure := errors.New("isolated injected failure")
			mock.ExpectBegin()
			for _, ancestor := range []string{"/", "/isolated"} {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1,34981))")).WithArgs(ancestor).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_xact_lock(hashtextextended($1,34981))")).WithArgs("/isolated/output").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
			mock.ExpectQuery(regexp.QuoteMeta(shareBaselineOwnerLockSQL)).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(scenario != "owner-busy"))
			proceed := scenario != "owner-busy"
			if proceed {
				mock.ExpectQuery(regexp.QuoteMeta(shareBaselineSelectionLockSQL)).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(scenario != "selection-busy"))
				proceed = scenario != "selection-busy"
			}
			if proceed {
				lock := mock.ExpectExec(regexp.QuoteMeta(shareBaselineTablesLockSQL))
				if scenario == "producer-busy" {
					lock.WillReturnError(failure)
					proceed = false
				} else {
					lock.WillReturnResult(sqlmock.NewResult(0, 0))
				}
			}
			if proceed {
				state, completed := "building", int64(0)
				fingerprint, protocol, reconciled := "fingerprint", 1, true
				if scenario == "fingerprint-changed" {
					fingerprint = "changed"
				}
				if scenario == "protocol-changed" {
					protocol = 2
				}
				if scenario == "required" {
					state = "required"
				}
				if scenario == "completed-changed" {
					completed = 1
				}
				if scenario == "ready-untrusted" {
					state, completed, reconciled = "ready", 3, false
				}
				if scenario == "ready-new-pending" {
					state, completed = "ready", 3
				}
				mock.ExpectQuery(regexp.QuoteMeta(shareBaselineConsumerSQL)).WillReturnRows(sqlmock.NewRows([]string{"revision", "prepared", "completed", "fingerprint", "state", "protocol", "reconciled"}).AddRow(3, 3, completed, fingerprint, state, protocol, reconciled))
				if scenario == "revision-changed" {
					request.Expected["config_revision"] = 2
				}
				proceed = scenario != "ready-new-pending" && scenario != "revision-changed" && scenario != "fingerprint-changed" && scenario != "protocol-changed" && scenario != "required" && scenario != "completed-changed" && scenario != "ready-untrusted"
			}
			if proceed {
				count, invalid := int64(2), int64(0)
				if scenario == "counts-changed" {
					count = 3
				}
				if scenario == "inconsistent" {
					invalid = 1
				}
				mock.ExpectQuery(regexp.QuoteMeta(shareBaselineCountsSQL)).WithArgs("/isolated/output").WillReturnRows(sqlmock.NewRows([]string{"selection", "output", "pending", "dirty_sum", "selection_sum", "invalid"}).AddRow(count, 2, 1, 2, 2, invalid))
				proceed = scenario != "counts-changed" && scenario != "inconsistent"
			}
			if proceed {
				source := domain.ShareStrmSource{ID: 11, MediaID: 7, URL: "https://115.com/s/selected", Password: "selected-code", FileName: "Show/episode.mkv", RemoteFileID: "selected-file"}
				entry, err := domain.BuildShareStrmEntry(source)
				if err != nil {
					test.Fatal(err)
				}
				if scenario == "mapping-mismatch" {
					entry.ShareCode, entry.FileID = "unselected", "unselected-file"
				}
				payload, err := json.Marshal(entry)
				if err != nil {
					test.Fatal(err)
				}
				query := mock.ExpectQuery(regexp.QuoteMeta(shareBaselineMappingsSQL))
				if scenario == "mapping-read-failed" {
					query.WillReturnError(failure)
				} else {
					rows := sqlmock.NewRows([]string{"source_id", "file_name", "file_id", "media_id", "url", "password", "mapping_id", "payload"})
					for index := 0; index < 2; index++ {
						rows.AddRow(source.ID, source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, entry.ID, payload)
					}
					query.WillReturnRows(rows)
				}
				proceed = scenario != "mapping-mismatch" && scenario != "mapping-read-failed"
			}
			if proceed {
				steps := []struct {
					query string
					args  []driver.Value
					count int64
					fails string
				}{
					{shareBaselineSourcesSQL, []driver.Value{"task"}, 2, "seed-failed"},
					{shareBaselineStaleSourcesSQL, []driver.Value{"task"}, 0, ""},
					{shareBaselineSelectionAckSQL, nil, 2, ""},
					{shareBaselineAckSQL, []driver.Value{"task"}, 1, "ack-failed"},
					{shareBaselineTrustSQL, nil, 1, ""},
					{shareExportFinishBaselineSQL, []driver.Value{int64(3)}, 1, "finish-failed"},
					{shareWatermarkMirrorSQL, []driver.Value{"task"}, 1, ""},
					{shareBaselineAuditSQL, []driver.Value{baselineAuditArgument{}}, 1, "audit-failed"},
				}
				for _, step := range steps {
					expectation := mock.ExpectExec(regexp.QuoteMeta(step.query)).WithArgs(step.args...)
					if scenario == step.fails {
						expectation.WillReturnError(failure)
						proceed = false
						break
					}
					zeroScenario := map[string]string{shareBaselineAckSQL: "ack-count-changed", shareBaselineTrustSQL: "trust-zero", shareExportFinishBaselineSQL: "finish-zero", shareWatermarkMirrorSQL: "mirror-zero", shareBaselineAuditSQL: "audit-zero"}
					if scenario == zeroScenario[step.query] {
						expectation.WillReturnResult(sqlmock.NewResult(0, 0))
						proceed = false
						break
					}
					expectation.WillReturnResult(sqlmock.NewResult(0, step.count))
				}
			}
			if proceed && scenario != "commit-guard-failed" {
				commit := mock.ExpectCommit()
				if scenario == "commit-failed" {
					commit.WillReturnError(failure)
				}
			} else {
				mock.ExpectRollback()
			}
			var guards []func() error
			if scenario == "commit-guard-failed" {
				guards = []func() error{func() error { return failure }}
			}
			result, err := NewShareExportCheckpointDAO(database).AdoptBaseline(context.Background(), request, "fingerprint", "task", guards...)
			if scenario == "success" || scenario == "ready-new-pending" {
				if err != nil || result.BaselineState != "ready" || result.CompletedRevision != 3 || result.AlreadyReady != (scenario == "ready-new-pending") {
					test.Fatalf("result=%+v error=%v", result, err)
				}
				if scenario == "ready-new-pending" && result.AcknowledgedWorks != 0 {
					test.Fatal("ready must never acknowledge new pending")
				}
			} else if err == nil || result.BaselineState != "" || result.AcknowledgedWorks != 0 {
				test.Fatalf("must reject/rollback without false success: %+v %v", result, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}
