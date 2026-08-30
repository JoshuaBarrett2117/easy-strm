package service

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/fsnotify/fsnotify"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

var videoExtensions = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".m4v":  true,
	".ts":   true,
	".m2ts": true,
	".rmvb": true,
	".rm":   true,
	".mpg":  true,
	".mpeg": true,
}

const defaultDebounceInterval = 30 * time.Second
const defaultCloud115MinInterval = 60
const cloud115WatchPageSize = 1000
const watchAutoOrganizeTaskType = "watch_auto_organize"

type watchTaskManager interface {
	Create(taskID string, taskType, taskName string) error
	Get(taskID string) (map[string]interface{}, error)
	Resume(taskID string) error
	UpdateStatus(taskID, status string) error
	UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error
	UpdateMetadata(taskID string, metadata map[string]interface{}) error
	SetError(taskID, errMsg string) error
}

type WatchService struct {
	mediaSourceService *MediaSourceService
	organizeService    *OrganizeService
	cloud115DAO        *dao.Cloud115DAO
	client             Cloud115Client
	taskManager        watchTaskManager

	watcher         *fsnotify.Watcher
	localWatches    map[int]*localWatchState
	cloud115Watches map[int]*cloud115WatchState
	mu              sync.RWMutex
	stopCh          chan struct{}
}

type localWatchState struct {
	source      *domain.MediaSource
	debounce    map[string]*time.Timer
	watchedDirs map[string]struct{}
	debounceMu  sync.Mutex
}

type cloud115WatchState struct {
	source     *domain.MediaSource
	ticker     *time.Ticker
	stopCh     chan struct{}
	knownFiles map[string]cloud115WatchFile
}

type cloud115WatchFile struct {
	FileID   string
	FileName string
}

type watchFailureItem struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason"`
}

func resolveWatchPath(source *domain.MediaSource) string {
	if source == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(source.WatchPath); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(source.Path)
}

func collectLocalWatchTree(root string) ([]string, []string, error) {
	directories := make([]string, 0)
	videos := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if videoExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			videos = append(videos, path)
		}
		return nil
	})
	return directories, videos, err
}

func collectCloud115VideoFileSet(rootCID int, listPage func(cid, offset, limit int) (*driver.FileListResp, error)) (map[string]cloud115WatchFile, error) {
	type pendingDirectory struct {
		cid          int
		relativePath string
	}

	fileSet := make(map[string]cloud115WatchFile)
	pending := []pendingDirectory{{cid: rootCID}}
	visited := make(map[int]bool)

	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		cid := current.cid
		if visited[cid] {
			continue
		}
		visited[cid] = true

		for offset := 0; ; offset += cloud115WatchPageSize {
			page, err := listPage(cid, offset, cloud115WatchPageSize)
			if err != nil {
				return nil, err
			}
			if page == nil || len(page.Files) == 0 {
				break
			}

			for _, file := range page.Files {
				if file.FileID == "" || file.Type == "folder" {
					directoryID, parseErr := strconv.Atoi(string(file.CategoryID))
					if parseErr != nil {
						return nil, fmt.Errorf("115 子目录ID无效: name=%s, cid=%s", file.Name, file.CategoryID)
					}
					pending = append(pending, pendingDirectory{
						cid:          directoryID,
						relativePath: path.Join(current.relativePath, file.Name),
					})
					continue
				}
				if !videoExtensions[strings.ToLower(filepath.Ext(file.Name))] {
					continue
				}
				pickcode := file.PickCode
				if pickcode == "" {
					pickcode = file.FileID
				}
				if pickcode != "" {
					fileSet[pickcode] = cloud115WatchFile{
						FileID:   path.Join(current.relativePath, file.Name),
						FileName: file.Name,
					}
				}
			}

			if len(page.Files) < cloud115WatchPageSize {
				break
			}
		}
	}

	return fileSet, nil
}

func findNewCloud115FileIDs(knownFiles, currentFiles map[string]cloud115WatchFile) []string {
	newFiles := make([]string, 0)
	for identity, file := range currentFiles {
		if _, exists := knownFiles[identity]; exists {
			continue
		}
		if file.FileID != "" {
			newFiles = append(newFiles, file.FileID)
		}
	}
	sort.Strings(newFiles)
	return newFiles
}

