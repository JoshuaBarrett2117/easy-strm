package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	source     *domain.MediaSource
	debounce   map[string]*time.Timer
	debounceMu sync.Mutex
}

type cloud115WatchState struct {
	source     *domain.MediaSource
	ticker     *time.Ticker
	stopCh     chan struct{}
	knownFiles map[string]bool
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
	taskName := fmt.Sprintf("115鑷姩鏁寸悊-%s", source.Name)
	if source.SourceType == domain.SourceTypeLocal {
		taskName = fmt.Sprintf("鏈湴鑷姩鏁寸悊-%s", source.Name)
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
		"result_summary":       fmt.Sprintf("鎴愬姛 %d锛屽け璐?%d", success, failed),
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

func resolveWatchOrganizeDefaults(source *domain.MediaSource) (string, string, string) {
	mediaType := normalizeMediaType(source.MediaType)
	conflictPolicy := normalizeConflictPolicy(source.ConflictPolicy)
	operationMode := normalizeOperationMode(source.OperationMode)

	if source != nil && source.SourceType == domain.SourceTypeCloud115 {
		switch operationMode {
		case "hardlink", "symlink":
			operationMode = "move"
		}
	}

	return mediaType, conflictPolicy, operationMode
}

func summarizeAutoOrganizeFailure(err error) (string, string) {
	if err == nil {
		return "", ""
	}

	message := err.Error()
	lowerMessage := strings.ToLower(message)

	switch {
	case strings.Contains(lowerMessage, "target") && strings.Contains(lowerMessage, "path"):
		return "target_path", message
	case strings.Contains(lowerMessage, "scan") || strings.Contains(lowerMessage, "list"):
		return "scan_failed", message
	case strings.Contains(lowerMessage, "identify") || strings.Contains(lowerMessage, "tmdb") || strings.Contains(lowerMessage, "recogniz"):
		return "identify_failed", message
	case isCloud115AuthFailureMessage(message):
        return "cloud115_auth_failed", "115 账号 Cookie 已失效，请重新登录"
	case strings.Contains(lowerMessage, "115") || strings.Contains(lowerMessage, "cloud"):
		return "cloud115_failed", message
	default:
		return "organize_failed", message
	}
}

func classifyWatchFailureCategory(reason string) string {
	lowerReason := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case lowerReason == "":
		return "other"
	case strings.Contains(lowerReason, "identify") || strings.Contains(lowerReason, "tmdb") || strings.Contains(lowerReason, "recogniz"):
		return "identify_failed"
	case strings.Contains(lowerReason, "cookie") || strings.Contains(lowerReason, "auth") || strings.Contains(lowerReason, "unauthorized") || strings.Contains(lowerReason, "token"):
		return "cloud115_auth_failed"
	case strings.Contains(lowerReason, "cloud115") || strings.Contains(lowerReason, "115"):
		return "cloud115_failed"
	case strings.Contains(lowerReason, "scan") || strings.Contains(lowerReason, "list"):
		return "scan_failed"
	case strings.Contains(lowerReason, "target") && strings.Contains(lowerReason, "path"):
		return "target_path"
	case strings.Contains(lowerReason, "panic") || strings.Contains(lowerReason, "exception"):
		return "panic"
	case strings.Contains(lowerReason, "organize") || strings.Contains(lowerReason, "move") || strings.Contains(lowerReason, "copy"):
		return "organize_failed"
	default:
		return "other"
	}
}

func summarizeAutoOrganizeResults(results []OrganizeResult) (string, string) {
	if len(results) == 0 {
		return "", ""
	}

	identifyFailures := 0
	conflictFailures := 0
	moveFailures := 0
	var firstMessage string

	for _, result := range results {
		if result.Success {
			continue
		}
		if firstMessage == "" {
			firstMessage = result.Message
		}

		lowerMessage := strings.ToLower(result.Message)
		switch {
		case strings.Contains(lowerMessage, "identify") || strings.Contains(lowerMessage, "tmdb") || strings.Contains(lowerMessage, "recogniz"):
			identifyFailures++
		case isCloud115AuthFailureMessage(result.Message):
            return "cloud115_auth_failed", "115 账号 Cookie 已失效，请重新登录"
		case strings.Contains(lowerMessage, "conflict") || strings.Contains(lowerMessage, "exists"):
			conflictFailures++
		default:
			moveFailures++
		}
	}

	switch {
	case identifyFailures > 0 && conflictFailures == 0 && moveFailures == 0:
		return "identify_failed", fmt.Sprintf("璇嗗埆澶辫触 %d 椤癸細%s", identifyFailures, firstMessage)
	case conflictFailures > 0 && identifyFailures == 0 && moveFailures == 0:
		return "conflict_skipped", fmt.Sprintf("鍐茬獊璺宠繃 %d 椤癸細%s", conflictFailures, firstMessage)
	case isCloud115AuthFailureMessage(firstMessage):
        return "cloud115_auth_failed", "115 账号 Cookie 已失效，请重新登录"
	case moveFailures > 0 && identifyFailures == 0 && conflictFailures == 0:
		return "organize_failed", fmt.Sprintf("鏁寸悊澶辫触 %d 椤癸細%s", moveFailures, firstMessage)
	case identifyFailures > 0 || conflictFailures > 0 || moveFailures > 0:
        return "partial_failed", fmt.Sprintf("识别失败 %d 项，冲突跳过 %d 项，整理失败 %d 项", identifyFailures, conflictFailures, moveFailures)
	default:
		return "", ""
	}
}

func buildWatchFailureItems(results []OrganizeResult) []watchFailureItem {
	items := make([]watchFailureItem, 0)
	for _, result := range results {
		if result.Success {
			continue
		}
		reason := result.Message
		if reason == "" {
			reason = "鏁寸悊澶辫触"
		}
		items = append(items, watchFailureItem{
			FileID:   result.FileID,
			FileName: result.FileName,
			Category: classifyWatchFailureCategory(reason),
			Reason:   reason,
		})
	}
	return items
}

func buildWatchFailureItemsFromIDs(fileIDs []string, reason, category string) []watchFailureItem {
	if category == "" {
		category = classifyWatchFailureCategory(reason)
	}
	items := make([]watchFailureItem, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if fileID == "" {
			continue
		}
		items = append(items, watchFailureItem{
			FileID:   fileID,
			FileName: fileID,
			Category: category,
			Reason:   reason,
		})
	}
	return items
}

func parseRetryFileIDs(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		fileIDs := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				fileIDs = append(fileIDs, text)
			}
		}
		return fileIDs
	default:
		return nil
	}
}

