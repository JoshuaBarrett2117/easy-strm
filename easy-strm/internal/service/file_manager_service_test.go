package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"

	"easy-strm/internal/domain"
)

type fakeFileManagerMediaSources struct {
	items map[int]*domain.MediaSource
}

func (f *fakeFileManagerMediaSources) GetAll(_, _ string) ([]*domain.MediaSource, error) {
	result := make([]*domain.MediaSource, 0, len(f.items))
	for _, item := range f.items {
		result = append(result, item)
	}
	return result, nil
}

func (f *fakeFileManagerMediaSources) GetByID(id int) (*domain.MediaSource, error) {
	return f.items[id], nil
}

type fakeFileManagerAccounts struct {
	items map[int]*domain.Cloud115
}

func (f *fakeFileManagerAccounts) GetAll(_, _ string) ([]*domain.Cloud115, error) {
	result := make([]*domain.Cloud115, 0, len(f.items))
	for _, item := range f.items {
		result = append(result, item)
	}
	return result, nil
}

func (f *fakeFileManagerAccounts) GetByID(id int) (*domain.Cloud115, error) {
	return f.items[id], nil
}

type fakeFileManagerTasks struct {
	mu       sync.Mutex
	status   string
	progress int
	done     chan struct{}
	once     sync.Once
}

func newFakeFileManagerTasks() *fakeFileManagerTasks {
	return &fakeFileManagerTasks{done: make(chan struct{})}
}

func (f *fakeFileManagerTasks) Create(_, _, _ string) error { return nil }
func (f *fakeFileManagerTasks) UpdateStatus(_, status string) error {
	f.mu.Lock()
	f.status = status
	f.mu.Unlock()
	if status == "completed" {
		f.once.Do(func() { close(f.done) })
	}
	return nil
}
func (f *fakeFileManagerTasks) UpdateProgress(_ string, _, processed, _, _ int) error {
	f.mu.Lock()
	f.progress = processed
	f.mu.Unlock()
	return nil
}
func (f *fakeFileManagerTasks) UpdateMetadata(string, map[string]interface{}) error { return nil }
func (f *fakeFileManagerTasks) SetError(_ string, _ string) error {
	f.mu.Lock()
	f.status = "failed"
	f.mu.Unlock()
	f.once.Do(func() { close(f.done) })
	return nil
}
func (f *fakeFileManagerTasks) IsCancelled(string) bool                   { return false }
func (f *fakeFileManagerTasks) RegisterCancel(string, context.CancelFunc) {}
func (f *fakeFileManagerTasks) RemoveCancel(string)                       {}

type fakeFileManagerCloudClient struct {
	mu             sync.Mutex
	lists          map[string][]driver.FileInfo
	copies         []string
	moves          []string
	deletes        []string
	downloads      []string
	rapidTransfers []string
	uploads        []string
	createdDirs    []string
	rapidErr       error
	downloadErr    error
}

