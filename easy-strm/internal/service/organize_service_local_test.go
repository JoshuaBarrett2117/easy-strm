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

func TestOrganizeServiceScanLocalFilesWithoutMetadataStillReturnsCandidates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed video file: %v", err)
	}

	svc := &OrganizeService{}
	files, err := svc.scanLocalFiles(root, "", "all", []string{"movie.mp4"}, false)
	if err != nil {
		t.Fatalf("expected local scan without metadata to succeed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 scanned file, got %d", len(files))
	}
	if files[0].Name != "movie.mp4" {
		t.Fatalf("unexpected scanned file: %+v", files[0])
	}
	if files[0].Size != 0 {
		t.Fatalf("expected lightweight scan to skip file size lookup, got %d", files[0].Size)
	}
	if !files[0].ModifyTime.IsZero() {
		t.Fatalf("expected lightweight scan to skip modify time lookup, got %v", files[0].ModifyTime)
	}
}

func TestOrganizeServiceScanFilesDirectlyCollectsSelectedLocalFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed selected video file: %v", err)
	}
	blockedDir := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blockedDir, 0755); err != nil {
		t.Fatalf("failed to seed blocked dir: %v", err)
	}

	svc := &OrganizeService{}
	files, err := svc.scanFiles(
		&domain.MediaSource{SourceType: domain.SourceTypeLocal, Path: root},
		"",
		"all",
		[]string{"movie.mp4"},
		false,
	)
	if err != nil {
		t.Fatalf("expected direct selected-file scan to succeed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected only the selected file, got %d", len(files))
	}
	if files[0].Name != "movie.mp4" {
		t.Fatalf("unexpected selected file result: %+v", files[0])
	}
}

func TestOrganizeServiceApplyOrganizeRenameOverridesUsesEditedName(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	source := &domain.MediaSource{SourceType: domain.SourceTypeLocal}
	previews := []OrganizePreview{
		{
			FileID:     "Inception.2010.1080p.mkv",
			FileName:   "Inception.2010.1080p.mkv",
			NewName:    "盗梦空间 (2010).mkv",
			TargetPath: targetDir,
			NewPath:    filepath.Join(targetDir, "盗梦空间 (2010).mkv"),
		},
	}
	renameItems := []domain.OrganizeRenameOverride{
		{
			FileID:  "Inception.2010.1080p.mkv",
			NewName: "Manual Override Final.mkv",
		},
	}

	svc := &OrganizeService{}
	svc.applyOrganizeRenameOverrides(previews, source, renameItems)

	if previews[0].NewName != "Manual Override Final.mkv" {
		t.Fatalf("expected edited name to replace preview name, got %q", previews[0].NewName)
	}
	if filepath.Base(previews[0].NewPath) != "Manual Override Final.mkv" {
		t.Fatalf("expected edited name to flow into target path, got %q", previews[0].NewPath)
	}
	if previews[0].Conflict {
		t.Fatalf("expected no conflict for non-existing override target")
	}
}

func TestOrganizeServiceAppendMissingFilePreviewsAddsRemovedSelectedFile(t *testing.T) {
	svc := &OrganizeService{}
	files := []domain.MediaFile{
		{ID: "present.mkv", Name: "present.mkv", Path: "present.mkv"},
	}
	previews := []OrganizePreview{
		{FileID: "present.mkv", FileName: "present.mkv", FilePath: "present.mkv"},
	}

	result := svc.appendMissingFilePreviews(previews, files, []string{"present.mkv", "missing.mkv"})
	if len(result) != 2 {
		t.Fatalf("expected 2 previews after appending missing file, got %d", len(result))
	}
	if result[1].FileID != "missing.mkv" {
		t.Fatalf("expected missing preview to keep original file id, got %+v", result[1])
	}
	if result[1].IdentifyError != "源文件不存在或已被移除" {
		t.Fatalf("expected missing preview error message, got %+v", result[1])
	}
}
