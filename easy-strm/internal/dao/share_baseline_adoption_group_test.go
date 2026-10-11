package dao

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareBaselineMappingGroupMultiplicity(test *testing.T) {
	for _, scenario := range []struct {
		name                   string
		weight, count, allowed int64
		different, accepted    bool
	}{
		{"multi-episode", 3, 3, 0, false, true},
		{"duplicate-members", 2, 1, 0, false, false},
		{"missing-members", 1, 2, 0, false, false},
		{"zero", 0, 1, 0, false, false},
		{"negative", -1, 1, 0, false, false},
		{"overflow", 9223372036854775807, 1, 0, false, false},
		{"exact-benign", 3, 3, 3, true, true},
		{"benign-under", 3, 3, 2, true, false},
		{"benign-over", 3, 3, 4, true, false},
		{"strict-media", 3, 3, 0, true, false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				test.Fatal(err)
			}
			defer database.Close()
			source := domain.ShareStrmSource{ID: 11, MediaID: 7, URL: "https://115.com/s/selected", FileName: "Show/episode.mkv", RemoteFileID: "file"}
			entry, err := domain.BuildShareStrmEntry(source)
			if err != nil {
				test.Fatal(err)
			}
			if scenario.different {
				entry.MediaID = 8
			}
			payload, err := json.Marshal(entry)
			if err != nil {
				test.Fatal(err)
			}
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta(shareBaselineMappingsSQL)).WillReturnRows(sqlmock.NewRows([]string{"source", "name", "file", "media", "url", "password", "mapping", "payload", "export_key", "multiplicity"}).AddRow(source.ID, source.FileName, source.RemoteFileID, source.MediaID, source.URL, source.Password, entry.ID, payload, "work:1:1", scenario.weight))
			mock.ExpectRollback()
			transaction, err := database.Begin()
			if err != nil {
				test.Fatal(err)
			}
			err = validateShareBaselineMappings(context.Background(), transaction, scenario.count, scenario.allowed)
			_ = transaction.Rollback()
			if (err == nil) != scenario.accepted {
				test.Fatalf("accepted=%v error=%v", scenario.accepted, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareBaselineSetQueryStructure(test *testing.T) {
	if strings.Replace(shareBaselineLivePredicate, "e.file_id IS NOT NULL", shareBaselineEpisodeExistsSQL, 1) != shareSelectionLivePredicate {
		test.Fatal("baseline live rules differ from the shared live predicate")
	}
	if !strings.Contains(shareBaselineLiveRowsSQL, "f.id source_id,live.id") {
		test.Fatal("file identity must not replace the actual live media ID")
	}
	for _, fragment := range []string{"owned_outputs AS MATERIALIZED", "ON mapping.id=CASE WHEN", "THEN output.share_strm_id::uuid END", "AND mapping.id::text=output.share_strm_id"} {
		if !strings.Contains(shareBaselineOwnedOutputsSQL, fragment) {
			test.Fatalf("mapping lookup must be materialized and indexable without changing text identity: %s", fragment)
		}
	}
	for _, query := range []string{shareBaselineCountsSQL, shareBaselineBenignCountsSQL, shareBaselineMappingsSQL, shareBaselineSourcesSQL} {
		if strings.Contains(query, "LATERAL") || strings.Contains(query, "NOT EXISTS") || strings.Contains(query, "EXISTS(SELECT") || !strings.Contains(query, "AS MATERIALIZED") {
			test.Fatal("baseline hot path retains correlated membership or aggregation")
		}
	}
	if strings.Contains(shareBaselineMappingsSQL, "SELECT DISTINCT live.source_id") {
		test.Fatal("mapping validation must preserve every source row through multiplicity")
	}
}
