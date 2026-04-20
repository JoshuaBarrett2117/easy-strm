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

func setupServiceMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dao.Init(db, nil)
	return mock, func() { _ = db.Close() }
}

func TestMediaSourceServiceCreateLocal(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "movies", "local", `C:\\media`, `C:\\media`, nil, 10, true, "/organized", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_media_source (name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time`)).
		WithArgs("movies", domain.SourceTypeLocal, `C:\media`, `C:\media`, nil, 10, true, "/organized", "all", "skip", "move", false, false, 60, "").
		WillReturnRows(rows)

	source, err := svc.Create("movies", domain.SourceTypeLocal, `C:\media`, `C:\media`, nil, 10, true, "/organized", "all", "skip", "move", false, false, 60, "")
	if err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}
	if source.Name != "movies" {
		t.Fatalf("unexpected source: %+v", source)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceCreateDisablesAutoOrganizeWithoutWatch(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(2, "movies", "local", `C:\\media`, `C:\\media`, nil, 10, true, "/organized", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_media_source (name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time`)).
		WithArgs("movies", domain.SourceTypeLocal, `C:\media`, `C:\media`, nil, 10, true, "/organized", "all", "skip", "move", false, false, 60, "").
		WillReturnRows(rows)

	source, err := svc.Create("movies", domain.SourceTypeLocal, `C:\media`, `C:\media`, nil, 10, true, "/organized", "all", "skip", "move", true, false, 60, "")
	if err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}
	if source.AutoOrganize {
		t.Fatalf("expected auto organize to be disabled when watch is off")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceGetFilesLocal(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("text"), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "Season1"), 0755); err != nil {
		t.Fatalf("failed to seed dir: %v", err)
	}

	svc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	now := time.Now()
	sourceRows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(1, "movies", domain.SourceTypeLocal, root, root, nil, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)
	cacheRows := sqlmock.NewRows([]string{"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path", "overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number", "raw_data", "expire_at", "create_time", "update_time"})

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time FROM t_media_source WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(sourceRows)
	mock.ExpectQuery(`SELECT .* FROM t_tmdb_cache\s+WHERE query_key IN \(.*`).
		WillReturnRows(cacheRows)

	result, err := svc.GetFiles(1, "", 1, 20, "name", "asc", "", "")
	if err != nil {
		t.Fatalf("expected get files to succeed: %v", err)
	}
	if result.Total != 3 {
		t.Fatalf("expected 3 entries, got %d", result.Total)
	}
	if result.Files[0].Name != "Season1" {
		t.Fatalf("expected directory to be sorted first, got %s", result.Files[0].Name)
	}
	for _, file := range result.Files {
		if len(file.ID) > 0 && (file.ID[0] == '\\' || file.ID[0] == '/') {
			t.Fatalf("expected relative file id without leading slash, got %q", file.ID)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceServiceValidateLocalPath(t *testing.T) {
	svc := NewMediaSourceService(nil, nil)
	validDir := t.TempDir()

	if err := svc.ValidateLocalPath(validDir); err != nil {
		t.Fatalf("expected valid path to pass: %v", err)
	}
	if err := svc.ValidateLocalPath(filepath.Join(validDir, "missing")); err == nil {
		t.Fatal("expected missing path to fail")
	}
}

func TestMediaSourceServiceCreateHardLink(t *testing.T) {
	svc := NewMediaSourceService(nil, nil)
	root := t.TempDir()
	src := filepath.Join(root, "source.mkv")
	dst := filepath.Join(root, "library", "target.mkv")

	if err := os.WriteFile(src, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed source file: %v", err)
	}
	if err := svc.CreateHardLink(src, dst); err != nil {
		t.Fatalf("expected hard link creation to succeed: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("expected hard link to exist: %v", err)
	}
	if info.Size() != int64(len("video")) {
		t.Fatalf("unexpected hard link size: %d", info.Size())
	}
}

func TestMediaSourceServiceCreateSymbolicLink(t *testing.T) {
	svc := NewMediaSourceService(nil, nil)
	root := t.TempDir()
	src := filepath.Join(root, "source.mkv")
	dst := filepath.Join(root, "library", "target.mkv")

	if err := os.WriteFile(src, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed source file: %v", err)
	}
	if err := svc.CreateSymbolicLink(src, dst); err != nil {
		t.Skipf("symbolic link is unavailable in current environment: %v", err)
	}

	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("expected symbolic link to exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %q to be a symbolic link", dst)
	}
}

