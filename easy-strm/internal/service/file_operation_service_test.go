package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	driver "github.com/SheltonZhu/115driver/pkg/driver"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

type fakeFileOperationCloud115Client struct {
	mkdirPath string
	mkdirCID  string
	moveFile  string
	moveDir   string
	copyFile  string
	copyDir   string
	moveErr   error
	copyErr   error
	mkdirErr  error
}

func (f *fakeFileOperationCloud115Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	return nil, nil
}

func (f *fakeFileOperationCloud115Client) GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeFileOperationCloud115Client) GetCIDByPath(path string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeFileOperationCloud115Client) RenameFile(fileID, newName string, cloud115ID int, cookie string) error {
	return nil
}

func (f *fakeFileOperationCloud115Client) CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error {
	f.copyFile = fileID
	f.copyDir = targetDirID
	return f.copyErr
}

func (f *fakeFileOperationCloud115Client) MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error {
	f.moveFile = fileID
	f.moveDir = targetDirID
	return f.moveErr
}

func (f *fakeFileOperationCloud115Client) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	f.mkdirPath = path
	if f.mkdirErr != nil {
		return "", f.mkdirErr
	}
	return f.mkdirCID, nil
}

func (f *fakeFileOperationCloud115Client) RapidTransferFile(sourcePickCode string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (string, error) {
	return "", nil
}

func (f *fakeFileOperationCloud115Client) GetFileInfo(pickCode string, cloud115ID int, cookie string) (*driver.File, error) {
	return nil, nil
}

func (f *fakeFileOperationCloud115Client) GetShareSnap(shareCode, receiveCode, dirID string, queries ...driver.Query) (*driver.ShareSnapResp, error) {
	return nil, nil
}

func (f *fakeFileOperationCloud115Client) ReceiveShare(shareCode, receiveCode, fileIDs, saveFolderID string, targetCloud115ID int, targetCookie string) error {
	return nil
}

func expectFileOperationMediaSourceByID(mock sqlmock.Sqlmock, id int, sourceType, path string, cloud115ID *int) {
	now := time.Now()
	var cloud115Value interface{}
	if cloud115ID != nil {
		cloud115Value = *cloud115ID
	}
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "watch_path", "cloud115_id", "priority", "enabled", "organize_target_path", "media_type", "conflict_policy", "operation_mode", "auto_organize", "watch_enabled", "watch_interval", "emby_library_id", "create_time", "update_time"}).
		AddRow(id, fmt.Sprintf("source-%d", id), sourceType, path, path, cloud115Value, 10, true, "", "all", "skip", "move", false, false, 60, "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, watch_path, cloud115_id, priority, enabled, organize_target_path, media_type, conflict_policy, operation_mode, auto_organize, watch_enabled, watch_interval, emby_library_id, create_time, update_time
		FROM t_media_source WHERE id = $1`)).
		WithArgs(id).
		WillReturnRows(rows)
}

func expectCloud115ByID(mock sqlmock.Sqlmock, id int, cookie string) {
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "cookie", "cookie_source", "refresh_token", "access_token", "expires_in", "transfer_account_id", "transfer_directory", "account_type", "quota_used", "priority", "status", "cooling_start_time", "transfer_method", "alist_url", "alist_token", "create_time", "update_time"}).
		AddRow(id, "cloud", cookie, "网页版", "", "", 0, 0, "", "resource", 0, 5, "active", nil, "115driver", "", "", now, now)

	mock.ExpectQuery("SELECT id, name, cookie, COALESCE\\(cookie_source, ''\\), refresh_token").
		WithArgs(id).
		WillReturnRows(rows)
}

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

func TestFileOperationServiceMoveCloud115CallsClient(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cloudID := 2
	expectFileOperationMediaSourceByID(mock, 1, domain.SourceTypeCloud115, "0", &cloudID)
	expectCloud115ByID(mock, cloudID, "UID=u; CID=c; SEID=s; KID=k")

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	client := &fakeFileOperationCloud115Client{mkdirCID: "888"}
	fileSvc := NewFileOperationService(mediaSvc, dao.NewCloud115DAO(), client)

	result, err := fileSvc.MoveFile(1, "file-pick-code", "/整理/电影")
	if err != nil {
		t.Fatalf("expected cloud move to succeed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected move result to be successful: %+v", result)
	}
	if client.mkdirPath != "/整理/电影" {
		t.Fatalf("expected target directory to be created, got %q", client.mkdirPath)
	}
	if client.moveFile != "file-pick-code" || client.moveDir != "888" {
		t.Fatalf("expected client move call, got file=%q dir=%q", client.moveFile, client.moveDir)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFileOperationServiceMoveCloud115RequiresClient(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cloudID := 2
	expectFileOperationMediaSourceByID(mock, 1, domain.SourceTypeCloud115, "0", &cloudID)

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	fileSvc := NewFileOperationService(mediaSvc, dao.NewCloud115DAO())

	if _, err := fileSvc.MoveFile(1, "file-pick-code", "888"); err == nil {
		t.Fatal("expected cloud move without client to fail")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFileOperationServiceCopyCloud115CallsClient(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	cloudID := 2
	expectFileOperationMediaSourceByID(mock, 1, domain.SourceTypeCloud115, "0", &cloudID)
	expectFileOperationMediaSourceByID(mock, 2, domain.SourceTypeCloud115, "0", &cloudID)
	expectCloud115ByID(mock, cloudID, "UID=u; CID=c; SEID=s; KID=k")

	mediaSvc := NewMediaSourceService(dao.NewMediaSourceDAO(), nil)
	client := &fakeFileOperationCloud115Client{mkdirCID: "999"}
	fileSvc := NewFileOperationService(mediaSvc, dao.NewCloud115DAO(), client)

	result, err := fileSvc.CopyFile(1, 2, "file-pick-code", "/整理/剧集", false)
	if err != nil {
		t.Fatalf("expected cloud copy to succeed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected copy result to be successful: %+v", result)
	}
	if client.copyFile != "file-pick-code" || client.copyDir != "999" {
		t.Fatalf("expected client copy call, got file=%q dir=%q", client.copyFile, client.copyDir)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
