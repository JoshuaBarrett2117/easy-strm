package service

import (
	"path/filepath"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	driver "github.com/SheltonZhu/115driver/pkg/driver"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

type fakeOrganizeCloud115Client struct {
	fileList   *driver.FileListResp
	renameFile string
	renameName string
	moveFile   string
	moveTarget string
	copyFile   string
	copyTarget string
	mkdirPath  string
}

func (f *fakeOrganizeCloud115Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	if f.fileList == nil {
		return &driver.FileListResp{}, nil
	}
	return f.fileList, nil
}

func (f *fakeOrganizeCloud115Client) GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}

func (f *fakeOrganizeCloud115Client) GetCIDByPath(path string, cloud115ID int, cookie string) (string, error) {
	return "0", nil
}

func (f *fakeOrganizeCloud115Client) RenameFile(fileID, newName string, cloud115ID int, cookie string) error {
	f.renameFile = fileID
	f.renameName = newName
	return nil
}

func (f *fakeOrganizeCloud115Client) CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error {
	f.copyFile = fileID
	f.copyTarget = targetDirID
	return nil
}

func (f *fakeOrganizeCloud115Client) MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error {
	f.moveFile = fileID
	f.moveTarget = targetDirID
	return nil
}

func (f *fakeOrganizeCloud115Client) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	f.mkdirPath = path
	return "target-cid", nil
}

func expectCloud115Account(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "name", "cookie", "refresh_token", "access_token", "expires_in",
		"transfer_account_id", "transfer_directory", "account_type", "quota_used", "priority",
		"status", "cooling_start_time", "transfer_method", "alist_url", "alist_token",
		"create_time", "update_time",
	}).AddRow(1, "115", "cookie", "", "", 0, 0, "", "resource", 0, 5, "active", nil, "115driver", "", "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(rows)
}

func TestOrganizeService_BuildTargetPathDoesNotCreateSameNameFolder(t *testing.T) {
	svc := &OrganizeService{}

	finalTargetPath, newName, newPath := svc.buildOrganizeTargetPath("D:/Target", "BROWSER-Title.mp4")
	if filepath.ToSlash(finalTargetPath) != "D:/Target" {
		t.Fatalf("expected target path to stay unchanged, got %q", finalTargetPath)
	}
	if newName != "BROWSER-Title.mp4" {
		t.Fatalf("expected file name to stay unchanged, got %q", newName)
	}
	if filepath.ToSlash(newPath) != "D:/Target/BROWSER-Title.mp4" {
		t.Fatalf("expected file to be placed directly in target path, got %q", newPath)
	}
}

func TestOrganizeService_BuildTargetPathUsesTemplateDirectory(t *testing.T) {
	svc := &OrganizeService{}

	finalTargetPath, newName, newPath := svc.buildOrganizeTargetPath("D:/Target", "Movies/BROWSER-Title.mp4")
	if filepath.ToSlash(finalTargetPath) != "D:/Target/Movies" {
		t.Fatalf("expected template directory to become parent path, got %q", finalTargetPath)
	}
	if newName != "BROWSER-Title.mp4" {
		t.Fatalf("expected file name without template directory, got %q", newName)
	}
	if filepath.ToSlash(newPath) != "D:/Target/Movies/BROWSER-Title.mp4" {
		t.Fatalf("expected file to be placed in template directory, got %q", newPath)
	}
}

func TestOrganizeService_ScanCloud115MatchesSelectedFileID(t *testing.T) {
	client := &fakeOrganizeCloud115Client{
		fileList: &driver.FileListResp{
			Files: []driver.FileInfo{
				{
					CategoryID: driver.IntString("0"),
					FileID:     "fid-1",
					Name:       "Movie.mkv",
					Type:       "mkv",
				},
			},
		},
	}
	svc := &OrganizeService{client: client}

	files, err := svc.scanCloud115Recursive("0", "", 1, "cookie", "all", []string{"fid-1"})
	if err != nil {
		t.Fatalf("expected scan to succeed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected selected 115 file to be scanned, got %d files", len(files))
	}
	if files[0].CID != "fid-1" {
		t.Fatalf("expected cloud file id to be preserved, got %q", files[0].CID)
	}
}

func TestOrganizeService_OrganizeCloud115MovesFile(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectCloud115Account(t, mock)

	client := &fakeOrganizeCloud115Client{}
	cloudID := 1
	svc := &OrganizeService{
		cloud115DAO: dao.NewCloud115DAO(),
		client:      client,
	}

	result, err := svc.organizeCloud115File(&domain.MediaSource{
		ID:         1,
		SourceType: domain.SourceTypeCloud115,
		Cloud115ID: &cloudID,
	}, OrganizePreview{
		FileID:     "Movie.mkv",
		CloudID:    "fid-1",
		FileName:   "Movie.mkv",
		FilePath:   "Movie.mkv",
		NewName:    "BROWSER-Movie.mkv",
		NewPath:    "/library/movies/BROWSER-Movie.mkv",
		TargetPath: "/library/movies",
	}, "skip", true)
	if err != nil {
		t.Fatalf("expected 115 organize to succeed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	if client.mkdirPath != "/library/movies" {
		t.Fatalf("expected target directory to be created, got %q", client.mkdirPath)
	}
	if client.renameFile != "fid-1" || client.renameName != "BROWSER-Movie.mkv" {
		t.Fatalf("expected rename by 115 file id, got file=%q name=%q", client.renameFile, client.renameName)
	}
	if client.moveFile != "fid-1" || client.moveTarget != "target-cid" {
		t.Fatalf("expected move by 115 file id, got file=%q target=%q", client.moveFile, client.moveTarget)
	}
	if client.copyFile != "" || client.copyTarget != "" {
		t.Fatalf("expected move mode not to copy, got file=%q target=%q", client.copyFile, client.copyTarget)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