func (f *fakeFileManagerCloudClient) GetFileList(cid int, _ int, offset int, limit int, _ int, _ string) (*driver.FileListResp, error) {
	items := f.lists[strconv.Itoa(cid)]
	if offset >= len(items) {
		return &driver.FileListResp{Files: []driver.FileInfo{}}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return &driver.FileListResp{Files: items[offset:end]}, nil
}
func (f *fakeFileManagerCloudClient) CopyFile(fileID, target string, accountID int, _ string) error {
	f.mu.Lock()
	f.copies = append(f.copies, fileID+":"+target+":"+strconv.Itoa(accountID))
	f.mu.Unlock()
	return nil
}
func (f *fakeFileManagerCloudClient) MoveFile115(fileID, target string, accountID int, _ string) error {
	f.mu.Lock()
	f.moves = append(f.moves, fileID+":"+target+":"+strconv.Itoa(accountID))
	f.mu.Unlock()
	return nil
}
func (f *fakeFileManagerCloudClient) RapidTransferFile(pickCode string, _ int, _ string, target string, _ int, _ string, name string) (string, error) {
	f.mu.Lock()
	f.rapidTransfers = append(f.rapidTransfers, pickCode+":"+target+":"+name)
	f.mu.Unlock()
	if f.rapidErr != nil {
		return "", f.rapidErr
	}
	return "new-pick", nil
}
func (f *fakeFileManagerCloudClient) CreateDirectory115(parent, name string, _ int, _ string) (string, error) {
	f.mu.Lock()
	f.createdDirs = append(f.createdDirs, parent+":"+name)
	f.mu.Unlock()
	return "900", nil
}
func (f *fakeFileManagerCloudClient) DeleteFiles115(fileIDs []string, accountID int, _ string) error {
	f.mu.Lock()
	f.deletes = append(f.deletes, strings.Join(fileIDs, ",")+":"+strconv.Itoa(accountID))
	f.mu.Unlock()
	return nil
}
func (f *fakeFileManagerCloudClient) UploadLocalFile115(path, target, name string, _ int, _ string) error {
	f.mu.Lock()
	f.uploads = append(f.uploads, filepath.Base(path)+":"+target+":"+name)
	f.mu.Unlock()
	return nil
}
func (f *fakeFileManagerCloudClient) DownloadFile115(pickCode, target string, accountID int, _ string) error {
	f.mu.Lock()
	f.downloads = append(f.downloads, pickCode+":"+filepath.Base(target)+":"+strconv.Itoa(accountID))
	f.mu.Unlock()
	if f.downloadErr != nil {
		return f.downloadErr
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte("downloaded"), 0644)
}

func TestFileManagerBrowseLocalAndRejectEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "Series"), 0755); err != nil {
		t.Fatal(err)
	}
	service := newFileManagerTestService(map[int]*domain.MediaSource{1: {ID: 1, Name: "local", SourceType: domain.SourceTypeLocal, Path: root, Enabled: true}}, nil)
	result, err := service.Browse(domain.FileManagerLocationRef{Type: "local", ID: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || !result.Entries[0].IsDirectory || result.Entries[1].Name != "movie.mp4" {
		t.Fatalf("unexpected browse result: %+v", result)
	}
	if _, err := service.Browse(domain.FileManagerLocationRef{Type: "local", ID: 1}, "../"); err == nil {
		t.Fatal("expected path escape to be rejected")
	}
}

func TestFileManagerLocalToLocalMoveRunsAsTask(t *testing.T) {
	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceRoot, "episode.mkv"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	tasks := newFakeFileManagerTasks()
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{
		1: {ID: 1, SourceType: domain.SourceTypeLocal, Path: sourceRoot},
		2: {ID: 2, SourceType: domain.SourceTypeLocal, Path: targetRoot},
	}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{}}, &fakeFileManagerCloudClient{}, tasks)
	_, err := service.StartTransfer(domain.FileManagerTransferRequest{Operation: "move", Source: domain.FileManagerLocationRef{Type: "local", ID: 1}, Target: domain.FileManagerLocationRef{Type: "local", ID: 2}, Items: []domain.FileManagerTransferItem{{ID: "episode.mkv", Path: "episode.mkv", Name: "episode.mkv"}}})
	if err != nil {
		t.Fatal(err)
	}
	waitFileManagerTask(t, tasks)
	if _, err := os.Stat(filepath.Join(targetRoot, "episode.mkv")); err != nil {
		t.Fatalf("target missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "episode.mkv")); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, got %v", err)
	}
}

