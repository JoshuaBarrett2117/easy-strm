package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/DATA-DOG/go-sqlmock"
)

// 编译期接口一致性断言：确保 OrganizeService 满足 PostTransferOrganizer，
// LocalScraper / Cloud115Scraper 满足 PostTransferScraper（编排层依赖契约）。
var (
	_ PostTransferOrganizer = (*OrganizeService)(nil)
	_ PostTransferScraper   = (*LocalScraper)(nil)
	_ PostTransferScraper   = (*Cloud115Scraper)(nil)
)

// ==================== 纯函数单测 ====================

func TestInferMediaTypeFromName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Game.of.Thrones.S01E02.1080p.mkv", "tv"},
		{"第1季绝命毒师", "tv"},
		{"The.Movie.2020.1080p.mkv", "all"},
		{"", "all"},
		{"some random folder", "all"},
	}
	for _, c := range cases {
		if got := inferMediaTypeFromName(c.name); got != c.want {
			t.Errorf("inferMediaTypeFromName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveOrganizeTargetPath(t *testing.T) {
	if got := resolveOrganizeTargetPath(domain.TransferRequest{OrganizeTargetPath: "/custom", TargetDirectory: "/dest"}); got != "/custom" {
		t.Errorf("期望 /custom，实际 %q", got)
	}
	if got := resolveOrganizeTargetPath(domain.TransferRequest{TargetDirectory: "/dest"}); got != "/dest" {
		t.Errorf("期望缺省 /dest，实际 %q", got)
	}
}

func TestBuildAdhocOrganizeSource(t *testing.T) {
	req := domain.TransferRequest{
		TargetCloud115Id:   5,
		TargetDirectory:    "/d",
		OrganizeTargetPath: "/o",
		ConflictStrategy:   "overwrite",
	}
	src := buildAdhocOrganizeSource(req, &domain.Cloud115{ID: 5}, "Show.S01E02")

	if src.ID != 0 {
		t.Errorf("临时源 ID 应为 0，实际 %d", src.ID)
	}
	if src.SourceType != domain.SourceTypeCloud115 {
		t.Errorf("临时源类型应为 cloud115，实际 %q", src.SourceType)
	}
	if src.Cloud115ID == nil || *src.Cloud115ID != 5 {
		t.Errorf("临时源 Cloud115ID 应为 5")
	}
	if src.Path != "/d" || src.OrganizeTargetPath != "/o" {
		t.Errorf("路径不匹配: path=%q target=%q", src.Path, src.OrganizeTargetPath)
	}
	if src.ConflictPolicy != "overwrite" {
		t.Errorf("冲突策略应为 overwrite，实际 %q", src.ConflictPolicy)
	}
	if src.OperationMode != "move" {
		t.Errorf("云盘强制 move，实际 %q", src.OperationMode)
	}
	if src.MediaType != "tv" {
		t.Errorf("分享名含 S01E02 应推断 tv，实际 %q", src.MediaType)
	}

	// OrganizeTargetPath 缺省回退到 TargetDirectory
	req2 := domain.TransferRequest{TargetCloud115Id: 1, TargetDirectory: "/dest"}
	src2 := buildAdhocOrganizeSource(req2, &domain.Cloud115{ID: 1}, "Movie")
	if src2.OrganizeTargetPath != "/dest" {
		t.Errorf("缺省 OrganizeTargetPath 应回退 /dest，实际 %q", src2.OrganizeTargetPath)
	}
	if src2.MediaType != "all" {
		t.Errorf("无剧集特征应推断 all，实际 %q", src2.MediaType)
	}
}

// ==================== 115 刮削降级单测 ====================

func TestCloud115ScraperScrape(t *testing.T) {
	scraper := NewCloud115Scraper(nil, nil)
	res, err := scraper.Scrape(context.Background(), &domain.MediaSource{SourceType: domain.SourceTypeCloud115}, []string{"/电影/x.mkv"})
	if err != nil {
		t.Fatalf("115 降级刮削期望 error=nil，实际 %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d", len(res))
	}
	if res[0].Success {
		t.Errorf("115 降级结果应 Success=false")
	}
	if res[0].Message != "115 云盘暂不支持 NFO 写入（本期未实现上传）" {
		t.Errorf("降级原因不符: %q", res[0].Message)
	}
}

// ==================== 编排单测（mock Organizer / Scraper / TaskDAO） ====================

// mockOrganizer 记录 OrganizeDirectoryForSource 调用并返回成功结果。
type mockOrganizer struct {
	callCount   int
	lastSource  *domain.MediaSource
	lastFileIDs []string
}

func (m *mockOrganizer) OrganizeDirectoryForSource(ctx context.Context, source *domain.MediaSource, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride, progress func(OrganizeExecutionProgress), shouldStop func() bool) ([]OrganizeResult, error) {
	m.callCount++
	m.lastSource = source
	m.lastFileIDs = fileIDs
	return []OrganizeResult{{FileID: "f1", FileName: "x.mkv", Success: true, NewPath: "/电影/x.mkv"}}, nil
}

// mockScraper 记录 Scrape 调用，close done 以便测试同步等待。
type mockScraper struct {
	calls int
	once  sync.Once
	done  chan struct{}
}

func newMockScraper() *mockScraper {
	return &mockScraper{done: make(chan struct{})}
}

func (m *mockScraper) Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error) {
	m.calls++
	m.once.Do(func() { close(m.done) })
	return []ScrapeResult{{FilePath: "p", Success: true, Message: "ok"}}, nil
}

// fakeTaskDAO 内存实现 TaskDAO，供编排单测注入。
type fakeTaskDAO struct {
	mu    sync.Mutex
	tasks map[string]map[string]interface{}
}

func newFakeTaskDAO() *fakeTaskDAO {
	return &fakeTaskDAO{tasks: map[string]map[string]interface{}{}}
}

func (f *fakeTaskDAO) Create(taskID, taskType, taskName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[taskID] = map[string]interface{}{
		"task_id": taskID, "task_type": taskType, "task_name": taskName,
		"status": "pending", "metadata": map[string]interface{}{},
		"total_files": 0, "processed_files": 0, "success_files": 0, "failed_files": 0,
	}
	return nil
}

func (f *fakeTaskDAO) Get(taskID string) (map[string]interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[taskID]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (f *fakeTaskDAO) UpdateStatus(taskID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tasks[taskID]; ok {
		t["status"] = status
	}
	return nil
}

func (f *fakeTaskDAO) UpdateProgress(taskID string, tf, pf, sf, ff int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tasks[taskID]; ok {
		t["total_files"] = tf
		t["processed_files"] = pf
		t["success_files"] = sf
		t["failed_files"] = ff
	}
	return nil
}

func (f *fakeTaskDAO) SetError(taskID, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tasks[taskID]; ok {
		t["status"] = "failed"
		t["error_message"] = errMsg
	}
	return nil
}

func (f *fakeTaskDAO) UpdateMetadata(taskID string, metadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tasks[taskID]; ok {
		t["metadata"] = metadata
	}
	return nil
}

func (f *fakeTaskDAO) IsCancelled(taskID string) bool { return false }

func (f *fakeTaskDAO) AddProcessedFileID(taskID, fileID string) error { return nil }

func (f *fakeTaskDAO) SetCancelFlag(taskID string) error { return nil }

func (f *fakeTaskDAO) ClearCancelFlag(taskID string) error { return nil }

func findTaskByType(f *fakeTaskDAO, taskType string) map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, tk := range f.tasks {
		if tk["task_type"] == taskType {
			return tk
		}
	}
	return nil
}

// TestRunPostTransferOrganizeNoSuccessfulFiles 验证：没有成功落盘文件时不触发整理（向后兼容安全兜底）。
func TestRunPostTransferOrganizeNoSuccessfulFiles(t *testing.T) {
	organizer := &mockOrganizer{}
	dao := newFakeTaskDAO()
	svc := &ShareTransferService{taskDAO: dao, organizer: organizer, cloud115Scraper: &mockScraper{}}
	req := domain.TransferRequest{AutoOrganize: true, TargetCloud115Id: 1, TargetDirectory: "/d"}
	// successFids 为空
	svc.runPostTransferOrganize(context.Background(), "task1", req, &domain.Cloud115{ID: 1}, "123", []string{})
	if organizer.callCount != 0 {
		t.Fatalf("无成功文件时不应调用 organizer，实际 %d", organizer.callCount)
	}
	if findTaskByType(dao, "share_transfer_organize") != nil {
		t.Fatal("不应创建 organize 任务")
	}
}

// TestRunPostTransferOrganizeCreatesOrganizeTask 验证：auto=true 且有成功文件时创建 organize 任务并回写元数据。
func TestRunPostTransferOrganizeCreatesOrganizeTask(t *testing.T) {
	organizer := &mockOrganizer{}
	scraper := newMockScraper()
	dao := newFakeTaskDAO()
	svc := &ShareTransferService{taskDAO: dao, organizer: organizer, localScraper: scraper, cloud115Scraper: scraper}
	req := domain.TransferRequest{AutoOrganize: true, TargetCloud115Id: 1, TargetDirectory: "/d", OrganizeTargetPath: "/d"}
	dao.Create("task1", "share_transfer", "转存")
	dao.UpdateMetadata("task1", map[string]interface{}{"share_folder_name": "Show"})

	svc.runPostTransferOrganize(context.Background(), "task1", req, &domain.Cloud115{ID: 1}, "123", []string{"fid1"})

	if organizer.callCount != 1 {
		t.Fatalf("期望 organizer 被调用 1 次，实际 %d", organizer.callCount)
	}
	orgTask := findTaskByType(dao, "share_transfer_organize")
	if orgTask == nil {
		t.Fatal("期望创建 organize 任务")
	}
	if orgTask["status"] != "completed" {
		t.Fatalf("期望 organize 任务 completed，实际 %v", orgTask["status"])
	}
	task1, _ := dao.Get("task1")
	meta := task1["metadata"].(map[string]interface{})
	if meta["organize_task_id"] == "" {
		t.Fatal("期望转存任务元数据回写 organize_task_id")
	}
	// auto_scrape=false 不应触发刮削
	if scraper.calls != 0 {
		t.Fatalf("auto_scrape=false 不应触发 scrape，实际 %d", scraper.calls)
	}
}

// TestRunPostTransferOrganizeTriggersScrape 验证：整理成功且 auto_scrape=true 时触发刮削任务。
func TestRunPostTransferOrganizeTriggersScrape(t *testing.T) {
	organizer := &mockOrganizer{}
	scraper := newMockScraper()
	dao := newFakeTaskDAO()
	svc := &ShareTransferService{taskDAO: dao, organizer: organizer, localScraper: scraper, cloud115Scraper: scraper}
	req := domain.TransferRequest{AutoOrganize: true, AutoScrape: true, TargetCloud115Id: 1, TargetDirectory: "/d", OrganizeTargetPath: "/d"}
	dao.Create("task1", "share_transfer", "转存")
	dao.UpdateMetadata("task1", map[string]interface{}{"share_folder_name": "Show"})

	svc.runPostTransferOrganize(context.Background(), "task1", req, &domain.Cloud115{ID: 1}, "123", []string{"fid1"})

	if organizer.callCount != 1 {
		t.Fatalf("期望 organizer 被调用 1 次，实际 %d", organizer.callCount)
	}
	select {
	case <-scraper.done:
	case <-time.After(2 * time.Second):
		t.Fatal("期望触发刮削（scraper 被调用）")
	}
	if findTaskByType(dao, "share_transfer_scrape") == nil {
		t.Fatal("期望创建 scrape 任务")
	}
}

// TestRunPostTransferScrape115Degrade 验证：115 源刮削降级为 completed + scrape_status=skipped。
func TestRunPostTransferScrape115Degrade(t *testing.T) {
	dao := newFakeTaskDAO()
	scraper := NewCloud115Scraper(nil, nil)
	svc := &ShareTransferService{taskDAO: dao, cloud115Scraper: scraper, localScraper: scraper}
	source := &domain.MediaSource{ID: 0, SourceType: domain.SourceTypeCloud115, Name: "转存整理", OrganizeTargetPath: "/d"}
	results := []OrganizeResult{{FileID: "f1", FileName: "x.mkv", Success: true, NewPath: "/电影/x.mkv"}}
	dao.Create("task1", "share_transfer", "转存")
	dao.UpdateMetadata("task1", map[string]interface{}{"share_folder_name": "Show"})

	svc.runPostTransferScrape(context.Background(), "task1", domain.TransferRequest{AutoScrape: true}, "org1", source, results)

	scrapeTask := findTaskByType(dao, "share_transfer_scrape")
	if scrapeTask == nil {
		t.Fatal("期望创建 scrape 任务")
	}
	if scrapeTask["status"] != "completed" {
		t.Fatalf("115 降级期望 completed，实际 %v", scrapeTask["status"])
	}
	meta := scrapeTask["metadata"].(map[string]interface{})
	if meta["scrape_status"] != "skipped" {
		t.Fatalf("期望 scrape_status=skipped，实际 %v", meta["scrape_status"])
	}
	// 转存任务元数据应回填 scrape_task_id
	task1, _ := dao.Get("task1")
	if task1["metadata"].(map[string]interface{})["scrape_task_id"] == "" {
		t.Fatal("期望回写 scrape_task_id")
	}
}

// ==================== 向后兼容回归测试 ====================

// mockCloud115Client 实现 Cloud115Client 接口的测试桩：
// 所有文件操作均成功返回，用于驱动 executeTransfer 完成一次完整转存。
type mockCloud115Client struct{}

func (m *mockCloud115Client) GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error) {
	return &driver.FileListResp{}, nil
}
func (m *mockCloud115Client) GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}
func (m *mockCloud115Client) GetCIDByPath(path string, cloud115ID int, cookie string) (string, error) {
	return "123", nil
}
func (m *mockCloud115Client) RenameFile(fileID, newName string, cloud115ID int, cookie string) error {
	return nil
}
func (m *mockCloud115Client) CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error {
	return nil
}
func (m *mockCloud115Client) MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error {
	return nil
}
func (m *mockCloud115Client) MkdirAll115(path string, cloud115ID int, cookie string) (string, error) {
	return "", nil
}
func (m *mockCloud115Client) RapidTransferFile(sourcePickCode string, sourceCloud115ID int, sourceCookie string, targetDirID string, targetCloud115ID int, targetCookie string, fileName string) (newPickCode string, err error) {
	return "", nil
}
func (m *mockCloud115Client) GetFileInfo(pickCode string, cloud115ID int, cookie string) (*driver.File, error) {
	return &driver.File{}, nil
}
func (m *mockCloud115Client) GetShareSnap(shareCode, receiveCode, dirID string, queries ...driver.Query) (*driver.ShareSnapResp, error) {
	return &driver.ShareSnapResp{}, nil
}
func (m *mockCloud115Client) ReceiveShare(shareCode, receiveCode, fileIDs, saveFolderID string, targetCloud115ID int, targetCookie string) error {
	return nil
}