func parseRetrySourceID(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func isCloud115AuthFailureMessage(message string) bool {
	lowerMessage := strings.ToLower(strings.TrimSpace(message))
	if lowerMessage == "" {
		return false
	}

	switch {
	case strings.Contains(lowerMessage, "parse cookie failed"),
		strings.Contains(lowerMessage, "cookie"),
		strings.Contains(lowerMessage, "unauthorized"),
		strings.Contains(lowerMessage, "authentication"),
		strings.Contains(lowerMessage, "token expired"),
		strings.Contains(lowerMessage, "session expired"),
		strings.Contains(message, "鐧诲綍澶辨晥"),
		strings.Contains(message, "cookie澶辨晥"),
		strings.Contains(message, "cookie鏃犳晥"),
		strings.Contains(message, "115璐﹀彿澶辨晥"),
		strings.Contains(message, "璐﹀彿澶辨晥"),
        strings.Contains(message, "请重新登录"),
        strings.Contains(message, "账号不存在"):
		return true
	default:
		return false
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
		return fmt.Errorf("婵帊缍嬪┃鎰瑝鐎涙ê婀? id=%d", sourceID)
	}
	if !source.Enabled {
		return fmt.Errorf("婵帊缍嬪┃鎰弓閸氼垳鏁? id=%d", sourceID)
	}
	if !source.WatchEnabled {
		return fmt.Errorf("濯掍綋婧愭湭寮€鍚洃鎺? id=%d", sourceID)
	}

	switch source.SourceType {
	case domain.SourceTypeLocal:
		return ws.startLocalWatching(source)
	case domain.SourceTypeCloud115:
		return ws.startCloud115Watching(source)
	default:
		return fmt.Errorf("涓嶆敮鎸佺殑濯掍綋婧愮被鍨? %s", source.SourceType)
	}
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
			_ = ws.watcher.Remove(resolveWatchPath(state.source))
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
			return fmt.Errorf("鍒涘缓 fsnotify watcher 澶辫触: %v", err)
		}
		ws.watcher = watcher
		go ws.handleLocalEvents()
	}

	watchPath := resolveWatchPath(source)
	info, err := os.Stat(watchPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("鐩綍涓嶅瓨鍦ㄦ垨涓嶅彲璁块棶: %s", watchPath)
	}

	if err := ws.watcher.Add(watchPath); err != nil {
		return fmt.Errorf("娣诲姞鐩戞帶鐩綍澶辫触: %s, error: %v", watchPath, err)
	}

	ws.localWatches[source.ID] = &localWatchState{
		source:   source,
		debounce: make(map[string]*time.Timer),
	}
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
	ext := strings.ToLower(filepath.Ext(filePath))
	if !videoExtensions[ext] {
		return
	}

	ws.mu.RLock()
	var matchedState *localWatchState
	for _, state := range ws.localWatches {
		if strings.HasPrefix(filePath, resolveWatchPath(state.source)) {
			matchedState = state
			break
		}
	}
	ws.mu.RUnlock()

	if matchedState == nil {
		return
	}

	source := matchedState.source
	matchedState.debounceMu.Lock()
	if timer, exists := matchedState.debounce[filePath]; exists {
		timer.Stop()
	}
	matchedState.debounce[filePath] = time.AfterFunc(defaultDebounceInterval, func() {
		matchedState.debounceMu.Lock()
		delete(matchedState.debounce, filePath)
		matchedState.debounceMu.Unlock()
		ws.processNewLocalFile(source, filePath)
	})
	matchedState.debounceMu.Unlock()
}

