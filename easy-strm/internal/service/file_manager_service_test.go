package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	rapidTransfers []string
	uploads        []string
	createdDirs    []string
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
func (f *fakeFileManagerCloudClient) CopyFile(string, string, int, string) error    { return nil }
func (f *fakeFileManagerCloudClient) MoveFile115(string, string, int, string) error { return nil }
func (f *fakeFileManagerCloudClient) RapidTransferFile(pickCode string, _ int, _ string, target string, _ int, _ string, name string) (string, error) {
	f.mu.Lock()
	f.rapidTransfers = append(f.rapidTransfers, pickCode+":"+target+":"+name)
	f.mu.Unlock()
	return "new-pick", nil
}
func (f *fakeFileManagerCloudClient) CreateDirectory115(parent, name string, _ int, _ string) (string, error) {
	f.mu.Lock()
	f.createdDirs = append(f.createdDirs, parent+":"+name)
	f.mu.Unlock()
	return "900", nil
}
func (f *fakeFileManagerCloudClient) DeleteFiles115([]string, int, string) error { return nil }
func (f *fakeFileManagerCloudClient) UploadLocalFile115(path, target, name string, _ int, _ string) error {
	f.mu.Lock()
	f.uploads = append(f.uploads, filepath.Base(path)+":"+target+":"+name)
	f.mu.Unlock()
	return nil
}
func (f *fakeFileManagerCloudClient) DownloadFile115(string, string, int, string) error { return nil }

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

func TestFileManagerCrossAccountDirectoryUsesRapidTransfer(t *testing.T) {
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
	if len(cloud.rapidTransfers) != 1 || cloud.rapidTransfers[0] != "pick-1:900:demo.mp4" {
		t.Fatalf("unexpected rapid transfers: %v", cloud.rapidTransfers)
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
