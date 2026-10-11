package dao

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareBaselineAdoptionItemMappingIdentity(test *testing.T) {
	for _, scenario := range []struct {
		name     string
		accepted bool
	}{
		{"valid-item", true},
		{"valid-legacy-nonempty-key", true},
		{"valid-legacy-empty-key", true},
		{"legacy-row-item-payload", false},
		{"item-row-legacy-payload", false},
		{"unknown-consistent-id", false},
		{"wrong-item-id", false},
		{"wrong-item-row", false},
		{"wrong-item-payload", false},
		{"empty-key-item", false},
		{"blank-key-item", false},
		{"blank-key-legacy", false},
		{"wrong-share-code", false},
		{"wrong-password", false},
		{"wrong-file-path", false},
		{"wrong-file-name", false},
		{"wrong-file-id", false},
		{"missing-known-file-id", false},
		{"cached-file-id", true},
		{"path-only", true},
		{"cached-wrong-path", false},
		{"normalized-path-url-password", true},
		{"explicit-password", true},
		{"invalid-url", false},
		{"invalid-payload", false},
	} {
		for _, allowance := range []int64{0, 1} {
			name := scenario.name
			if allowance > 0 {
				name += "-with-allowance"
			}
			test.Run(name, func(test *testing.T) {
				database, mock, err := sqlmock.New()
				if err != nil {
					test.Fatal(err)
				}
				defer database.Close()
				source := domain.ShareStrmSource{ID: 11, MediaID: 7, URL: "https://115.com/s/selected?password=url-code", Password: "selected-code", FileName: "Show/episode.mkv", RemoteFileID: "selected-file"}
				switch scenario.name {
				case "cached-file-id", "path-only", "cached-wrong-path":
					source.RemoteFileID = ""
				case "normalized-path-url-password":
					source.FileName, source.Password, source.URL = `Show\unused\..\episode.mkv`, "", "https://115cdn.com/s/selected?password=%20url%2Bcode%20"
				case "explicit-password":
					source.Password = " selected-code "
				}
				exportKey := "work:1:1"
				legacy, err := domain.BuildShareStrmEntry(source)
				if err != nil {
					test.Fatal(err)
				}
				item, err := domain.BuildShareWorkReexportEntry(source, exportKey)
				if err != nil {
					test.Fatal(err)
				}
				otherItem, err := domain.BuildShareWorkReexportEntry(source, "work:1:2")
				if err != nil {
					test.Fatal(err)
				}
				entry, mappingID := item, item.ID
				switch scenario.name {
				case "valid-legacy-nonempty-key", "valid-legacy-empty-key", "blank-key-legacy":
					entry, mappingID = legacy, legacy.ID
				case "legacy-row-item-payload":
					mappingID = legacy.ID
				case "item-row-legacy-payload":
					entry = legacy
				case "unknown-consistent-id":
					mappingID, entry.ID = "9bd490fc-7797-4ff8-a47c-8cd5b0fdf2a0", "9bd490fc-7797-4ff8-a47c-8cd5b0fdf2a0"
				case "wrong-item-id":
					entry, mappingID = otherItem, otherItem.ID
				case "wrong-item-row":
					mappingID = otherItem.ID
				case "wrong-item-payload":
					entry = otherItem
				case "wrong-share-code":
					entry.ShareCode = "other-share"
				case "wrong-password":
					entry.Password = "other-password"
				case "wrong-file-path", "cached-wrong-path":
					entry.FilePath = "Other/episode.mkv"
				case "wrong-file-name":
					entry.FileName = "other.mkv"
				case "wrong-file-id":
					entry.FileID = "other-file"
				case "missing-known-file-id":
					entry.FileID = ""
				case "cached-file-id":
					entry.FileID = "cached-file"
				case "invalid-url":
					source.URL = "https://example.com/s/selected"
				}
				switch scenario.name {
				case "valid-legacy-empty-key", "empty-key-item":
					exportKey = ""
				case "blank-key-item", "blank-key-legacy":
					exportKey = " \t "
				}
				if allowance > 0 {
					entry.MediaID = 8
				}
				payload, err := json.Marshal(entry)
				if err != nil {
					test.Fatal(err)
				}
				if scenario.name == "invalid-payload" {
					payload = []byte(`{"media_id":"invalid"}`)
				}
				mock.ExpectBegin()
				mock.ExpectQuery(regexp.QuoteMeta(shareBaselineMappingsSQL)).WillReturnRows(sqlmock.NewRows([]string{"source_id", "file_name", "file_id", "media_id", "url", "password", "mapping_id", "payload", "export_key", "multiplicity"}).AddRow(source.ID, source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, mappingID, payload, exportKey, 1))
				mock.ExpectRollback()
				transaction, err := database.Begin()
				if err != nil {
					test.Fatal(err)
				}
				err = validateShareBaselineMappings(context.Background(), transaction, 1, allowance)
				if (err == nil) != scenario.accepted {
					test.Errorf("accepted=%v exportKey=%q error=%v", scenario.accepted, exportKey, err)
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
}

func TestShareBaselineAdoptionItemMappingMultiplicity(test *testing.T) {
	for _, scenario := range []struct {
		name                           string
		itemWeight, legacyWeight       int64
		count, allowance               int64
		itemDiff, legacyDiff, accepted bool
	}{
		{"item-multiple", 3, 0, 3, 0, false, false, true},
		{"item-zero", 0, 0, 1, 0, false, false, false},
		{"item-negative", -1, 0, 1, 0, false, false, false},
		{"item-duplicate", 2, 0, 1, 0, false, false, false},
		{"item-missing", 1, 0, 2, 0, false, false, false},
		{"item-overflow", 9223372036854775807, 0, 1, 0, false, false, false},
		{"item-strict-media", 3, 0, 3, 0, true, false, false},
		{"item-exact-allowance", 3, 0, 3, 3, true, false, true},
		{"item-under-allowance", 3, 0, 3, 2, true, false, false},
		{"item-over-allowance", 3, 0, 3, 4, true, false, false},
		{"item-unused-allowance", 3, 0, 3, 1, false, false, false},
		{"mixed-valid", 2, 3, 5, 0, false, false, true},
		{"mixed-item-difference", 2, 3, 5, 2, true, false, true},
		{"mixed-legacy-difference", 2, 3, 5, 3, false, true, true},
		{"mixed-both-difference", 2, 3, 5, 5, true, true, true},
		{"mixed-under-allowance", 2, 3, 5, 4, true, true, false},
		{"mixed-over-allowance", 2, 3, 5, 6, true, true, false},
		{"mixed-missing", 2, 3, 6, 0, false, false, false},
		{"mixed-extra", 2, 3, 4, 0, false, false, false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			source := domain.ShareStrmSource{ID: 11, MediaID: 7, URL: "https://115.com/s/selected", FileName: "Show/episode.mkv", RemoteFileID: "file"}
			item, err := domain.BuildShareWorkReexportEntry(source, "work:1:1")
			if err != nil {
				test.Fatal(err)
			}
			legacy, err := domain.BuildShareStrmEntry(source)
			if err != nil {
				test.Fatal(err)
			}
			if scenario.itemDiff {
				item.MediaID++
			}
			if scenario.legacyDiff {
				legacy.MediaID++
			}
			rows := sqlmock.NewRows([]string{"source_id", "file_name", "file_id", "media_id", "url", "password", "mapping_id", "payload", "export_key", "multiplicity"})
			for _, group := range []struct {
				entry  domain.ShareStrmEntry
				key    string
				weight int64
			}{{item, "work:1:1", scenario.itemWeight}, {legacy, "work:1:2", scenario.legacyWeight}} {
				if group.entry.ID == legacy.ID && group.weight == 0 {
					continue
				}
				payload, err := json.Marshal(group.entry)
				if err != nil {
					test.Fatal(err)
				}
				rows.AddRow(source.ID, source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, group.entry.ID, payload, group.key, group.weight)
			}
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(shareBaselineMappingsSQL)).WillReturnRows(rows)
			mock.ExpectRollback()
			transaction, err := database.Begin()
			if err != nil {
				test.Fatal(err)
			}
			err = validateShareBaselineMappings(context.Background(), transaction, scenario.count, scenario.allowance)
			if (err == nil) != scenario.accepted {
				test.Errorf("accepted=%v error=%v", scenario.accepted, err)
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
