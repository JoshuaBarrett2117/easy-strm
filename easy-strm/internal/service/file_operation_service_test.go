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

func TestFileOperationServiceRenameLocalPreservesExtension(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	fileSvc := NewFileOperationService(mediaSvc, nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	result, err := fileSvc.RenameFile(1, "old.mkv", "renamed")
	if err != nil {
		t.Fatalf("expected rename to succeed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected rename result to be successful: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.mkv")); err != nil {
		t.Fatalf("expected renamed file to exist: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFileOperationServiceDeleteLocal(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "delete.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	fileSvc := NewFileOperationService(mediaSvc, nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	result, err := fileSvc.DeleteFile(1, []string{"delete.mp4"})
	if err != nil {
		t.Fatalf("expected delete to succeed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected delete result to be successful: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "delete.mp4")); !os.IsNotExist(err) {
		t.Fatalf("expected deleted file to be removed, got err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFileOperationServiceDeleteLocalDirectory(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	dirPath := filepath.Join(root, "season1")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("failed to seed directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "episode1.mkv"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed nested file: %v", err)
	}

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	fileSvc := NewFileOperationService(mediaSvc, nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "shows", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	result, err := fileSvc.DeleteFile(1, []string{"season1"})
	if err != nil {
		t.Fatalf("expected directory delete to succeed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected directory delete result to be successful: %+v", result)
	}
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted directory to be removed, got err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}