func resolveCloud115WatchFileIDs(fileIDs []string, currentFiles map[string]cloud115WatchFile) []string {
	resolved := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if file, exists := currentFiles[fileID]; exists && file.FileID != "" {
			resolved = append(resolved, file.FileID)
			continue
		}
		resolved = append(resolved, fileID)
	}
	return resolved
}

func NewWatchService(
	mediaSourceService *MediaSourceService,
	organizeService *OrganizeService,
	cloud115DAO *dao.Cloud115DAO,
	client Cloud115Client,
	taskManager watchTaskManager,
) *WatchService {
	return &WatchService{
		mediaSourceService: mediaSourceService,
		organizeService:    organizeService,
		cloud115DAO:        cloud115DAO,
		client:             client,
		taskManager:        taskManager,
		localWatches:       make(map[int]*localWatchState),
		cloud115Watches:    make(map[int]*cloud115WatchState),
		stopCh:             make(chan struct{}),
	}
}

func (ws *WatchService) createAutoOrganizeTask(source *domain.MediaSource, sourcePath string, fileIDs []string) string {
	if ws.taskManager == nil || source == nil {
		return ""
	}

	taskID := fmt.Sprintf("watch_auto_organize_%d_%d", source.ID, time.Now().UnixNano())
	taskName := fmt.Sprintf("115自动整理-%s", source.Name)
	if source.SourceType == domain.SourceTypeLocal {
		taskName = fmt.Sprintf("本地自动整理-%s", source.Name)
	}

	if err := ws.taskManager.Create(taskID, watchAutoOrganizeTaskType, taskName); err != nil {
		logger.Warnf("[WatchService] create auto organize task failed: source_id=%d, error=%v", source.ID, err)
		return ""
	}

	ws.updateTaskMetadata(taskID, source, sourcePath, fileIDs)
	ws.updateTaskProgress(taskID, len(fileIDs), 0, 0, 0)
	return taskID
}

func (ws *WatchService) updateTaskStatus(taskID, status string) {
	if ws.taskManager == nil || taskID == "" {
		return
	}
	if err := ws.taskManager.UpdateStatus(taskID, status); err != nil {
		logger.Warnf("[WatchService] update task status failed: task_id=%s, status=%s, error=%v", taskID, status, err)
	}
}

func (ws *WatchService) updateTaskProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) {
	if ws.taskManager == nil || taskID == "" {
		return
	}
	if err := ws.taskManager.UpdateProgress(taskID, totalFiles, processedFiles, successFiles, failedFiles); err != nil {
		logger.Warnf("[WatchService] update task progress failed: task_id=%s, error=%v", taskID, err)
	}
}

func (ws *WatchService) updateTaskMetadata(taskID string, source *domain.MediaSource, sourcePath string, fileIDs []string) {
	if ws.taskManager == nil || taskID == "" || source == nil {
		return
	}

	mediaType, conflictPolicy, operationMode := resolveWatchOrganizeDefaults(source)
	triggerMode := "fsnotify"
	if source.SourceType == domain.SourceTypeCloud115 {
		triggerMode = "polling"
	}

	metadata := map[string]interface{}{
		"source_id":            source.ID,
		"source_name":          source.Name,
		"source_type":          source.SourceType,
		"organize_target_path": source.OrganizeTargetPath,
		"watch_path":           sourcePath,
		"media_type":           mediaType,
		"conflict_policy":      conflictPolicy,
		"operation_mode":       operationMode,
		"watch_interval":       source.WatchInterval,
		"trigger_mode":         triggerMode,
		"source_path":          sourcePath,
		"file_ids":             fileIDs,
		"detected_files":       len(fileIDs),
	}

	if err := ws.taskManager.UpdateMetadata(taskID, metadata); err != nil {
		logger.Warnf("[WatchService] update task metadata failed: task_id=%s, error=%v", taskID, err)
	}
}