func (ws *WatchService) processNewLocalFile(source *domain.MediaSource, filePath string) {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return
	}
	if !source.AutoOrganize {
		return
	}
	ws.triggerAutoOrganize(source, resolveWatchPath(source), []string{filePath})
}

func (ws *WatchService) startCloud115Watching(source *domain.MediaSource) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if _, ok := ws.cloud115Watches[source.ID]; ok {
		return nil
	}
	if source.Cloud115ID == nil {
		return fmt.Errorf("115 濯掍綋婧愭湭鍏宠仈璐﹀彿: source_id=%d", source.ID)
	}

	interval := source.WatchInterval
	if interval < defaultCloud115MinInterval {
		interval = defaultCloud115MinInterval
	}

	knownFiles, err := ws.fetchCloud115FileSet(source)
	if err != nil {
		logger.Warnf("[WatchService] init cloud115 file list failed: source_id=%d, error=%v", source.ID, err)
		knownFiles = make(map[string]bool)
	}

	state := &cloud115WatchState{
		source:     source,
		ticker:     time.NewTicker(time.Duration(interval) * time.Second),
		stopCh:     make(chan struct{}),
		knownFiles: knownFiles,
	}
	ws.cloud115Watches[source.ID] = state
	go ws.cloud115PollLoop(state)
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

	var newFiles []string
	for pickcode := range currentFiles {
		if !state.knownFiles[pickcode] {
			newFiles = append(newFiles, pickcode)
		}
	}
	state.knownFiles = currentFiles

	if len(newFiles) == 0 || !latestSource.AutoOrganize {
		return
	}
	ws.triggerAutoOrganize(latestSource, resolveWatchPath(latestSource), newFiles)
}

func (ws *WatchService) fetchCloud115FileSet(source *domain.MediaSource) (map[string]bool, error) {
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("鏈叧鑱?115 璐﹀彿")
	}

	cloud115, err := ws.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("115 璐﹀彿涓嶅瓨鍦? cloud115_id=%d", *source.Cloud115ID)
	}

	cidStr := resolveWatchPath(source)
	if cidStr == "" || cidStr == "/" {
		cidStr = "0"
	}
	cid, err := strconv.Atoi(cidStr)
	if err != nil {
		return nil, fmt.Errorf("CID 鏍煎紡閿欒: %s", cidStr)
	}

	fileList, err := ws.client.GetFileList(cid, 1, 0, 1000, cloud115.ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇 115 鏂囦欢鍒楄〃澶辫触: %v", err)
	}

	fileSet := make(map[string]bool, len(fileList.Files))
	for _, f := range fileList.Files {
		isDir := f.FileID == "" || f.Type == "folder"
		if isDir {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if !videoExtensions[ext] {
			continue
		}

		pickcode := f.PickCode
		if pickcode == "" {
			pickcode = f.FileID
		}
		if pickcode != "" {
			fileSet[pickcode] = true
		}
	}

	return fileSet, nil
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
				reason := fmt.Sprintf("鑷姩鏁寸悊寮傚父: %v", r)
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
		return fmt.Errorf("当前仅支持重试 115 自动整理任务")
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

	sourcePath, _ := metadata["watch_path"].(string)
	if sourcePath == "" {
		sourcePath, _ = metadata["source_path"].(string)
	}
	if sourcePath == "" {
		sourcePath = resolveWatchPath(source)
	}
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
