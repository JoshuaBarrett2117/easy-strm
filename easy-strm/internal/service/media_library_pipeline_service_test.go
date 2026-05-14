package service

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"easy-strm/internal/dao"
)

func TestBuildPipelineStrmPath(t *testing.T) {
	path := buildPipelineStrmPath(`D:\strm`, "Movies/Smoke/Movie.Smoke.2026.mkv", "Movie.Smoke.2026.mkv")
	expectedSuffix := filepath.Join("Movies", "Smoke", "Movie.Smoke.2026.strm")
	if !strings.HasSuffix(path, expectedSuffix) {
		t.Fatalf("expected suffix %s, got %s", expectedSuffix, path)
	}
}

func TestNormalizePipelineMediaType(t *testing.T) {
	if got := normalizePipelineMediaType("anime"); got != "tv" {
		t.Fatalf("expected anime to map to tv, got %s", got)
	}
	if got := normalizePipelineMediaType(""); got != "movie" {
		t.Fatalf("expected empty media type to map to movie, got %s", got)
	}
}

func TestGenerateStrmForItemTaskReturnsTraceableTaskID(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, source_id, source_type, source_file_id, source_path, source_name, source_pick_code, source_sha1, source_size, source_modified_time, target_path, strm_path, metadata_path, media_server_type, media_server_library_id, tmdb_id, media_type, identity_status, sync_status, last_change_type, last_task_id, created_at, updated_at FROM t_media_sync_index WHERE id=$1`)).
			WithArgs(9).
			WillReturnRows(newMediaSyncIndexRows().AddRow(
				9, 1, "local", "readme", `D:\media\README.txt`, "README.txt", "", "", int64(12), time.Now(),
				"", "", "", "", "", 0, "movie", "unknown", "active", "created", "", time.Now(), time.Now(),
			))
	}

	svc := NewMediaLibraryPipelineService(nil, dao.NewMediaSyncIndexDAO(), nil, nil, nil, nil, nil, nil, nil)
	result, err := svc.GenerateStrmForItemTask(9)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasPrefix(result.TaskID, "library_strm_item_9_") {
		t.Fatalf("expected traceable task id, got %s", result.TaskID)
	}
	if result.ItemID != 9 {
		t.Fatalf("expected item id 9, got %d", result.ItemID)
	}
	if result.Message == "" {
		t.Fatalf("expected user-facing action message")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func newMediaSyncIndexRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "source_id", "source_type", "source_file_id", "source_path", "source_name", "source_pick_code", "source_sha1",
		"source_size", "source_modified_time", "target_path", "strm_path", "metadata_path", "media_server_type",
		"media_server_library_id", "tmdb_id", "media_type", "identity_status", "sync_status", "last_change_type",
		"last_task_id", "created_at", "updated_at",
	})
}