func (ws *WatchService) updateTaskResultMetadata(taskID string, source *domain.MediaSource, sourcePath string, fileIDs []string, total, success, failed int, category, reason string, failedItems []watchFailureItem) {
	if ws.taskManager == nil || taskID == "" || source == nil {
		return
	}

	mediaType, conflictPolicy, operationMode := resolveWatchOrganizeDefaults(source)
	triggerMode := "fsnotify"
	if source.SourceType == domain.SourceTypeCloud115 {
		triggerMode = "polling"
	}

	metadata := map[string]interface{}{
		"source_id":            source.ID,
		"source_name":          source.Name,
		"source_type":          source.SourceType,
		"organize_target_path": source.OrganizeTargetPath,
		"watch_path":           sourcePath,
		"media_type":           mediaType,
		"conflict_policy":      conflictPolicy,
		"operation_mode":       operationMode,
		"watch_interval":       source.WatchInterval,
		"trigger_mode":         triggerMode,
		"source_path":          sourcePath,
		"file_ids":             fileIDs,
		"detected_files":       total,
		"success_files":        success,
		"failed_files":         failed,
		"result_summary":       fmt.Sprintf("成功 %d，失败 %d", success, failed),
	}

	if category != "" {
		metadata["failure_category"] = category
	}
	if reason != "" {
		metadata["failure_reason"] = reason
	}
	if len(failedItems) > 0 {
		metadata["failed_items"] = failedItems
		metadata["failed_item_count"] = len(failedItems)
	}

	if err := ws.taskManager.UpdateMetadata(taskID, metadata); err != nil {
		logger.Warnf("[WatchService] update task result metadata failed: task_id=%s, error=%v", taskID, err)
	}
}

func (ws *WatchService) setTaskError(taskID, errMsg string) {
	if ws.taskManager == nil || taskID == "" {
		return
	}
	if err := ws.taskManager.SetError(taskID, errMsg); err != nil {
		logger.Warnf("[WatchService] set task error failed: task_id=%s, error=%v", taskID, err)
	}
}

func (ws *WatchService) StartAll() {
	sources, err := ws.mediaSourceService.GetWatchEnabled()
	if err != nil {
		logger.Errorf("[WatchService] load watch enabled sources failed: %v", err)
		return
	}

	for _, source := range sources {
		if err := ws.StartWatching(source.ID); err != nil {
			logger.Errorf("[WatchService] start watching failed: source_id=%d, name=%s, error=%v", source.ID, source.Name, err)
		}
	}
}

func (ws *WatchService) StopAll() {
	select {
	case <-ws.stopCh:
	default:
		close(ws.stopCh)
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	for id, state := range ws.localWatches {
		state.debounceMu.Lock()
		for _, timer := range state.debounce {
			timer.Stop()
		}
		state.debounceMu.Unlock()
		delete(ws.localWatches, id)
	}

	for id, state := range ws.cloud115Watches {
		state.ticker.Stop()
		close(state.stopCh)
		delete(ws.cloud115Watches, id)
	}

	if ws.watcher != nil {
		_ = ws.watcher.Close()
		ws.watcher = nil
	}
}

func (ws *WatchService) StartWatching(sourceID int) error {
	source, err := ws.mediaSourceService.GetByID(sourceID)
	if err != nil || source == nil {
		return fmt.Errorf("获取媒体源失败或不存在: id=%d", sourceID)
	}
	if !source.Enabled {
		return fmt.Errorf("媒体源未启用: id=%d", sourceID)
	}
	if !source.WatchEnabled {
		return fmt.Errorf("媒体源未开启监控: id=%d", sourceID)
	}

	switch source.SourceType {
	case domain.SourceTypeLocal:
		return ws.startLocalWatching(source)
	case domain.SourceTypeCloud115:
		return ws.startCloud115Watching(source)
	default:
		return fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
	}
}

// IsWatching 返回指定媒体源是否已在当前进程中实际运行监控。
func (ws *WatchService) IsWatching(sourceID int) bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	if _, exists := ws.localWatches[sourceID]; exists {
		return true
	}
	_, exists := ws.cloud115Watches[sourceID]
	return exists
}

func (ws *WatchService) StopWatching(sourceID int) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if state, ok := ws.localWatches[sourceID]; ok {
		state.debounceMu.Lock()
		for _, timer := range state.debounce {
			timer.Stop()
		}
		state.debounceMu.Unlock()
		if ws.watcher != nil {
			for directory := range state.watchedDirs {
				usedByAnotherSource := false
				for otherID, otherState := range ws.localWatches {
					if otherID == sourceID {
						continue
					}
					if _, exists := otherState.watchedDirs[directory]; exists {
						usedByAnotherSource = true
						break
					}
				}
				if !usedByAnotherSource {
					_ = ws.watcher.Remove(directory)
				}
			}
		}
		delete(ws.localWatches, sourceID)
	}

	if state, ok := ws.cloud115Watches[sourceID]; ok {
		state.ticker.Stop()
		close(state.stopCh)
		delete(ws.cloud115Watches, sourceID)
	}
}

