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

func TestMediaSourceServiceGetFilesPagination(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	for i := 1; i <= 30; i++ {
		name := "video" + string(rune(i/10+'0')) + string(rune(i%10+'0')) + ".mp4"
		if err := os.WriteFile(filepath.Join(root, name), []byte("video"), 0644); err != nil {
			t.Fatalf("failed to seed file: %v", err)
		}
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 10, "name", "asc", "", "")
	if err != nil {
		t.Fatalf("expected get files to succeed: %v", err)
	}
	if result.Total != 30 {
		t.Fatalf("expected 30 total files, got %d", result.Total)
	}
	if len(result.Files) != 10 {
		t.Fatalf("expected 10 files per page, got %d", len(result.Files))
	}
	if result.Page != 1 {
		t.Fatalf("expected page 1, got %d", result.Page)
	}
	if result.PageSize != 10 {
		t.Fatalf("expected page size 10, got %d", result.PageSize)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesSortingAsc(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	files := []string{"zebra.mp4", "apple.mp4", "banana.mp4"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte("video"), 0644); err != nil {
			t.Fatalf("failed to seed file: %v", err)
		}
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 50, "name", "asc", "", "")
	if err != nil {
		t.Fatalf("expected get files to succeed: %v", err)
	}
	if len(result.Files) < 3 {
		t.Fatalf("expected at least 3 files, got %d", len(result.Files))
	}
	firstFile := result.Files[0]
	if firstFile.Name != "apple.mp4" {
		t.Fatalf("expected first file to be apple.mp4 (ascending), got %s", firstFile.Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesSortingDesc(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	files := []string{"zebra.mp4", "apple.mp4", "banana.mp4"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte("video"), 0644); err != nil {
			t.Fatalf("failed to seed file: %v", err)
		}
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 50, "name", "desc", "", "")
	if err != nil {
		t.Fatalf("expected get files descending to succeed: %v", err)
	}
	firstFile := result.Files[0]
	if firstFile.Name != "zebra.mp4" {
		t.Fatalf("expected first file to be zebra.mp4 (descending), got %s", firstFile.Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesFilterVideo(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("image"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0755); err != nil {
		t.Fatalf("failed to seed dir: %v", err)
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 50, "name", "asc", "video", "")
	if err != nil {
		t.Fatalf("expected get files with video filter to succeed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 video file, got %d", result.Total)
	}
	if result.Files[0].Type != "video" {
		t.Fatalf("expected file type video, got %s", result.Files[0].Type)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesFilterDir(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0755); err != nil {
		t.Fatalf("failed to seed dir: %v", err)
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 50, "name", "asc", "dir", "")
	if err != nil {
		t.Fatalf("expected get files with dir filter to succeed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 directory, got %d", result.Total)
	}
	if !result.Files[0].IsDirectory {
		t.Fatalf("expected directory, got file")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesSearch(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	files := []string{"avatar.mp4", "test.mp4", "other.mp4"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte("video"), 0644); err != nil {
			t.Fatalf("failed to seed file: %v", err)
		}
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 50, "name", "asc", "", "avatar")
	if err != nil {
		t.Fatalf("expected get files with search to succeed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 file matching 'avatar', got %d", result.Total)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesSubdirectory(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	subdir := filepath.Join(root, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "root.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "sub.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "subdir", 1, 50, "name", "asc", "", "")
	if err != nil {
		t.Fatalf("expected get files in subdir to succeed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 file in subdir, got %d", result.Total)
	}
	if result.Files[0].Name != "sub.mp4" {
		t.Fatalf("expected file sub.mp4, got %s", result.Files[0].Name)
	}

	if len(result.Breadcrumb) != 1 {
		t.Fatalf("expected 1 breadcrumb item (subdir only), got %d", len(result.Breadcrumb))
	}
	if result.Breadcrumb[0].Name != "subdir" {
		t.Fatalf("expected breadcrumb to be subdir, got %s", result.Breadcrumb[0].Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesNonExistentSource(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(999).
		WillReturnRows(sourceRows)

	_, err := svc.GetFiles(999, "", 1, 50, "name", "asc", "", "")
	if err == nil {
		t.Fatalf("expected error for non-existent source, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesDisabledSource(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, `C:\test`, `C:\test`, nil, 10, false, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	_, err := svc.GetFiles(1, "", 1, 50, "name", "asc", "", "")
	if err == nil {
		t.Fatalf("expected error for disabled source, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesHiddenFiles(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "visible.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 50, "name", "asc", "", "")
	if err != nil {
		t.Fatalf("expected get files to succeed: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 visible file (hidden files should be filtered), got %d", result.Total)
	}
	if result.Files[0].Name != "visible.mp4" {
		t.Fatalf("expected visible.mp4, got %s", result.Files[0].Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesInvalidPath(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "test", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)

	_, err := svc.GetFiles(1, "nonexistent_path", 1, 50, "name", "asc", "", "")
	if err == nil {
		t.Fatalf("expected error for invalid path, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}