func TestFileManagerCrossAccountDirectoryCopiesViaDownloadAndUpload(t *testing.T) {
	cloud := &fakeFileManagerCloudClient{lists: map[string][]driver.FileInfo{
		"10": {{FileID: "file-id", Name: "demo.mp4", PickCode: "pick-1"}},
	}}
	tasks := newFakeFileManagerTasks()
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{
		1: {ID: 1, Name: "source", Cookie: "a"},
		2: {ID: 2, Name: "target", Cookie: "b"},
	}}, cloud, tasks)
	_, err := service.StartTransfer(domain.FileManagerTransferRequest{Operation: "copy", Source: domain.FileManagerLocationRef{Type: "cloud115", ID: 1}, Target: domain.FileManagerLocationRef{Type: "cloud115", ID: 2}, TargetPath: "20", Items: []domain.FileManagerTransferItem{{ID: "10", Name: "Folder", IsDirectory: true}}})
	if err != nil {
		t.Fatal(err)
	}
	waitFileManagerTask(t, tasks)
	if len(cloud.createdDirs) != 1 || cloud.createdDirs[0] != "20:Folder" {
		t.Fatalf("unexpected created dirs: %v", cloud.createdDirs)
	}
	if len(cloud.downloads) != 1 || cloud.downloads[0] != "pick-1:demo.mp4:1" {
		t.Fatalf("unexpected downloads: %v", cloud.downloads)
	}
	if len(cloud.uploads) != 1 || cloud.uploads[0] != "demo.mp4:900:demo.mp4" {
		t.Fatalf("unexpected uploads: %v", cloud.uploads)
	}
}

func TestFileManagerLocalDirectoryUploadsRecursively(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Anime"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Anime", "S01E01.mp4"), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	cloud := &fakeFileManagerCloudClient{}
	tasks := newFakeFileManagerTasks()
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{1: {ID: 1, SourceType: domain.SourceTypeLocal, Path: root}}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{2: {ID: 2, Cookie: "cookie"}}}, cloud, tasks)
	_, err := service.StartTransfer(domain.FileManagerTransferRequest{Operation: "copy", Source: domain.FileManagerLocationRef{Type: "local", ID: 1}, Target: domain.FileManagerLocationRef{Type: "cloud115", ID: 2}, Items: []domain.FileManagerTransferItem{{ID: "Anime", Path: "Anime", Name: "Anime", IsDirectory: true}}})
	if err != nil {
		t.Fatal(err)
	}
	waitFileManagerTask(t, tasks)
	if len(cloud.uploads) != 1 || cloud.uploads[0] != "S01E01.mp4:900:S01E01.mp4" {
		t.Fatalf("unexpected uploads: %v", cloud.uploads)
	}
}

func TestFileManagerLocalDirectoryCopyAndDelete(t *testing.T) {
	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceRoot, "Series", "Season 01"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "Series", "Season 01", "S01E01.mp4"), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	tasks := newFakeFileManagerTasks()
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{
		1: {ID: 1, SourceType: domain.SourceTypeLocal, Path: sourceRoot},
		2: {ID: 2, SourceType: domain.SourceTypeLocal, Path: targetRoot},
	}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{}}, &fakeFileManagerCloudClient{}, tasks)
	_, err := service.StartTransfer(domain.FileManagerTransferRequest{Operation: "copy", Source: domain.FileManagerLocationRef{Type: "local", ID: 1}, Target: domain.FileManagerLocationRef{Type: "local", ID: 2}, Items: []domain.FileManagerTransferItem{{ID: "Series", Path: "Series", Name: "Series", IsDirectory: true}}})
	if err != nil {
		t.Fatal(err)
	}
	waitFileManagerTask(t, tasks)
	if _, err := os.Stat(filepath.Join(targetRoot, "Series", "Season 01", "S01E01.mp4")); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "Series", "Season 01", "S01E01.mp4")); err != nil {
		t.Fatalf("copy must preserve source: %v", err)
	}
	if err := service.Delete(domain.FileManagerDeleteRequest{Location: domain.FileManagerLocationRef{Type: "local", ID: 2}, Items: []domain.FileManagerTransferItem{{Path: "Series", Name: "Series", IsDirectory: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "Series")); !os.IsNotExist(err) {
		t.Fatalf("deleted directory still exists: %v", err)
	}
	if err := service.Delete(domain.FileManagerDeleteRequest{Location: domain.FileManagerLocationRef{Type: "local", ID: 2}, Items: []domain.FileManagerTransferItem{{Path: "", Name: "root", IsDirectory: true}}}); err == nil {
		t.Fatal("expected media source root deletion to be rejected")
	}
}

func TestFileManagerCloudDirectoryDownloadsRecursively(t *testing.T) {
	targetRoot := t.TempDir()
	cloud := &fakeFileManagerCloudClient{lists: map[string][]driver.FileInfo{
		"10": {{CategoryID: jsonNumber("11"), Name: "Season 01", Type: "folder"}},
		"11": {{FileID: "file-1", Name: "S01E01.mp4", PickCode: "pick-1"}},
	}}
	tasks := newFakeFileManagerTasks()
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{2: {ID: 2, SourceType: domain.SourceTypeLocal, Path: targetRoot}}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{1: {ID: 1, Cookie: "cookie"}}}, cloud, tasks)
	_, err := service.StartTransfer(domain.FileManagerTransferRequest{Operation: "copy", Source: domain.FileManagerLocationRef{Type: "cloud115", ID: 1}, Target: domain.FileManagerLocationRef{Type: "local", ID: 2}, Items: []domain.FileManagerTransferItem{{ID: "10", Name: "Series", IsDirectory: true}}})
	if err != nil {
		t.Fatal(err)
	}
	waitFileManagerTask(t, tasks)
	if _, err := os.Stat(filepath.Join(targetRoot, "Series", "Season 01", "S01E01.mp4")); err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}
	if len(cloud.downloads) != 1 || cloud.downloads[0] != "pick-1:S01E01.mp4:1" {
		t.Fatalf("unexpected downloads: %v", cloud.downloads)
	}
}

