package service

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

func expectLocalMediaSourceByID(t *testing.T, mock sqlmock.Sqlmock, sourceID int, root string) {
	t.Helper()

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(sourceID, "local", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(sourceID).
		WillReturnRows(rows)
}

func TestOrganizeServiceListOrganizeCandidatesIncludesVideosFromSelectedDirectories(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	dirA := filepath.Join(root, "DirA")
	dirB := filepath.Join(root, "DirB")
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatalf("failed to create DirA: %v", err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatalf("failed to create DirB: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed video file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "song.mp3"), []byte("audio"), 0644); err != nil {
		t.Fatalf("failed to seed audio file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "episode.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed episode file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("text"), 0644); err != nil {
		t.Fatalf("failed to seed note file: %v", err)
	}

	expectLocalMediaSourceByID(t, mock, 1, root)
	svc := &OrganizeService{
		mediaSourceService: NewMediaSourceService(dao.NewMediaSourceDAO(), nil),
	}

	candidates, err := svc.ListOrganizeCandidates(1, "", "all", []string{"DirA", "DirB"})
	if err != nil {
		t.Fatalf("expected candidate listing to succeed: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 video candidates, got %d", len(candidates))
	}
	for _, candidate := range candidates {
		if ext := filepath.Ext(candidate.FileName); ext != ".mp4" && ext != ".mkv" {
			t.Fatalf("expected only video candidates, got %q", candidate.FileName)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}





