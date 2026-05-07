package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

type mockWatchTaskManager struct {
	createdTaskID   string
	createdTaskType string
	createdTaskName string
	statuses        []string
	progressCalls   [][4]int
	metadata        map[string]interface{}
	resumedTaskID   string
	errorMessage    string
}

func (m *mockWatchTaskManager) Create(taskID string, taskType, taskName string) error {
	m.createdTaskID = taskID
	m.createdTaskType = taskType
	m.createdTaskName = taskName
	return nil
}

func (m *mockWatchTaskManager) Get(taskID string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"task_id":   taskID,
		"task_type": watchAutoOrganizeTaskType,
		"metadata":  m.metadata,
	}, nil
}

func (m *mockWatchTaskManager) Resume(taskID string) error {
	m.resumedTaskID = taskID
	return nil
}

func (m *mockWatchTaskManager) UpdateStatus(taskID, status string) error {
	m.statuses = append(m.statuses, status)
	return nil
}

func (m *mockWatchTaskManager) UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error {
	m.progressCalls = append(m.progressCalls, [4]int{totalFiles, processedFiles, successFiles, failedFiles})
	return nil
}

func (m *mockWatchTaskManager) UpdateMetadata(taskID string, metadata map[string]interface{}) error {
	m.metadata = metadata
	return nil
}

func (m *mockWatchTaskManager) SetError(taskID, errMsg string) error {
	m.errorMessage = errMsg
	return nil
}

func TestVideoExtensionsCheck(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"movie.mp4", true},
		{"movie.mkv", true},
		{"movie.avi", true},
		{"movie.mov", true},
		{"movie.txt", false},
		{"movie.jpg", false},
		{"movie.srt", false},
		{"TV.Show.S01E01.mkv", true},
		{"movie.m2ts", true},
		{"movie.ts", true},
		{"movie.rmvb", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			ext := filepath.Ext(tt.filename)
			isVideo := videoExtensions[ext]
			if isVideo != tt.expected {
				t.Errorf("videoExtensions[%s] = %v, want %v", ext, isVideo, tt.expected)
			}
		})
	}
}

func TestCloud115FileSetDifference(t *testing.T) {
	oldSet := map[string]bool{
		"file1": true,
		"file2": true,
		"file3": true,
	}

	newSet := map[string]bool{
		"file1": true,
		"file2": true,
		"file4": true,
		"file5": true,
	}

	newFiles := []string{}
	for pickcode := range newSet {
		if !oldSet[pickcode] {
			newFiles = append(newFiles, pickcode)
		}
	}

	if len(newFiles) != 2 {
		t.Errorf("Expected 2 new files, got %d: %v", len(newFiles), newFiles)
	}
}

func TestWatchServiceLocalWatchState(t *testing.T) {
	source := &domain.MediaSource{
		ID:            1,
		Name:          "test-source",
		Path:          "C:\\test\\media",
		SourceType:    "local",
		WatchEnabled:  true,
		AutoOrganize:  true,
		WatchInterval: 60,
		EmbyLibraryID: "",
	}

	state := &localWatchState{
		source:   source,
		debounce: make(map[string]*time.Timer),
	}

	if state.source.ID != 1 {
		t.Errorf("Expected source ID 1, got %d", state.source.ID)
	}
	if state.source.Path != "C:\\test\\media" {
		t.Errorf("Expected path C:\\test\\media, got %s", state.source.Path)
	}
	if !state.source.WatchEnabled {
		t.Error("Expected watch enabled")
	}
	if !state.source.AutoOrganize {
		t.Error("Expected auto organize enabled")
	}
}

func TestWatchServiceCloud115WatchState(t *testing.T) {
	sourceID := 2
	source := &domain.MediaSource{
		ID:                 2,
		Name:               "cloud-source",
		Cloud115ID:         &sourceID,
		WatchInterval:      300,
		SourceType:         "cloud115",
		WatchEnabled:       true,
		OrganizeTargetPath: "/media/target",
	}

	state := &cloud115WatchState{
		source:     source,
		knownFiles: make(map[string]bool),
	}

	state.knownFiles["file1"] = true
	state.knownFiles["file2"] = true

	if len(state.knownFiles) != 2 {
		t.Errorf("Expected 2 known files, got %d", len(state.knownFiles))
	}
	if state.source.WatchInterval != 300 {
		t.Errorf("Expected watch interval 300, got %d", state.source.WatchInterval)
	}
	if *state.source.Cloud115ID != 2 {
		t.Errorf("Expected cloud115 ID 2, got %d", *state.source.Cloud115ID)
	}
}