func (ws *WatchService) startLocalWatching(source *domain.MediaSource) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if _, ok := ws.localWatches[source.ID]; ok {
		return nil
	}

	if ws.watcher == nil {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("创建 fsnotify watcher 失败: %v", err)
		}
		ws.watcher = watcher
		go ws.handleLocalEvents()
	}

	watchPath := resolveWatchPath(source)
	info, err := os.Stat(watchPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("目录不存在或不可访问: %s", watchPath)
	}

	directories, _, err := collectLocalWatchTree(watchPath)
	if err != nil {
		return fmt.Errorf("扫描监控目录失败: %s, error: %v", watchPath, err)
	}
	addedDirectories := make([]string, 0, len(directories))
	for _, directory := range directories {
		if err := ws.watcher.Add(directory); err != nil {
			for _, addedDirectory := range addedDirectories {
				_ = ws.watcher.Remove(addedDirectory)
			}
			return fmt.Errorf("添加监控目录失败: %s, error: %v", directory, err)
		}
		addedDirectories = append(addedDirectories, directory)
	}

	watchedDirs := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		watchedDirs[directory] = struct{}{}
	}
	ws.localWatches[source.ID] = &localWatchState{
		source:      source,
		debounce:    make(map[string]*time.Timer),
		watchedDirs: watchedDirs,
	}
	logger.Infof("[WatchService] local watch started: source_id=%d, directories=%d", source.ID, len(directories))
	return nil
}

func (ws *WatchService) handleLocalEvents() {
	for {
		select {
		case <-ws.stopCh:
			return
		case event, ok := <-ws.watcher.Events:
			if !ok {
				return
			}
			ws.processLocalEvent(event)
		case err, ok := <-ws.watcher.Errors:
			if !ok {
				return
			}
			logger.Errorf("[WatchService] fsnotify error: %v", err)
		}
	}
}

func (ws *WatchService) processLocalEvent(event fsnotify.Event) {
	if event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Write == 0 {
		return
	}

	filePath := event.Name
	ws.mu.RLock()
	var matchedState *localWatchState
	for _, state := range ws.localWatches {
		if isPathWithin(resolveWatchPath(state.source), filePath) {
			matchedState = state
			break
		}
	}
	ws.mu.RUnlock()

	if matchedState == nil {
		return
	}
	if info, err := os.Stat(filePath); err == nil && info.IsDir() {
		ws.registerLocalDirectoryTree(matchedState.source.ID, filePath)
		return
	}
	if !videoExtensions[strings.ToLower(filepath.Ext(filePath))] {
		return
	}

	ws.scheduleLocalFile(matchedState, filePath)
}

func isPathWithin(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func (ws *WatchService) registerLocalDirectoryTree(sourceID int, root string) {
	directories, videos, err := collectLocalWatchTree(root)
	if err != nil {
		logger.Warnf("[WatchService] scan new local directory failed: source_id=%d, path=%s, error=%v", sourceID, root, err)
		return
	}

	ws.mu.Lock()
	state, exists := ws.localWatches[sourceID]
	if !exists || ws.watcher == nil {
		ws.mu.Unlock()
		return
	}
	for _, directory := range directories {
		if _, exists := state.watchedDirs[directory]; exists {
			continue
		}
		if err := ws.watcher.Add(directory); err != nil {
			logger.Warnf("[WatchService] add new local directory failed: source_id=%d, path=%s, error=%v", sourceID, directory, err)
			continue
		}
		state.watchedDirs[directory] = struct{}{}
	}
	ws.mu.Unlock()

	for _, video := range videos {
		ws.scheduleLocalFile(state, video)
	}
}

func (ws *WatchService) scheduleLocalFile(state *localWatchState, filePath string) {
	if state == nil {
		return
	}
	source := state.source
	state.debounceMu.Lock()
	if timer, exists := state.debounce[filePath]; exists {
		timer.Stop()
	}
	state.debounce[filePath] = time.AfterFunc(defaultDebounceInterval, func() {
		state.debounceMu.Lock()
		delete(state.debounce, filePath)
		state.debounceMu.Unlock()
		ws.processNewLocalFile(source, filePath)
	})
	state.debounceMu.Unlock()
}

func (ws *WatchService) processNewLocalFile(source *domain.MediaSource, filePath string) {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return
	}
	if !source.AutoOrganize {
		return
	}

	organizeSourcePath, organizeFileID, err := buildLocalWatchOrganizeTarget(source, filePath)
	if err != nil {
		logger.Warnf("[WatchService] build local watch organize target failed: source_id=%d, file=%s, error=%v", source.ID, filePath, err)
		return
	}

	ws.triggerAutoOrganize(source, organizeSourcePath, []string{organizeFileID})
}

