package dao

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestShareBaselineAdoptionSelectedMappingIdentity(test *testing.T) {
	for _, scenario := range []string{"selected", "unselected-same-media", "wrong-share-code", "wrong-file-id", "missing-known-file-id", "wrong-file-path", "wrong-password", "wrong-mapping-id", "wrong-payload-id", "wrong-file-name", "wrong-media-id", "cached-file-id", "cached-wrong-path", "path-only", "normalized-path-url-password", "explicit-password", "invalid-url", "invalid-payload", "query-failed", "scan-failed", "rows-failed", "missing-row"} {
		test.Run(scenario, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			source := domain.ShareStrmSource{ID: 11, MediaID: 7, URL: "https://115.com/s/selected?password=url-code", Password: "selected-code", FileName: "Show/episode.mkv", RemoteFileID: "selected-file"}
			mappingID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("selected:path:Show/episode.mkv")).String()
			payload := map[string]interface{}{"id": mappingID, "share_code": "selected", "password": "selected-code", "file_id": "selected-file", "file_path": "Show/episode.mkv", "file_name": "episode.mkv", "media_id": 7}
			valid := false
			switch scenario {
			case "selected":
				valid = true
			case "unselected-same-media":
				payload["share_code"], payload["password"], payload["file_id"], payload["file_path"] = "unselected", "unselected-code", "unselected-file", "Other/episode.mkv"
				mappingID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("unselected:path:Other/episode.mkv")).String()
				payload["id"] = mappingID
			case "wrong-share-code":
				payload["share_code"] = "unselected"
			case "wrong-file-id":
				payload["file_id"] = "unselected-file"
			case "missing-known-file-id":
				payload["file_id"] = ""
			case "wrong-file-path":
				payload["file_path"] = "Other/episode.mkv"
			case "wrong-password":
				payload["password"] = "old-code"
			case "wrong-mapping-id":
				mappingID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("unselected:path:Show/episode.mkv")).String()
			case "wrong-payload-id":
				payload["id"] = "other-id"
			case "wrong-file-name":
				payload["file_name"] = "other.mkv"
			case "wrong-media-id":
				payload["media_id"] = 8
			case "cached-file-id", "path-only":
				source.RemoteFileID = ""
				if scenario == "path-only" {
					payload["file_id"] = ""
				}
				valid = true
			case "cached-wrong-path":
				source.RemoteFileID, payload["file_path"] = "", "Other/episode.mkv"
			case "normalized-path-url-password":
				source.FileName, source.Password, source.URL = `Show\unused\..\episode.mkv`, "", "https://115cdn.com/s/selected?password=%20url%2Bcode%20"
				payload["password"] = "url+code"
				valid = true
			case "explicit-password":
				source.Password, payload["password"] = " selected-code ", " selected-code "
				valid = true
			case "invalid-url":
				source.URL = "https://example.com/s/selected"
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				test.Fatal(err)
			}
			if scenario == "invalid-payload" {
				raw = []byte(`{"media_id":"invalid"}`)
			}
			request := adoptionRequest()
			request.Expected["selection_count"], request.Expected["output_count"] = 1, 1
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(shareBaselineCountsSQL)).WithArgs(request.ExpectedOutputPath).WillReturnRows(sqlmock.NewRows([]string{"selection", "output", "pending", "dirty_sum", "selection_sum", "invalid"}).AddRow(1, 1, 1, 2, 2, 0))
			query := mock.ExpectQuery(regexp.QuoteMeta(shareBaselineMappingsSQL))
			failure := errors.New("isolated mapping read failure")
			if scenario == "query-failed" {
				query.WillReturnError(failure)
			} else {
				rows := sqlmock.NewRows([]string{"source_id", "file_name", "file_id", "media_id", "url", "password", "mapping_id", "payload", "multiplicity"})
				if scenario == "scan-failed" {
					rows.AddRow("invalid-source-id", source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, mappingID, raw, 1)
				} else if scenario != "missing-row" {
					rows.AddRow(source.ID, source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, mappingID, raw, 1)
				}
				if scenario == "rows-failed" {
					rows.RowError(0, failure)
				}
				query.WillReturnRows(rows)
			}
			mock.ExpectRollback()
			transaction, err := database.Begin()
			if err != nil {
				test.Fatal(err)
			}
			result := domain.ShareBaselineAdoptionResult{}
			err = validateShareBaselineCounts(context.Background(), transaction, request, &result)
			if valid && err != nil {
				test.Errorf("selected source mapping rejected: %v", err)
			}
			if !valid && err == nil {
				test.Error("mapping inconsistency or read failure accepted despite matching media and counts")
			}
			if !valid && result.AcknowledgedWorks != 0 {
				test.Error("mapping failure must precede acknowledgement")
			}
			if (scenario == "query-failed" || scenario == "rows-failed") && !errors.Is(err, failure) {
				test.Errorf("mapping read error lost: %v", err)
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