func TestIntervalValidation(t *testing.T) {
	minInterval := defaultCloud115MinInterval

	if minInterval != 60 {
		t.Errorf("Expected min interval 60, got %d", minInterval)
	}

	tests := []struct {
		input    int
		expected bool
	}{
		{30, false},
		{60, true},
		{300, true},
		{1800, true},
		{0, false},
	}

	for _, tt := range tests {
		valid := tt.input >= minInterval
		if valid != tt.expected {
			t.Errorf("Interval %d validation = %v, want %v", tt.input, valid, tt.expected)
		}
	}
}

func TestDebounceInterval(t *testing.T) {
	interval := defaultDebounceInterval
	if interval != 30*time.Second {
		t.Errorf("Expected debounce interval 30s, got %v", interval)
	}
}

func TestMediaSourceWatchConfig(t *testing.T) {
	tests := []struct {
		name           string
		source         *domain.MediaSource
		shouldWatch    bool
		shouldOrganize bool
	}{
		{
			name: "local with watch enabled",
			source: &domain.MediaSource{
				ID:           1,
				SourceType:   "local",
				Path:         "/media",
				Enabled:      true,
				WatchEnabled: true,
				AutoOrganize: true,
			},
			shouldWatch:    true,
			shouldOrganize: true,
		},
		{
			name: "cloud115 with watch disabled",
			source: &domain.MediaSource{
				ID:           2,
				SourceType:   "cloud115",
				Cloud115ID:   func() *int { i := 1; return &i }(),
				Enabled:      true,
				WatchEnabled: false,
				AutoOrganize: true,
			},
			shouldWatch:    false,
			shouldOrganize: true,
		},
		{
			name: "disabled source",
			source: &domain.MediaSource{
				ID:           3,
				SourceType:   "local",
				Path:         "/media",
				Enabled:      false,
				WatchEnabled: true,
				AutoOrganize: true,
			},
			shouldWatch:    false,
			shouldOrganize: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.source.Enabled {
				shouldWatch := tt.source.WatchEnabled
				shouldOrganize := tt.source.AutoOrganize

				if shouldWatch != tt.shouldWatch {
					t.Errorf("shouldWatch = %v, want %v", shouldWatch, tt.shouldWatch)
				}
				if shouldOrganize != tt.shouldOrganize {
					t.Errorf("shouldOrganize = %v, want %v", shouldOrganize, tt.shouldOrganize)
				}
			} else {
				if tt.shouldWatch || tt.shouldOrganize {
					t.Errorf("Disabled source should have shouldWatch and shouldOrganize as false")
				}
			}
		})
	}
}

func TestCreateAutoOrganizeTaskForCloud115(t *testing.T) {
	manager := &mockWatchTaskManager{}
	sourceID := 2
	ws := &WatchService{
		taskManager: manager,
	}
	source := &domain.MediaSource{
		ID:         2,
		Name:       "cloud-source",
		SourceType: domain.SourceTypeCloud115,
		Cloud115ID: &sourceID,
	}

	taskID := ws.createAutoOrganizeTask(source, "", []string{"pickcode-1", "pickcode-2", "pickcode-3"})
	if taskID == "" {
		t.Fatal("expected task id to be created")
	}
	if manager.createdTaskType != watchAutoOrganizeTaskType {
		t.Fatalf("unexpected task type: got %s", manager.createdTaskType)
	}
	if manager.createdTaskName != "115自动整理-cloud-source" {
		t.Fatalf("unexpected task name: got %s", manager.createdTaskName)
	}
	if len(manager.progressCalls) != 1 {
		t.Fatalf("expected initial progress to be recorded once, got %d", len(manager.progressCalls))
	}
	if manager.progressCalls[0] != [4]int{3, 0, 0, 0} {
		t.Fatalf("unexpected initial progress: got %#v", manager.progressCalls[0])
	}
	if manager.metadata["source_name"] != "cloud-source" {
		t.Fatalf("unexpected source_name metadata: got %#v", manager.metadata["source_name"])
	}
	if manager.metadata["trigger_mode"] != "polling" {
		t.Fatalf("unexpected trigger_mode metadata: got %#v", manager.metadata["trigger_mode"])
	}
	if manager.metadata["detected_files"] != 3 {
		t.Fatalf("unexpected detected_files metadata: got %#v", manager.metadata["detected_files"])
	}
	if fileIDs, ok := manager.metadata["file_ids"].([]string); !ok || len(fileIDs) != 3 {
		t.Fatalf("unexpected file_ids metadata: got %#v", manager.metadata["file_ids"])
	}
}