func (ws *WatchService) startCloud115Watching(source *domain.MediaSource) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if _, ok := ws.cloud115Watches[source.ID]; ok {
		return nil
	}
	if source.Cloud115ID == nil {
		return fmt.Errorf("115 媒体源未关联账号: source_id=%d", source.ID)
	}

	interval := source.WatchInterval
	if interval < defaultCloud115MinInterval {
		interval = defaultCloud115MinInterval
	}

	knownFiles, err := ws.fetchCloud115FileSet(source)
	if err != nil {
		return fmt.Errorf("初始化115监控目录失败: %w", err)
	}

	state := &cloud115WatchState{
		source:     source,
		ticker:     time.NewTicker(time.Duration(interval) * time.Second),
		stopCh:     make(chan struct{}),
		knownFiles: knownFiles,
	}
	ws.cloud115Watches[source.ID] = state
	go ws.cloud115PollLoop(state)
	logger.Infof("[WatchService] cloud115 watch started: source_id=%d, known_files=%d, interval=%d", source.ID, len(knownFiles), interval)
	return nil
}

func (ws *WatchService) cloud115PollLoop(state *cloud115WatchState) {
	for {
		select {
		case <-state.stopCh:
			return
		case <-ws.stopCh:
			return
		case <-state.ticker.C:
			ws.pollCloud115Directory(state)
		}
	}
}

func (ws *WatchService) pollCloud115Directory(state *cloud115WatchState) {
	source := state.source
	latestSource, err := ws.mediaSourceService.GetByID(source.ID)
	if err != nil || latestSource == nil {
		return
	}
	if !latestSource.Enabled || !latestSource.WatchEnabled {
		ws.StopWatching(source.ID)
		return
	}

	currentFiles, err := ws.fetchCloud115FileSet(latestSource)
	if err != nil {
		logger.Warnf("[WatchService] poll cloud115 files failed: source_id=%d, error=%v", source.ID, err)
		return
	}

	newFiles := findNewCloud115FileIDs(state.knownFiles, currentFiles)
	state.knownFiles = currentFiles

	if len(newFiles) == 0 || !latestSource.AutoOrganize {
		return
	}
	ws.triggerAutoOrganize(latestSource, resolveWatchPath(latestSource), newFiles)
}

func (ws *WatchService) fetchCloud115FileSet(source *domain.MediaSource) (map[string]cloud115WatchFile, error) {
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("未关联 115 账号")
	}

	cloud115, err := ws.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("115 账号不存在: cloud115_id=%d", *source.Cloud115ID)
	}

	cidStr := resolveWatchPath(source)
	if cidStr == "" || cidStr == "/" {
		cidStr = "0"
	} else if strings.HasPrefix(cidStr, "/") {
		cidStr, err = ws.client.GetCIDByPath(cidStr, cloud115.ID, cloud115.Cookie)
		if err != nil {
			return nil, fmt.Errorf("解析 115 监控目录失败: %v", err)
		}
	}
	cid, err := strconv.Atoi(cidStr)
	if err != nil {
		return nil, fmt.Errorf("115 监控目录解析结果无效")
	}

	fileSet, err := collectCloud115VideoFileSet(cid, func(currentCID, offset, limit int) (*driver.FileListResp, error) {
		return ws.client.GetFileList(currentCID, 1, offset, limit, cloud115.ID, cloud115.Cookie)
	})
	if err != nil {
		return nil, fmt.Errorf("获取 115 文件列表失败: %v", err)
	}
	return fileSet, nil
}

