package dao

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func selection115Rows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "config", "filename", "relative", "size", "local", "cloud", "root", "metadata"})
}

func TestSelection115DiscoveryExactOrderingAndReadonly(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	root := t.TempDir()
	local := filepath.Join(root, "original.strm")
	if err := os.WriteFile(local, []byte("original directory bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(local)
	relative := `日番\恶作剧之吻 (2008) [tmdbid=36005]\Season 1\恶作剧之吻.2008.S01E01.mkv`
	rows := selection115Rows().
		AddRow(1, -1, "own", relative, 10000, local, 9, "/源", true).
		AddRow(2, 0, "large.mkv", relative, 9000, strings.ReplaceAll(local, "/", `\`), 9, "/源", true).
		AddRow(3, 1, "small.mkv", relative, 9000, filepath.Join(root, "missing.strm"), 10, "/源", true).
		AddRow(4, 1, "wrong.mkv", strings.ReplaceAll(relative, "36005", "360050"), 8000, local, 9, "/源", true).
		AddRow(5, 1, "directory.mkv", relative, 7000, root, 9, "/源", true)
	mock.ExpectQuery(regexp.QuoteMeta(selection115WorkSQL)).WithArgs("[tmdbid=36005]", "{tmdb-36005}", "tmdb:tv:36005").WillReturnRows(rows)
	values, err := selection115Work(context.Background(), database, "tmdb:tv:36005")
	if err != nil || len(values) != 3 || values[0].candidate.ExternalRef != "0:2" || !values[0].candidate.Available || values[1].candidate.Available || values[2].candidate.Available {
		t.Fatalf("%+v %v", values, err)
	}
	if values[0].candidate.CloudPath != "/源/"+domain.NormalizeStrm115Path(relative) {
		t.Fatal(values[0])
	}
	after, _ := os.Stat(local)
	content, _ := os.ReadFile(local)
	if string(content) != "original directory bytes" || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("directory metadata changed")
	}
	if !strings.Contains(selection115WorkSQL, "f.strm_config_id>=0") || !strings.Contains(selection115RefSQL, "f.strm_config_id>=0") || !strings.Contains(selection115WorkSQL, "ORDER BY f.file_size DESC NULLS LAST,f.id ASC") || !strings.Contains(selection115From, "JOIN t_strm_config") {
		t.Fatal("query guards")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelection115IndependentSourcesScopedAndMissingMetadata(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "failed", "untitled", "wrong-type"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			var media interface{} = int64(7)
			metadata := []byte(`{"success":true,"title":"剧","media_type":"tv"}`)
			switch scenario {
			case "missing":
				media, metadata = nil, nil
			case "failed":
				metadata = []byte(`{"success":false,"title":"剧","media_type":"tv"}`)
			case "untitled":
				metadata = []byte(`{"success":true,"title":" ","media_type":"tv"}`)
			case "wrong-type":
				metadata = []byte(`{"success":true,"title":"剧","media_type":"movie"}`)
			}
			mock.ExpectQuery(`SELECT s.media_item_key,.*s.source_kind='strm115'.*title ILIKE.*s.media_item_key>\$2 ORDER BY s.media_item_key LIMIT 100`).WithArgs("剧", "").WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "mode", "revision", "exported", "path", "ref", "media", "result"}).AddRow("tmdb:tv:1:1:2", "tmdb:tv:1", 1, 2, "manual", 3, 2, "fixed.strm", "0:5", media, metadata))
			if scenario == "valid" {
				local := filepath.Join(t.TempDir(), "source.strm")
				if err = os.WriteFile(local, []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
				mock.ExpectQuery(regexp.QuoteMeta(selection115RefSQL)).WithArgs(int64(0), int64(5), "tmdb:tv:1").WillReturnRows(selection115Rows().AddRow(5, 0, "剧.S01E02.mkv", `剧 [tmdbid=1]\Season 1\剧.S01E02.mkv`, nil, local, 9, "/root", true))
			}
			values, err := NewShareRecordDAO(database).Strm115SelectionSources(context.Background(), domain.ShareLibraryQuery{Keyword: "剧"}, "")
			if scenario == "valid" {
				if err != nil || len(values) != 1 || values[0].Source.ID != 0 || values[0].Source.ShareID != 0 || values[0].Source.MediaID != 7 || values[0].Source.Result.EpisodeNumber != 2 || values[0].Selection.SourceKind != "strm115" || values[0].Selection.CandidateID != 0 {
					t.Fatalf("%+v %v", values, err)
				}
			} else if !errors.Is(err, ErrShareSelectionUnavailable) || len(values) != 0 {
				t.Fatalf("missing metadata must fail closed: %+v %v", values, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelection115NullableResolveNoFallback(t *testing.T) {
	for _, scenario := range []string{"valid", "absent", "wrong-item", "unavailable", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			local := filepath.Join(t.TempDir(), "source.strm")
			if err := os.WriteFile(local, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			ref := "0:5"
			if scenario == "malformed" {
				ref = "-1:5"
			}
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT s.media_item_key").WithArgs("tmdb:tv:1:1:2").WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "kind", "ref", "title", "valid"}).AddRow("tmdb:tv:1:1:2", "tmdb:tv:1", 1, 2, nil, "auto", 3, 2, "fixed.strm", "strm115", ref, "剧", false))
			if scenario != "malformed" {
				rows := selection115Rows()
				if scenario != "absent" {
					filename := `剧 [tmdbid=1]\Season 1\剧.S01E02.mkv`
					if scenario == "wrong-item" {
						filename = strings.ReplaceAll(filename, "S01E02", "S01E03")
					}
					if scenario == "unavailable" {
						local = filepath.Join(t.TempDir(), "missing.strm")
					}
					rows.AddRow(5, 0, "file.mkv", filename, 100, local, 9, "/root", true)
				}
				mock.ExpectQuery(regexp.QuoteMeta(selection115RefSQL)).WithArgs(int64(0), int64(5), "tmdb:tv:1").WillReturnRows(rows)
			}
			if scenario == "valid" {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			selection, source, err := NewShareRecordDAO(database).ResolveSelection(context.Background(), "tmdb:tv:1:1:2")
			if (err == nil) != (scenario == "valid") || source != 0 || selection.CandidateID != 0 || selection.RelativePath != "fixed.strm" {
				t.Fatalf("%+v %d %v", selection, source, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelection115CASAndNullableUpdate(t *testing.T) {
	for _, scenario := range []string{"valid", "stale", "unavailable", "dirty-error"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			local := filepath.Join(t.TempDir(), "source.strm")
			if err := os.WriteFile(local, []byte("source"), 0600); err != nil {
				t.Fatal(err)
			}
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			revision := 3
			if scenario == "stale" {
				revision = 4
			}
			mock.ExpectQuery("SELECT selection_revision,work_key").WillReturnRows(sqlmock.NewRows([]string{"rev", "work"}).AddRow(revision, "tmdb:tv:1"))
			if scenario != "stale" {
				mock.ExpectQuery("SELECT s.media_item_key").WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "candidate", "mode", "revision", "exported", "path", "kind", "ref", "title", "valid"}).AddRow("tmdb:tv:1:1:2", "tmdb:tv:1", 1, 2, 12, "manual", 3, 2, "fixed.strm", "share", "", "剧", true))
				if scenario == "unavailable" {
					local = ""
				}
				mock.ExpectQuery(regexp.QuoteMeta(selection115RefSQL)).WithArgs(int64(0), int64(5), "tmdb:tv:1").WillReturnRows(selection115Rows().AddRow(5, 0, "file.mkv", `剧 [tmdbid=1]\Season 1\剧.S01E02.mkv`, 100, local, 9, "/root", true))
				if scenario != "unavailable" {
					mock.ExpectExec("UPDATE t_share_media_selection SET source_kind='strm115',external_ref=\\$2,selected_candidate_id=NULL").WithArgs("tmdb:tv:1:1:2", "0:5", int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
					enqueue := mock.ExpectExec("SELECT share_export_enqueue").WithArgs("tmdb:tv:1")
					if scenario == "dirty-error" {
						enqueue.WillReturnError(errors.New("dirty failure"))
					} else {
						enqueue.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
			if scenario == "valid" {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			_, err := NewShareRecordDAO(database).ChangeSelection(context.Background(), domain.ShareSelectionChange{ItemKey: "tmdb:tv:1:1:2", SourceKind: "strm115", ExternalRef: "0:5", ExpectedRevision: 3})
			if (err == nil) != (scenario == "valid") {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelection115UnavailableMetadataAndPathRowsRemainVisible(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	local := filepath.Join(t.TempDir(), "source.strm")
	if err := os.WriteFile(local, []byte("readonly"), 0600); err != nil {
		t.Fatal(err)
	}
	relative := `剧 [tmdbid=1]\Season 1\剧.S01E02.mkv`
	rows := selection115Rows().
		AddRow(1, 0, "leading.mkv", `\`+relative, 100, local, 9, "/root", true).
		AddRow(2, 0, "metadata-missing.mkv", relative, 90, local, 9, "/root", false).
		AddRow(3, 0, "cloud-missing.mkv", relative, nil, local, nil, "/root", true).
		AddRow(4, 0, "root-invalid.mkv", relative, 80, local, 9, "relative-root", true).
		AddRow(5, 0, "network.mkv", `\\`+relative, 70, local, 9, "/root", true).
		AddRow(6, 0, "traversal.mkv", `../`+relative, 60, local, 9, "/root", true)
	mock.ExpectQuery(regexp.QuoteMeta(selection115WorkSQL)).WithArgs("[tmdbid=1]", "{tmdb-1}", "tmdb:tv:1").WillReturnRows(rows)
	values, err := selection115Work(context.Background(), database, "tmdb:tv:1")
	if err != nil || len(values) != 6 || !values[0].candidate.Available {
		t.Fatalf("%+v %v", values, err)
	}
	for _, value := range values[1:] {
		if value.candidate.Available || value.candidate.State != "unavailable" {
			t.Fatal(value)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelection115PreflightCachesPerWorkAndRejectsUnknown(t *testing.T) {
	for _, kind := range []string{"strm115", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			local := filepath.Join(t.TempDir(), "source.strm")
			if err := os.WriteFile(local, []byte("readonly"), 0600); err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery(regexp.QuoteMeta(selection115PreflightSQL)).WithArgs("tmdb:tv:1").WillReturnRows(sqlmock.NewRows([]string{"key", "work", "season", "episode", "ref", "kind"}).AddRow("tmdb:tv:1:1:1", "tmdb:tv:1", 1, 1, "0:1", kind).AddRow("tmdb:tv:1:1:2", "tmdb:tv:1", 1, 2, "0:2", kind))
			if kind == "strm115" {
				mock.ExpectQuery(regexp.QuoteMeta(selection115WorkSQL)).WithArgs("[tmdbid=1]", "{tmdb-1}", "tmdb:tv:1").WillReturnRows(selection115Rows().AddRow(1, 0, "one.mkv", `剧 [tmdbid=1]/Season 1/剧.S01E01.mkv`, 100, local, 9, "/root", true).AddRow(2, 0, "two.mkv", `剧 [tmdbid=1]/Season 1/剧.S01E02.mkv`, 100, local, 9, "/root", true))
			}
			err := preflightSelection115(context.Background(), database, "tmdb:tv:1")
			if (err == nil) != (kind == "strm115") {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelection115ReceiptCASRejectsChangedRefAndRevision(t *testing.T) {
	for _, scenario := range []string{"success", "revision", "ref", "kind"} {
		t.Run(scenario, func(t *testing.T) {
			database, mock, _ := sqlmock.New()
			defer database.Close()
			connection, err := database.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			selection := domain.ShareSelection{ItemKey: "tmdb:tv:1:1:2", SourceKind: "strm115", ExternalRef: "0:5", Revision: 3}
			revision := 3
			kind, ref := "strm115", "0:5"
			switch scenario {
			case "revision":
				revision = 4
			case "ref":
				ref = "0:6"
			case "kind":
				kind = "share"
			}
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(shareSelectionLockSQL)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT selection_revision,stable_relative_path").WillReturnRows(sqlmock.NewRows([]string{"rev", "path"}).AddRow(revision, "fixed.strm"))
			mock.ExpectQuery("SELECT source_kind,external_ref").WillReturnRows(sqlmock.NewRows([]string{"kind", "ref"}).AddRow(kind, ref))
			if scenario == "success" {
				mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO t_strm_export_state").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE t_share_media_selection SET stable_relative_path").WithArgs(selection.ItemKey, "fixed.strm", selection.Revision).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = (&StrmExportDAO{Conn: connection}).SaveSelectionExport(context.Background(), ExportState{Owner: "share:default", Key: selection.ItemKey}, selection, "fixed.strm")
			if (err == nil) != (scenario == "success") {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelection115CompleteRetainsKeyWithoutActiveShareState(t *testing.T) {
	database, mock, _ := sqlmock.New()
	defer database.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT config_revision.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
	mock.ExpectQuery("SELECT revision,acked_revision.*FOR UPDATE").WithArgs("tmdb:tv:1").WillReturnRows(sqlmock.NewRows([]string{"revision", "acked"}).AddRow(3, 2))
	mock.ExpectExec("UPDATE t_share_export_source_state SET state='stale'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(shareExportAckSQL)).WithArgs("tmdb:tv:1", int64(3), "run").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	marked, err := NewShareExportCheckpointDAO(database).CompleteSelectionWork(context.Background(), domain.ShareExportDirty{WorkKey: "tmdb:tv:1", Revision: 3}, 1, "run", nil, ExportSnapshot{"tmdb:tv:1:1:2": "old-run"}, []string{"tmdb:tv:1:1:2"})
	if err != nil || marked != 0 {
		t.Fatalf("%d %v", marked, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