func TestCreateAutoOrganizeTaskForLocalSource(t *testing.T) {
	manager := &mockWatchTaskManager{}
	ws := &WatchService{
		taskManager: manager,
	}
	source := &domain.MediaSource{
		ID:         1,
		Name:       "local-source",
		SourceType: domain.SourceTypeLocal,
	}

	taskID := ws.createAutoOrganizeTask(source, "C:\\media", []string{"file-1"})
	if taskID == "" {
		t.Fatal("expected task id to be created")
	}
	if manager.createdTaskName != "本地自动整理-local-source" {
		t.Fatalf("unexpected local task name: got %s", manager.createdTaskName)
	}
	if manager.metadata["trigger_mode"] != "fsnotify" {
		t.Fatalf("unexpected local trigger_mode metadata: got %#v", manager.metadata["trigger_mode"])
	}
	if manager.metadata["source_path"] != "C:\\media" {
		t.Fatalf("unexpected source_path metadata: got %#v", manager.metadata["source_path"])
	}
}

func TestBuildLocalWatchOrganizeTarget(t *testing.T) {
	source := &domain.MediaSource{
		Path:      `C:\media`,
		WatchPath: `C:\media\incoming`,
	}

	sourcePath, fileID, err := buildLocalWatchOrganizeTarget(source, `C:\media\incoming\Movie.2024.mkv`)
	if err != nil {
		t.Fatalf("expected local watch organize target to build successfully: %v", err)
	}
	if sourcePath != `incoming` {
		t.Fatalf("unexpected sourcePath: got %q", sourcePath)
	}
	if fileID != `incoming\Movie.2024.mkv` {
		t.Fatalf("unexpected fileID: got %q", fileID)
	}
}

func TestBuildLocalWatchOrganizeTargetForRootWatchPath(t *testing.T) {
	source := &domain.MediaSource{
		Path:      `C:\media`,
		WatchPath: `C:\media`,
	}

	sourcePath, fileID, err := buildLocalWatchOrganizeTarget(source, `C:\media\Movie.2024.mkv`)
	if err != nil {
		t.Fatalf("expected root watch path to build successfully: %v", err)
	}
	if sourcePath != "" {
		t.Fatalf("expected empty sourcePath for root watch path, got %q", sourcePath)
	}
	if fileID != `Movie.2024.mkv` {
		t.Fatalf("unexpected fileID: got %q", fileID)
	}
}

func TestBuildLocalWatchOrganizeTargetRejectsOutsideFile(t *testing.T) {
	source := &domain.MediaSource{
		Path:      `C:\media`,
		WatchPath: `C:\media\incoming`,
	}

	_, _, err := buildLocalWatchOrganizeTarget(source, `C:\other\Movie.2024.mkv`)
	if err == nil {
		t.Fatal("expected outside file to be rejected")
	}
}

func TestResolveRetrySourcePathPreservesEmptyRootForLocalWatch(t *testing.T) {
	source := &domain.MediaSource{
		SourceType: domain.SourceTypeLocal,
		Path:       `C:\media`,
		WatchPath:  `C:\media`,
	}

	actual := resolveRetrySourcePath(map[string]interface{}{
		"watch_path":  "",
		"source_path": "",
	}, source, []string{`Inception.2010.1080p.mkv`})

	if actual != "" {
		t.Fatalf("expected empty retry source path to be preserved, got %q", actual)
	}
}

func TestResolveRetrySourcePathFallsBackToRelativeDirForLocalFile(t *testing.T) {
	source := &domain.MediaSource{
		SourceType: domain.SourceTypeLocal,
		Path:       `C:\media`,
		WatchPath:  `C:\media\incoming`,
	}

	actual := resolveRetrySourcePath(nil, source, []string{`incoming\Inception.2010.1080p.mkv`})
	if actual != `incoming` {
		t.Fatalf("expected retry source path to use file directory, got %q", actual)
	}
}

func TestWatchTaskHelpersIgnoreNilManager(t *testing.T) {
	ws := &WatchService{}

	ws.updateTaskStatus("task-1", "running")
	ws.updateTaskProgress("task-1", 1, 1, 1, 0)
	ws.setTaskError("task-1", "boom")
}