func buildLocalWatchOrganizeTarget(source *domain.MediaSource, filePath string) (string, string, error) {
	if source == nil {
		return "", "", fmt.Errorf("媒体源不能为空")
	}

	basePath := filepath.Clean(source.Path)
	absFilePath := filepath.Clean(filePath)
	watchPath := filepath.Clean(resolveWatchPath(source))

	relativeFileID, err := filepath.Rel(basePath, absFilePath)
	if err != nil {
		return "", "", fmt.Errorf("计算文件相对路径失败: %w", err)
	}
	if relativeFileID == "." || strings.HasPrefix(relativeFileID, "..") {
		return "", "", fmt.Errorf("文件不在媒体源目录内: %s", filePath)
	}

	relativeSourcePath, err := filepath.Rel(basePath, watchPath)
	if err != nil {
		return "", "", fmt.Errorf("计算监控目录相对路径失败: %w", err)
	}
	if relativeSourcePath == "." {
		relativeSourcePath = ""
	}
	if strings.HasPrefix(relativeSourcePath, "..") {
		return "", "", fmt.Errorf("监控目录不在媒体源目录内: %s", watchPath)
	}

	return relativeSourcePath, relativeFileID, nil
}

func (ws *WatchService) triggerAutoOrganize(source *domain.MediaSource, sourcePath string, fileIDs []string) {
	if len(fileIDs) == 0 {
		return
	}
	targetPath := source.OrganizeTargetPath
	if targetPath == "" {
		logger.Warnf("[WatchService] auto organize skipped because target path is empty: source_id=%d", source.ID)
		return
	}

	go func() {
		taskID := ws.createAutoOrganizeTask(source, sourcePath, fileIDs)
		if taskID != "" {
			ws.updateTaskStatus(taskID, "running")
		}

		defer func() {
			if r := recover(); r != nil {
				reason := fmt.Sprintf("自动整理异常: %v", r)
				if taskID != "" {
					ws.updateTaskResultMetadata(taskID, source, sourcePath, fileIDs, len(fileIDs), 0, len(fileIDs), "panic", reason, buildWatchFailureItemsFromIDs(fileIDs, reason, "panic"))
					ws.setTaskError(taskID, reason)
				}
				logger.Errorf("[WatchService] auto organize panic: source_id=%d, error=%v", source.ID, r)
			}
		}()

		mediaType, conflictPolicy, operationMode := resolveWatchOrganizeDefaults(source)
		results, err := ws.organizeService.OrganizeDirectory(
			source.ID,
			sourcePath,
			targetPath,
			mediaType,
			"",
			conflictPolicy,
			operationMode,
			fileIDs,
			true,
			nil,
			nil,
		)
		if err != nil {
			if taskID != "" {
				category, reason := summarizeAutoOrganizeFailure(err)
				ws.updateTaskResultMetadata(taskID, source, sourcePath, fileIDs, len(fileIDs), 0, len(fileIDs), category, reason, buildWatchFailureItemsFromIDs(fileIDs, reason, category))
				ws.setTaskError(taskID, reason)
			}
			return
		}

		successCount := 0
		failedCount := 0
		for _, result := range results {
			if result.Success {
				successCount++
			} else {
				failedCount++
			}
		}

		if taskID != "" {
			category, reason := summarizeAutoOrganizeResults(results)
			ws.updateTaskResultMetadata(taskID, source, sourcePath, fileIDs, len(results), successCount, failedCount, category, reason, buildWatchFailureItems(results))
			ws.updateTaskProgress(taskID, len(results), len(results), successCount, failedCount)
			if failedCount > 0 && successCount == 0 {
				ws.setTaskError(taskID, reason)
				return
			}
			ws.updateTaskStatus(taskID, "completed")
		}
	}()
}