func TestFileManagerFailedCloudDirectoryDownloadLeavesNoPartialTarget(t *testing.T) {
	targetRoot := t.TempDir()
	cloud := &fakeFileManagerCloudClient{
		lists:       map[string][]driver.FileInfo{"10": {{FileID: "file-1", Name: "S01E01.mp4", PickCode: "pick-1"}}},
		downloadErr: errors.New("download failed"),
	}
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{2: {ID: 2, SourceType: domain.SourceTypeLocal, Path: targetRoot}}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{1: {ID: 1, Cookie: "cookie"}}}, cloud, newFakeFileManagerTasks())
	err := service.transferCloudToLocal(context.Background(), 1, 2, "", domain.FileManagerTransferItem{ID: "10", Name: "Series", IsDirectory: true}, false)
	if err == nil {
		t.Fatal("expected download error")
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "Series")); !os.IsNotExist(err) {
		t.Fatalf("partial target must be cleaned up: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(targetRoot, ".easy-strm-download-dir-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary directories must be cleaned up: matches=%v err=%v", matches, err)
	}
}

func TestFileManagerSameAccountCopyAndMove(t *testing.T) {
	cloud := &fakeFileManagerCloudClient{}
	service := newFileManagerCloudTestService(cloud)
	item := domain.FileManagerTransferItem{ID: "file-1", Name: "demo.mp4"}
	if err := service.transferCloudToCloud(context.Background(), 1, 1, "20", item, false); err != nil {
		t.Fatal(err)
	}
	if err := service.transferCloudToCloud(context.Background(), 1, 1, "30", item, true); err != nil {
		t.Fatal(err)
	}
	if len(cloud.copies) != 1 || cloud.copies[0] != "file-1:20:1" {
		t.Fatalf("unexpected copies: %v", cloud.copies)
	}
	if len(cloud.moves) != 1 || cloud.moves[0] != "file-1:30:1" {
		t.Fatalf("unexpected moves: %v", cloud.moves)
	}
}