func TestSummarizeAutoOrganizeFailure(t *testing.T) {
	category, reason := summarizeAutoOrganizeFailure(filepath.ErrBadPattern)
	if category != "organize_failed" {
		t.Fatalf("unexpected category: got %s", category)
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestSummarizeAutoOrganizeResults(t *testing.T) {
	results := []OrganizeResult{
		{FileName: "a.mkv", Success: false, Message: "identify failed: no match found"},
		{FileName: "b.mkv", Success: false, Message: "tmdb identify failed: unknown year"},
	}

	category, reason := summarizeAutoOrganizeResults(results)
	if category != "identify_failed" {
		t.Fatalf("unexpected category: got %s", category)
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestSummarizeAutoOrganizeResultsConflictAndPartial(t *testing.T) {
	tests := []struct {
		name     string
		results  []OrganizeResult
		category string
	}{
		{
			name: "conflict skipped",
			results: []OrganizeResult{
				{FileName: "a.mkv", Success: false, Message: "conflict exists"},
			},
			category: "conflict_skipped",
		},
		{
			name: "partial failed",
			results: []OrganizeResult{
				{FileName: "a.mkv", Success: false, Message: "identify failed"},
				{FileName: "b.mkv", Success: false, Message: "move failed"},
			},
			category: "partial_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, reason := summarizeAutoOrganizeResults(tt.results)
			if category != tt.category {
				t.Fatalf("unexpected category: got %s want %s", category, tt.category)
			}
			if reason == "" {
				t.Fatal("expected non-empty reason")
			}
		})
	}
}

func TestSummarizeAutoOrganizeCloud115AuthFailure(t *testing.T) {
	category, reason := summarizeAutoOrganizeFailure(fmt.Errorf("parse cookie failed: invalid cookie"))
	if category != "cloud115_auth_failed" {
		t.Fatalf("unexpected category: got %s", category)
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}

	results := []OrganizeResult{
		{FileName: "a.mkv", Success: false, Message: "parse cookie failed: invalid cookie"},
	}
	category, reason = summarizeAutoOrganizeResults(results)
	if category != "cloud115_auth_failed" {
		t.Fatalf("unexpected result category: got %s", category)
	}
	if reason == "" {
		t.Fatal("expected non-empty result reason")
	}
}

func TestSummarizeAutoOrganizeFailureMappings(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		category string
		reason   string
	}{
		{
			name:     "target path",
			err:      fmt.Errorf("target path missing"),
			category: "target_path",
			reason:   "target path missing",
		},
		{
			name:     "scan failed",
			err:      fmt.Errorf("scan file list failed"),
			category: "scan_failed",
			reason:   "scan file list failed",
		},
		{
			name:     "cloud115 auth failed",
			err:      fmt.Errorf("parse cookie failed: invalid cookie"),
			category: "cloud115_auth_failed",
			reason:   "115 账号 Cookie 已失效，请重新登录",
		},
		{
			name:     "cloud115 failed",
			err:      fmt.Errorf("115 request failed"),
			category: "cloud115_failed",
			reason:   "115 request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, reason := summarizeAutoOrganizeFailure(tt.err)
			if category != tt.category {
				t.Fatalf("unexpected category: got %s want %s", category, tt.category)
			}
			if reason != tt.reason {
				t.Fatalf("unexpected reason: got %q want %q", reason, tt.reason)
			}
		})
	}
}

func TestCloud115AuthFailureMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "cookie invalid", message: "cookie 无效", want: true},
		{name: "account expired", message: "115账号失效，请重新登录", want: true},
		{name: "account absent", message: "账号不存在", want: true},
		{name: "unrelated", message: "network timeout", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCloud115AuthFailureMessage(tt.message); got != tt.want {
				t.Fatalf("unexpected match result: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestResolveWatchOrganizeDefaults(t *testing.T) {
	tests := []struct {
		name          string
		source        *domain.MediaSource
		wantMediaType string
		wantPolicy    string
		wantMode      string
	}{
		{
			name: "cloud115 coerces hardlink to move",
			source: &domain.MediaSource{
				SourceType:     domain.SourceTypeCloud115,
				MediaType:      "movie",
				ConflictPolicy: "overwrite",
				OperationMode:  "hardlink",
			},
			wantMediaType: "movie",
			wantPolicy:    "overwrite",
			wantMode:      "move",
		},
		{
			name: "local keeps symlink",
			source: &domain.MediaSource{
				SourceType:     domain.SourceTypeLocal,
				MediaType:      "tv",
				ConflictPolicy: "skip",
				OperationMode:  "symlink",
			},
			wantMediaType: "tv",
			wantPolicy:    "skip",
			wantMode:      "symlink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mediaType, policy, mode := resolveWatchOrganizeDefaults(tt.source)
			if mediaType != tt.wantMediaType {
				t.Fatalf("unexpected mediaType: got %s want %s", mediaType, tt.wantMediaType)
			}
			if policy != tt.wantPolicy {
				t.Fatalf("unexpected policy: got %s want %s", policy, tt.wantPolicy)
			}
			if mode != tt.wantMode {
				t.Fatalf("unexpected mode: got %s want %s", mode, tt.wantMode)
			}
		})
	}
}

func TestBuildWatchFailureItems(t *testing.T) {
	results := []OrganizeResult{
		{FileID: "fid-1", FileName: "a.mkv", Success: false, Message: "identify failed: no match found"},
		{FileID: "fid-2", FileName: "b.mkv", Success: true, Message: "ok"},
	}

	items := buildWatchFailureItems(results)
	if len(items) != 1 {
		t.Fatalf("unexpected items len: %d", len(items))
	}
	if items[0].FileID != "fid-1" || items[0].FileName != "a.mkv" {
		t.Fatalf("unexpected item: %#v", items[0])
	}
	if items[0].Category != "identify_failed" {
		t.Fatalf("unexpected item category: %#v", items[0].Category)
	}
}

func TestBuildWatchFailureItemsFromIDs(t *testing.T) {
	items := buildWatchFailureItemsFromIDs([]string{"fid-1", "", "fid-2"}, "parse cookie failed: invalid cookie", "cloud115_auth_failed")
	if len(items) != 2 {
		t.Fatalf("unexpected items len: %d", len(items))
	}
	if items[0].FileID != "fid-1" || items[0].Category != "cloud115_auth_failed" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].FileID != "fid-2" || items[1].Reason != "parse cookie failed: invalid cookie" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
}

