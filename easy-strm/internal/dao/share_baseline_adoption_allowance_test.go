package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func baselineAllowanceRequest(test *testing.T, requested int, attestation string) domain.ShareBaselineAdoptionRequest {
	test.Helper()
	request := adoptionRequest()
	payload, err := json.Marshal(map[string]interface{}{"allow_known_benign_invalid": requested, "benign_attestation": attestation})
	if err != nil {
		test.Fatal(err)
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		test.Fatal(err)
	}
	return request
}

func baselineAllowanceRecheckSQL() string {
	return strings.Replace(shareBaselineCountsSQL, " AND (mapping.payload->>'media_id')::integer=live.id", "", 1)
}

func TestShareBaselineAdoptionBenignAllowanceCounts(test *testing.T) {
	for _, scenario := range []struct {
		name              string
		requested         int
		invalid           int64
		attestation       string
		recheck, accepted bool
	}{
		{"missing", 0, 68, "", false, false},
		{"negative", -1, 68, "review", false, false},
		{"zero", 0, 68, "review", false, false},
		{"under", 67, 68, "review", false, false},
		{"over", 69, 68, "review", false, false},
		{"blank", 68, 68, "", false, false},
		{"whitespace", 68, 68, " \t\n ", false, false},
		{"over-cap", 201, 201, "review", false, false},
		{"boundary", 200, 200, "review", true, true},
		{"exact", 68, 68, "  review\n原始核验  ", true, true},
		{"no-invalid", 68, 0, "review", false, false},
		{"snapshot", 68, 68, "review", false, false},
		{"unequal-output", 68, 68, "review", false, false},
		{"zero-selection", 68, 68, "review", false, false},
		{"path-invalid", 68, 68, "review", true, false},
		{"revision-invalid", 68, 68, "review", true, false},
		{"unknown-media", 68, 68, "review", true, false},
		{"recheck-error", 68, 68, "review", true, false},
		{"recheck-scan-error", 68, 68, "review", true, false},
		{"recheck-snapshot", 68, 68, "review", true, false},
		{"difference-under", 68, 68, "review", true, false},
		{"difference-over", 68, 68, "review", true, false},
		{"wrong-file-id", 68, 68, "review", true, false},
		{"missing-file-id", 68, 68, "review", true, false},
		{"wrong-path", 68, 68, "review", true, false},
		{"wrong-share", 68, 68, "review", true, false},
		{"wrong-password", 68, 68, "review", true, false},
		{"wrong-name", 68, 68, "review", true, false},
		{"wrong-id", 68, 68, "review", true, false},
		{"wrong-mapping-id", 68, 68, "review", true, false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			request := baselineAllowanceRequest(test, scenario.requested, scenario.attestation)
			if scenario.name == "missing" {
				request = adoptionRequest()
			}
			selection, output := int64(210), int64(210)
			if scenario.name == "unequal-output" {
				output--
			}
			if scenario.name == "zero-selection" {
				selection, output = 0, 0
			}
			request.Expected["selection_count"], request.Expected["output_count"], request.Expected["selection_revision_sum"] = selection, output, selection
			if scenario.name == "snapshot" {
				request.Expected["dirty_revision_sum"]++
			}
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(shareBaselineCountsSQL)).WithArgs(request.ExpectedOutputPath).WillReturnRows(sqlmock.NewRows([]string{"selection", "output", "pending", "dirty_sum", "selection_sum", "invalid"}).AddRow(selection, output, 1, 2, selection, scenario.invalid))
			failure := errors.New("mock strict recheck failure")
			remaining := int64(0)
			if scenario.recheck {
				query := mock.ExpectQuery(regexp.QuoteMeta(baselineAllowanceRecheckSQL())).WithArgs(request.ExpectedOutputPath)
				switch scenario.name {
				case "path-invalid", "revision-invalid", "unknown-media":
					remaining = 1
				case "recheck-error":
					query.WillReturnError(failure)
				case "recheck-scan-error":
					query.WillReturnRows(sqlmock.NewRows([]string{"selection", "output", "pending", "dirty_sum", "selection_sum", "invalid"}).AddRow(selection, output, 1, 2, selection, "invalid"))
				case "recheck-snapshot":
					query.WillReturnRows(sqlmock.NewRows([]string{"selection", "output", "pending", "dirty_sum", "selection_sum", "invalid"}).AddRow(selection, output, 1, 3, selection, 0))
				}
				if !strings.HasPrefix(scenario.name, "recheck-") {
					query.WillReturnRows(sqlmock.NewRows([]string{"selection", "output", "pending", "dirty_sum", "selection_sum", "invalid"}).AddRow(selection, output, 1, 2, selection, remaining))
				}
				if remaining == 0 && !strings.HasPrefix(scenario.name, "recheck-") {
					rows := sqlmock.NewRows([]string{"source_id", "file_name", "file_id", "media_id", "url", "password", "mapping_id", "payload"})
					differences := scenario.requested
					if scenario.name == "difference-under" {
						differences--
					}
					if scenario.name == "difference-over" {
						differences++
					}
					for index := 0; index < int(selection); index++ {
						source := domain.ShareStrmSource{ID: index + 1, MediaID: 7, URL: "https://115.com/s/selected", Password: "code", FileName: fmt.Sprintf("Show/episode-%d.mkv", index), RemoteFileID: fmt.Sprintf("file-%d", index)}
						entry, err := domain.BuildShareStrmEntry(source)
						if err != nil {
							test.Fatal(err)
						}
						mappingID := entry.ID
						if index < differences {
							entry.MediaID = 8
						}
						if index == 0 {
							switch scenario.name {
							case "wrong-file-id":
								entry.FileID = "other-file"
							case "missing-file-id":
								entry.FileID = ""
							case "wrong-path":
								entry.FilePath = "Other/episode.mkv"
							case "wrong-share":
								entry.ShareCode = "other-share"
							case "wrong-password":
								entry.Password = "other-code"
							case "wrong-name":
								entry.FileName = "other.mkv"
							case "wrong-id":
								entry.ID = "other-id"
							case "wrong-mapping-id":
								mappingID = "other-id"
							}
						}
						payload, err := json.Marshal(entry)
						if err != nil {
							test.Fatal(err)
						}
						rows.AddRow(source.ID, source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, mappingID, payload)
					}
					mock.ExpectQuery(regexp.QuoteMeta(shareBaselineMappingsSQL)).WillReturnRows(rows)
				}
			}
			mock.ExpectRollback()
			transaction, err := database.Begin()
			if err != nil {
				test.Fatal(err)
			}
			result := domain.ShareBaselineAdoptionResult{}
			err = validateShareBaselineCounts(context.Background(), transaction, request, &result)
			if (err == nil) != scenario.accepted {
				test.Errorf("accepted=%v result=%+v error=%v", scenario.accepted, result, err)
			}
			if !scenario.recheck && scenario.name != "snapshot" {
				expectedError := fmt.Sprintf("采纳一致性校验失败：selection=%d output=%d invalid=%d", selection, output, scenario.invalid)
				if err == nil || err.Error() != expectedError {
					test.Errorf("consistency rejection changed: %v", err)
				}
			}
			if remaining != 0 && (err == nil || err.Error() != "采纳良性放行校验失败：仍有非 MediaID 差异 invalid=1") {
				test.Errorf("non-MediaID invalid must remain strict: %v", err)
			}
			if !scenario.accepted && result.AcknowledgedWorks != 0 {
				test.Error("failure acknowledged pending work")
			}
			if scenario.accepted && result.AcknowledgedWorks != 1 {
				test.Error("success did not preserve acknowledgement count")
			}
			if scenario.name == "recheck-error" && !errors.Is(err, failure) {
				test.Errorf("strict recheck error lost: %v", err)
			}
			if scenario.name == "recheck-snapshot" && !errors.Is(err, ErrShareExportChanged) {
				test.Errorf("strict recheck snapshot guard lost: %v", err)
			}
			if err := transaction.Rollback(); err != nil {
				test.Error(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Error(err)
			}
		})
	}
}