func TestFileManagerCrossAccountMoveDeletesOnlyAfterSuccess(t *testing.T) {
	item := domain.FileManagerTransferItem{ID: "file-1", Name: "demo.mp4"}
	cloud := &fakeFileManagerCloudClient{}
	service := newFileManagerCloudTestService(cloud)
	if err := service.transferCloudToCloud(context.Background(), 1, 2, "20", item, true); err != nil {
		t.Fatal(err)
	}
	if len(cloud.downloads) != 1 || len(cloud.uploads) != 1 || len(cloud.deletes) != 1 || cloud.deletes[0] != "file-1:1" {
		t.Fatalf("unexpected successful move calls: downloads=%v uploads=%v delete=%v", cloud.downloads, cloud.uploads, cloud.deletes)
	}

	failedCloud := &fakeFileManagerCloudClient{rapidErr: errors.New("rapid transfer failed"), downloadErr: errors.New("download failed")}
	failedService := newFileManagerCloudTestService(failedCloud)
	if err := failedService.transferCloudToCloud(context.Background(), 1, 2, "20", item, true); err == nil {
		t.Fatal("expected rapid transfer error")
	}
	if len(failedCloud.deletes) != 0 {
		t.Fatalf("source must not be deleted after failed transfer: %v", failedCloud.deletes)
	}
}

func TestFileManagerCrossAccountCopiesViaDownloadAndUpload(t *testing.T) {
	item := domain.FileManagerTransferItem{ID: "file-1", PickCode: "pick-1", Name: "demo.mp4"}
	cloud := &fakeFileManagerCloudClient{}
	service := newFileManagerCloudTestService(cloud)

	if err := service.transferCloudToCloud(context.Background(), 1, 2, "20", item, false); err != nil {
		t.Fatal(err)
	}
	if len(cloud.downloads) != 1 || cloud.downloads[0] != "pick-1:demo.mp4:1" {
		t.Fatalf("unexpected fallback downloads: %v", cloud.downloads)
	}
	if len(cloud.uploads) != 1 || cloud.uploads[0] != "demo.mp4:20:demo.mp4" {
		t.Fatalf("unexpected fallback uploads: %v", cloud.uploads)
	}
}

func TestFileManagerLocalToCloudMoveAndCloudDelete(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "move.mp4")
	if err := os.WriteFile(filePath, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	cloud := &fakeFileManagerCloudClient{}
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{1: {ID: 1, SourceType: domain.SourceTypeLocal, Path: root}}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{2: {ID: 2, Cookie: "cookie"}}}, cloud, newFakeFileManagerTasks())
	if err := service.transferLocalToCloud(context.Background(), 1, 2, "30", domain.FileManagerTransferItem{ID: "move.mp4", Path: "move.mp4", Name: "move.mp4"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("source should be removed after upload: %v", err)
	}
	if err := service.Delete(domain.FileManagerDeleteRequest{Location: domain.FileManagerLocationRef{Type: "cloud115", ID: 2}, Items: []domain.FileManagerTransferItem{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}}}); err != nil {
		t.Fatal(err)
	}
	if len(cloud.deletes) != 1 || cloud.deletes[0] != "a,b:2" {
		t.Fatalf("unexpected delete calls: %v", cloud.deletes)
	}
}

func newFileManagerCloudTestService(cloud *fakeFileManagerCloudClient) *FileManagerService {
	return NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{
		1: {ID: 1, Cookie: "source"},
		2: {ID: 2, Cookie: "target"},
	}}, cloud, newFakeFileManagerTasks())
}

func jsonNumber(value string) driver.IntString {
	return driver.IntString(value)
}

func newFileManagerTestService(sources map[int]*domain.MediaSource, accounts map[int]*domain.Cloud115) *FileManagerService {
	if accounts == nil {
		accounts = map[int]*domain.Cloud115{}
	}
	return NewFileManagerService(&fakeFileManagerMediaSources{items: sources}, &fakeFileManagerAccounts{items: accounts}, &fakeFileManagerCloudClient{}, newFakeFileManagerTasks())
}

func waitFileManagerTask(t *testing.T, tasks *fakeFileManagerTasks) {
	t.Helper()
	select {
	case <-tasks.done:
	case <-time.After(2 * time.Second):
		t.Fatal("file manager task timed out")
	}
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if tasks.status != "completed" {
		t.Fatalf("task status=%s", tasks.status)
	}
}