func (ws *WatchService) RetryAutoOrganizeTask(taskID string) error {
	if ws.taskManager == nil {
		return fmt.Errorf("task manager is not initialized")
	}

	task, err := ws.taskManager.Get(taskID)
	if err != nil {
		return fmt.Errorf("获取任务失败: %v", err)
	}
	if task == nil {
		return fmt.Errorf("任务不存在")
	}

	taskType, _ := task["task_type"].(string)
	if taskType != watchAutoOrganizeTaskType {
		return fmt.Errorf("当前仅支持重试自动整理任务")
	}

	metadata, _ := task["metadata"].(map[string]interface{})
	if metadata == nil {
		return fmt.Errorf("任务缺少重试元数据")
	}

	sourceID := parseRetrySourceID(metadata["source_id"])
	if sourceID <= 0 {
		return fmt.Errorf("任务缺少来源媒体源信息")
	}

	fileIDs := parseRetryFileIDs(metadata["file_ids"])
	if len(fileIDs) == 0 {
		return fmt.Errorf("任务缺少可重试的文件列表")
	}

	source, err := ws.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return fmt.Errorf("媒体源不存在")
	}
	if source.SourceType == domain.SourceTypeCloud115 {
		currentFiles, fetchErr := ws.fetchCloud115FileSet(source)
		if fetchErr != nil {
			return fmt.Errorf("恢复任务前解析 115 文件失败: %v", fetchErr)
		}
		fileIDs = resolveCloud115WatchFileIDs(fileIDs, currentFiles)
	}

	sourcePath := resolveRetrySourcePath(metadata, source, fileIDs)
	if source.OrganizeTargetPath == "" {
		return fmt.Errorf("媒体源未配置整理目标路径")
	}

	if err := ws.taskManager.Resume(taskID); err != nil {
		return fmt.Errorf("重置任务状态失败: %v", err)
	}

	ws.updateTaskMetadata(taskID, source, sourcePath, fileIDs)
	go func() {
		ws.updateTaskStatus(taskID, "running")
		ws.updateTaskProgress(taskID, len(fileIDs), 0, 0, 0)

		defer func() {
			if r := recover(); r != nil {
				reason := fmt.Sprintf("自动整理重试异常: %v", r)
				ws.updateTaskResultMetadata(taskID, source, sourcePath, fileIDs, len(fileIDs), 0, len(fileIDs), "panic", reason, buildWatchFailureItemsFromIDs(fileIDs, reason, "panic"))
				ws.setTaskError(taskID, reason)
			}
		}()

		mediaType, conflictPolicy, operationMode := resolveWatchOrganizeDefaults(source)
		results, runErr := ws.organizeService.OrganizeDirectory(
			source.ID,
			sourcePath,
			source.OrganizeTargetPath,
			mediaType,
			"",
			conflictPolicy,
			operationMode,
			fileIDs,
			true,
			nil,
			nil,
		)
		if runErr != nil {
			category, reason := summarizeAutoOrganizeFailure(runErr)
			ws.updateTaskResultMetadata(taskID, source, sourcePath, fileIDs, len(fileIDs), 0, len(fileIDs), category, reason, buildWatchFailureItemsFromIDs(fileIDs, reason, category))
			ws.setTaskError(taskID, reason)
			return
		}

		successCount := 0
		failedCount := 0
		for _, result := range results {
			if result.Success {
				successCount++
			} else {
				failedCount++
			}
		}

		category, reason := summarizeAutoOrganizeResults(results)
		ws.updateTaskResultMetadata(taskID, source, sourcePath, fileIDs, len(results), successCount, failedCount, category, reason, buildWatchFailureItems(results))
		ws.updateTaskProgress(taskID, len(results), len(results), successCount, failedCount)
		if failedCount > 0 && successCount == 0 {
			ws.setTaskError(taskID, reason)
			return
		}
		ws.updateTaskStatus(taskID, "completed")
	}()

	return nil
}

func resolveRetrySourcePath(metadata map[string]interface{}, source *domain.MediaSource, fileIDs []string) string {
	if metadata != nil {
		if raw, exists := metadata["watch_path"]; exists {
			if value, ok := raw.(string); ok {
				return value
			}
		}
		if raw, exists := metadata["source_path"]; exists {
			if value, ok := raw.(string); ok {
				return value
			}
		}
	}

	if source != nil && source.SourceType == domain.SourceTypeLocal && len(fileIDs) > 0 {
		dir := filepath.Dir(fileIDs[0])
		if dir == "." {
			return ""
		}
		return dir
	}

	return resolveWatchPath(source)
}