// TestExecuteTransferAutoOrganizeDisabledDoesNotCreateChildTasks 守护向后兼容硬约束：
// 当 AutoOrganize=false 时，执行一次完整转存（executeTransfer）终态后，
// taskDAO 不应创建任何 share_transfer_organize / share_transfer_scrape 任务，
// 也不应调用 OrganizeService / Scraper；行为与原版 100% 一致。
// 覆盖两种子情形：两者都关（默认）、整理关但刮削开（刮削依赖整理，同样不应触发）。
func TestExecuteTransferAutoOrganizeDisabledDoesNotCreateChildTasks(t *testing.T) {
	cases := []struct {
		name         string
		autoOrganize bool
		autoScrape   bool
	}{
		{"both off (default backward-compat)", false, false},
		{"organize off but scrape on (scrape depends on organize)", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			daoStore := newFakeTaskDAO()
			client := &mockCloud115Client{}

			// logDAO 需要非 nil（executeTransfer 每个文件都会调用 UpdateStatus）。
			// 用 sqlmock 提供内存 DB；即便 sqlmock 不匹配也不影响转存完成（错误仅被记录）。
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New 失败: %v", err)
			}
			defer db.Close()
			mock.ExpectExec("UPDATE t_share_transfer_log").WillReturnResult(sqlmock.NewResult(1, 1))
			logDAO := dao.NewShareTransferLogDAO(db)

			svc := &ShareTransferService{
				taskDAO: daoStore,
				client:  client,
				logDAO:  logDAO,
			}

			req := domain.TransferRequest{
				ShareCode:        "abc123",
				TargetCloud115Id: 1,
				TargetDirectory:  "/d",
				Files:            []domain.ShareTransferFileItem{{Fid: "fid1", Name: "movie.2020.1080p.mkv"}},
				AutoOrganize:     c.autoOrganize,
				AutoScrape:       c.autoScrape,
			}

			svc.executeTransfer(context.Background(), "transfer-1", req, &domain.Cloud115{ID: 1})

			// 向后兼容断言：不得创建 organize / scrape 子任务
			if findTaskByType(daoStore, "share_transfer_organize") != nil {
				t.Fatal("AutoOrganize=false 时不应创建 share_transfer_organize 任务")
			}
			if findTaskByType(daoStore, "share_transfer_scrape") != nil {
				t.Fatal("AutoOrganize=false 时不应创建 share_transfer_scrape 任务")
			}
			// 强化断言：executeTransfer 本身不调用 taskDAO.Create，
			// 故整个 taskDAO 不应出现任何任务。
			if len(daoStore.tasks) != 0 {
				t.Fatalf("AutoOrganize=false 时 taskDAO 不应创建任何任务，实际创建了 %d 个", len(daoStore.tasks))
			}
		})
	}
}