func TestUpdateTaskResultMetadata(t *testing.T) {
	manager := &mockWatchTaskManager{}
	ws := &WatchService{
		taskManager: manager,
	}
	source := &domain.MediaSource{
		ID:                 3,
		Name:               "watch-source",
		SourceType:         domain.SourceTypeCloud115,
		OrganizeTargetPath: "/media",
		MediaType:          "movie",
		ConflictPolicy:     "skip",
		OperationMode:      "move",
		WatchInterval:      1800,
	}

	results := []OrganizeResult{
		{FileID: "fid-1", FileName: "a.mkv", Success: true},
		{FileID: "fid-2", FileName: "b.mkv", Success: false, Message: "identify failed: no match found"},
	}
	failedItems := buildWatchFailureItems(results)

	ws.updateTaskResultMetadata(
		"task-1",
		source,
		"0",
		[]string{"fid-1", "fid-2"},
		2,
		1,
		1,
		"identify_failed",
		"identify failed: no match found",
		failedItems,
	)

	if manager.metadata["source_id"] != 3 {
		t.Fatalf("unexpected source_id: %#v", manager.metadata["source_id"])
	}
	if manager.metadata["success_files"] != 1 {
		t.Fatalf("unexpected success_files: %#v", manager.metadata["success_files"])
	}
	if manager.metadata["failed_files"] != 1 {
		t.Fatalf("unexpected failed_files: %#v", manager.metadata["failed_files"])
	}
	if manager.metadata["failure_category"] != "identify_failed" {
		t.Fatalf("unexpected failure_category: %#v", manager.metadata["failure_category"])
	}
	if manager.metadata["failure_reason"] != "identify failed: no match found" {
		t.Fatalf("unexpected failure_reason: %#v", manager.metadata["failure_reason"])
	}
	if manager.metadata["failed_item_count"] != 1 {
		t.Fatalf("unexpected failed_item_count: %#v", manager.metadata["failed_item_count"])
	}
	if items, ok := manager.metadata["failed_items"].([]watchFailureItem); !ok || len(items) != 1 {
		t.Fatalf("unexpected failed_items: %#v", manager.metadata["failed_items"])
	} else if items[0].Category != "identify_failed" {
		t.Fatalf("unexpected failed item category: %#v", items[0].Category)
	}
}

