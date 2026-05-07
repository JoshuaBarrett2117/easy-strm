package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type MediaSyncService struct {
	mediaSourceDAO  *dao.MediaSourceDAO
	cloud115DAO     *dao.Cloud115DAO
	indexDAO        *dao.MediaSyncIndexDAO
	taskService     *TaskService
	pipeline        *MediaLibraryPipelineService
	client          Cloud115Client
	systemConfigDAO *dao.SystemConfigDAO
}

type MediaSyncResult struct {
	TaskID       string `json:"task_id"`
	SourceID     int    `json:"source_id"`
	Mode         string `json:"mode"`
	TriggerMode  string `json:"trigger_mode"`
	Scanned      int    `json:"scanned"`
	Changed      int    `json:"changed"`
	Missing      int64  `json:"missing"`
	Skipped      int    `json:"skipped"`
	Failed       int    `json:"failed"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type mediaSyncScanOutcome struct {
	items    []*domain.MediaSyncIndex
	skipDiff bool
	metadata map[string]interface{}
}

type cloud115LifeCursor struct {
	LastEventID      int64 `json:"last_event_id"`
	LastUpdateTime   int64 `json:"last_update_time"`
	LastReconciledAt int64 `json:"last_reconciled_at"`
}

const cloud115LifeReconcileInterval = 24 * time.Hour

func NewMediaSyncService(mediaSourceDAO *dao.MediaSourceDAO, cloud115DAO *dao.Cloud115DAO, indexDAO *dao.MediaSyncIndexDAO, taskService *TaskService, client Cloud115Client) *MediaSyncService {
	return &MediaSyncService{
		mediaSourceDAO: mediaSourceDAO,
		cloud115DAO:    cloud115DAO,
		indexDAO:       indexDAO,
		taskService:    taskService,
		client:         client,
	}
}

func (s *MediaSyncService) SetSystemConfigDAO(systemConfigDAO *dao.SystemConfigDAO) {
	s.systemConfigDAO = systemConfigDAO
}

func (s *MediaSyncService) SetPipeline(pipeline *MediaLibraryPipelineService) {
	s.pipeline = pipeline
}

func (s *MediaSyncService) RunFullSync(sourceID int) (*MediaSyncResult, error) {
	return s.runSync(sourceID, "full", "manual")
}

func (s *MediaSyncService) RunIncrementalSync(sourceID int) (*MediaSyncResult, error) {
	return s.runSync(sourceID, "incremental", "manual")
}

func (s *MediaSyncService) ListIndex(sourceID int, status string) ([]*domain.MediaSyncIndex, error) {
	return s.indexDAO.ListBySource(sourceID, status)
}

func (s *MediaSyncService) runSync(sourceID int, mode, triggerMode string) (*MediaSyncResult, error) {
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	taskID := fmt.Sprintf("library_sync_%d_%d", source.ID, time.Now().UnixNano())
	taskName := fmt.Sprintf("媒体库%s同步-%s", syncModeName(mode), source.Name)
	result := &MediaSyncResult{
		TaskID:      taskID,
		SourceID:    source.ID,
		Mode:        mode,
		TriggerMode: triggerMode,
	}

	if s.taskService != nil {
		if err := s.taskService.Create(taskID, string(domain.TaskTypeLibrarySync), taskName); err != nil {
			return nil, err
		}
		_, _ = s.taskService.CreateStep(taskID, "scan_source", "扫描源目录", 10, fmt.Sprintf("source_id=%d mode=%s", source.ID, mode))
		_, _ = s.taskService.CreateStep(taskID, "diff_index", "差异对账", 20, "比较源端与同步索引")
		_, _ = s.taskService.CreateStep(taskID, "write_index", "写入同步索引", 30, "保存媒体库资产映射")
		_, _ = s.taskService.CreateStep(taskID, "identify_metadata", "识别媒体信息", 40, "自动识别并生成待处理项")
		_, _ = s.taskService.CreateStep(taskID, "generate_strm", "生成 STRM", 50, "按媒体源和 STRM 配置生成")
		_, _ = s.taskService.CreateStep(taskID, "refresh_server", "刷新媒体服务器", 60, "刷新已关联媒体服务器")
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusRunning)
	}

	outcome := &mediaSyncScanOutcome{}
	switch source.SourceType {
	case domain.SourceTypeLocal:
		outcome.items, err = s.scanLocalSource(source, taskID)
	case domain.SourceTypeCloud115:
		outcome, err = s.scanCloud115SourceForMode(source, taskID, mode)
	default:
		err = fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
	}
	if err != nil {
		result.ErrorMessage = err.Error()
		if s.taskService != nil {
			_ = s.taskService.FailStep(taskID, "scan_source", err.Error())
			_ = s.taskService.SetError(taskID, err.Error())
		}
		return result, err
	}
	items := outcome.items
	result.Scanned = len(items)
	if s.taskService != nil {
		scanSummary := fmt.Sprintf("扫描到 %d 个文件", result.Scanned)
		if outcome.skipDiff {
			scanSummary = "115 生活事件流无新变化，跳过目录扫描"
		}
		_ = s.taskService.CompleteStep(taskID, "scan_source", scanSummary)
		_ = s.taskService.StartStep(taskID, "diff_index")
	}

	if outcome.skipDiff {
		if s.taskService != nil {
			_ = s.taskService.CompleteStep(taskID, "diff_index", "无变化，跳过差异对账")
			_ = s.taskService.StartStep(taskID, "write_index")
			_ = s.taskService.CompleteStep(taskID, "write_index", "无变化，跳过索引写入")
			_ = s.taskService.SkipStep(taskID, "identify_metadata", "无新增或变更文件，跳过识别")
			_ = s.taskService.SkipStep(taskID, "generate_strm", "无新增或变更文件，跳过 STRM 生成")
			_ = s.taskService.SkipStep(taskID, "refresh_server", "无新增或变更文件，跳过媒体服务器刷新")
		}
		s.finishSyncTask(taskID, source, mode, triggerMode, result, outcome.metadata)
		return result, nil
	}

	activeIDs := make([]string, 0, len(items))
	for _, item := range items {
		activeIDs = append(activeIDs, item.SourceFileID)
	}
	missing, err := s.indexDAO.MarkMissingExcept(source.ID, taskID, activeIDs)
	if err != nil {
		result.ErrorMessage = err.Error()
		if s.taskService != nil {
			_ = s.taskService.FailStep(taskID, "diff_index", err.Error())
			_ = s.taskService.SetError(taskID, err.Error())
		}
		return result, err
	}
	result.Missing = missing
	if s.taskService != nil {
		_ = s.taskService.CompleteStep(taskID, "diff_index", fmt.Sprintf("失效 %d 项", missing))
		_ = s.taskService.StartStep(taskID, "write_index")
	}

	persistedItems := make([]*domain.MediaSyncIndex, 0, len(items))
	for _, item := range items {
		persisted, err := s.indexDAO.Upsert(item)
		if err != nil {
			result.Failed++
			logger.Warnf("MediaSyncService[runSync] 写入索引失败: source_id=%d file=%s error=%v", source.ID, item.SourceFileID, err)
			continue
		}
		persistedItems = append(persistedItems, persisted)
		result.Changed++
	}
	if s.taskService != nil {
		_ = s.taskService.CompleteStep(taskID, "write_index", fmt.Sprintf("写入 %d 项，失败 %d 项", result.Changed, result.Failed))
	}
	if s.pipeline != nil {
		pipelineResult := s.pipeline.ProcessItems(taskID, source, persistedItems)
		result.Skipped += pipelineResult.Skipped
		result.Failed += pipelineResult.Failed
	}
	s.finishSyncTask(taskID, source, mode, triggerMode, result, outcome.metadata)
	return result, nil
}

func (s *MediaSyncService) finishSyncTask(taskID string, source *domain.MediaSource, mode, triggerMode string, result *MediaSyncResult, extraMetadata map[string]interface{}) {
	if s.taskService != nil {
		_ = s.taskService.UpdateProgress(taskID, result.Scanned, result.Scanned, result.Changed, result.Failed)
		metadata := map[string]interface{}{
			"source_id":    source.ID,
			"source_name":  source.Name,
			"source_type":  source.SourceType,
			"sync_mode":    mode,
			"trigger_mode": triggerMode,
			"scanned":      result.Scanned,
			"changed":      result.Changed,
			"missing":      result.Missing,
			"failed":       result.Failed,
		}
		for key, value := range extraMetadata {
			metadata[key] = value
		}
		_ = s.taskService.UpdateMetadata(taskID, metadata)
		if result.Failed > 0 {
			_ = s.taskService.SetError(taskID, fmt.Sprintf("同步完成但有 %d 项失败", result.Failed))
		} else {
			_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusCompleted)
		}
	}
}

func (s *MediaSyncService) scanLocalSource(source *domain.MediaSource, taskID string) ([]*domain.MediaSyncIndex, error) {
	root := strings.TrimSpace(source.Path)
	if root == "" {
		return nil, fmt.Errorf("本地媒体源路径为空")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("本地媒体源目录不可访问: %s", root)
	}

	var items []*domain.MediaSyncIndex
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !isMediaLibraryTrackedExt(ext) {
			return nil
		}
		stat, err := entry.Info()
		if err != nil {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			relativePath = path
		}
		modified := stat.ModTime()
		item := &domain.MediaSyncIndex{
			SourceID:             source.ID,
			SourceType:           source.SourceType,
			SourceFileID:         filepath.ToSlash(relativePath),
			SourcePath:           path,
			SourceName:           entry.Name(),
			SourceSize:           stat.Size(),
			SourceModifiedTime:   &modified,
			TargetPath:           resolveDefaultTargetPath(source, relativePath),
			MediaServerType:      "emby",
			MediaServerLibraryID: source.EmbyLibraryID,
			IdentityStatus:       domain.IdentityStatusUnknown,
			SyncStatus:           domain.SyncStatusActive,
			LastChangeType:       "scanned",
			LastTaskID:           taskID,
		}
		if strings.EqualFold(ext, ".strm") {
			item.StrmPath = path
		}
		if isMetadataExt(ext) {
			item.MetadataPath = path
		}
		items = append(items, item)
		return nil
	})
	return items, err
}

func (s *MediaSyncService) scanCloud115Source(source *domain.MediaSource, taskID string) ([]*domain.MediaSyncIndex, error) {
	if s.client == nil {
		return nil, fmt.Errorf("115 客户端未初始化")
	}
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("115 媒体源未关联账号")
	}
	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("115 账号不存在")
	}
	cidText := strings.TrimSpace(source.Path)
	if cidText == "" || cidText == "/" {
		cidText = "0"
	}
	cid, err := strconv.Atoi(cidText)
	if err != nil {
		return nil, fmt.Errorf("115 目录 ID 无效: %s", cidText)
	}
	resp, err := s.client.GetFileList(cid, 1, 0, 1000, cloud115.ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("获取 115 文件列表失败: %v", err)
	}
	items := make([]*domain.MediaSyncIndex, 0, len(resp.Files))
	now := time.Now()
	for _, file := range resp.Files {
		isDir := file.FileID == "" || file.Type == "folder"
		if isDir {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name))
		if !isMediaLibraryTrackedExt(ext) {
			continue
		}
		sourceFileID := file.PickCode
		if sourceFileID == "" {
			sourceFileID = file.FileID
		}
		if sourceFileID == "" {
			continue
		}
		items = append(items, &domain.MediaSyncIndex{
			SourceID:             source.ID,
			SourceType:           source.SourceType,
			SourceFileID:         sourceFileID,
			SourcePath:           file.Name,
			SourceName:           file.Name,
			SourcePickCode:       file.PickCode,
			SourceSHA1:           file.Sha1,
			SourceSize:           int64(file.Size),
			SourceModifiedTime:   &now,
			TargetPath:           resolveDefaultTargetPath(source, file.Name),
			MediaServerType:      "emby",
			MediaServerLibraryID: source.EmbyLibraryID,
			IdentityStatus:       domain.IdentityStatusUnknown,
			SyncStatus:           domain.SyncStatusActive,
			LastChangeType:       "scanned",
			LastTaskID:           taskID,
		})
	}
	return items, nil
}

func (s *MediaSyncService) scanCloud115SourceForMode(source *domain.MediaSource, taskID string, mode string) (*mediaSyncScanOutcome, error) {
	if mode != "incremental" {
		items, err := s.scanCloud115Source(source, taskID)
		return &mediaSyncScanOutcome{items: items, metadata: map[string]interface{}{"cloud115_incremental_strategy": "directory_full"}}, err
	}

	metadata := map[string]interface{}{
		"cloud115_incremental_strategy": "life_event_first",
		"cloud115_event_endpoint":       "proapi.115.com/android/behavior/detail",
	}
	lifeClient, ok := s.client.(Cloud115LifeEventClient)
	if !ok {
		metadata["cloud115_event_fallback_reason"] = "client_not_supported"
		items, err := s.scanCloud115Source(source, taskID)
		return &mediaSyncScanOutcome{items: items, metadata: metadata}, err
	}
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("115 媒体源未关联账号")
	}
	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("115 账号不存在")
	}

	cursor := s.loadCloud115LifeCursor(source.ID)
	needsBaseline := cursor.LastEventID <= 0 && cursor.LastUpdateTime <= 0
	needsReconcile := cursor.LastReconciledAt <= 0 || time.Since(time.Unix(cursor.LastReconciledAt, 0)) >= cloud115LifeReconcileInterval

	resp, err := lifeClient.GetLifeEvents(0, 1000, "", "", cloud115.ID, cloud115.Cookie)
	if err != nil {
		logger.Warnf("MediaSyncService[scanCloud115SourceForMode] 115 生活事件流拉取失败，回退目录扫描: source_id=%d error=%v", source.ID, err)
		metadata["cloud115_event_fallback_reason"] = "event_fetch_failed"
		metadata["cloud115_event_error"] = err.Error()
		items, scanErr := s.scanCloud115Source(source, taskID)
		if scanErr == nil {
			s.saveCloud115LifeCursor(source.ID, cursorFromLifeEvents(resp, time.Now()))
		}
		return &mediaSyncScanOutcome{items: items, metadata: metadata}, scanErr
	}

	events := filterCloud115LifeEvents(resp.Events, cursor)
	metadata["cloud115_event_count"] = len(resp.Events)
	metadata["cloud115_event_new_count"] = len(events)
	if latestCursor := cursorFromLifeEvents(resp, time.Unix(cursor.LastReconciledAt, 0)); latestCursor.LastEventID > cursor.LastEventID || latestCursor.LastUpdateTime > cursor.LastUpdateTime {
		cursor.LastEventID = latestCursor.LastEventID
		cursor.LastUpdateTime = latestCursor.LastUpdateTime
	}

	if len(events) == 0 && !needsBaseline && !needsReconcile {
		s.saveCloud115LifeCursor(source.ID, cursor)
		metadata["cloud115_event_action"] = "skip_scan_no_event"
		return &mediaSyncScanOutcome{skipDiff: true, metadata: metadata}, nil
	}

	if needsBaseline {
		metadata["cloud115_event_fallback_reason"] = "cursor_baseline"
	} else if needsReconcile {
		metadata["cloud115_event_fallback_reason"] = "periodic_reconcile"
	} else {
		metadata["cloud115_event_action"] = "scan_after_event"
		metadata["cloud115_event_changed_types"] = summarizeCloud115LifeEventTypes(events)
	}

	items, err := s.scanCloud115Source(source, taskID)
	if err != nil {
		return &mediaSyncScanOutcome{metadata: metadata}, err
	}
	cursor.LastReconciledAt = time.Now().Unix()
	s.saveCloud115LifeCursor(source.ID, cursor)
	return &mediaSyncScanOutcome{items: items, metadata: metadata}, nil
}

func filterCloud115LifeEvents(events []Cloud115LifeEvent, cursor cloud115LifeCursor) []Cloud115LifeEvent {
	filtered := make([]Cloud115LifeEvent, 0, len(events))
	for _, event := range events {
		if cursor.LastEventID > 0 && event.ID <= cursor.LastEventID {
			continue
		}
		if cursor.LastEventID == 0 && cursor.LastUpdateTime > 0 && event.UpdateTime <= cursor.LastUpdateTime {
			continue
		}
		if isIgnoredCloud115LifeEvent(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func isIgnoredCloud115LifeEvent(event Cloud115LifeEvent) bool {
	switch event.Type {
	case 3, 4, 7, 8, 9, 10, 19:
		return true
	default:
		return false
	}
}

func summarizeCloud115LifeEventTypes(events []Cloud115LifeEvent) map[string]int {
	summary := make(map[string]int)
	for _, event := range events {
		name := strings.TrimSpace(event.EventName)
		if name == "" {
			name = strconv.Itoa(event.Type)
		}
		summary[name]++
	}
	return summary
}

func cursorFromLifeEvents(resp *Cloud115LifeEventResp, fallback time.Time) cloud115LifeCursor {
	cursor := cloud115LifeCursor{LastReconciledAt: fallback.Unix()}
	if resp == nil {
		return cursor
	}
	for _, event := range resp.Events {
		if event.ID > cursor.LastEventID {
			cursor.LastEventID = event.ID
		}
		if event.UpdateTime > cursor.LastUpdateTime {
			cursor.LastUpdateTime = event.UpdateTime
		}
	}
	return cursor
}

func (s *MediaSyncService) cloud115LifeCursorKey(sourceID int) string {
	return fmt.Sprintf("media_source:%d:cloud115_life_cursor", sourceID)
}

func (s *MediaSyncService) loadCloud115LifeCursor(sourceID int) cloud115LifeCursor {
	if s.systemConfigDAO == nil {
		return cloud115LifeCursor{}
	}
	config, err := s.systemConfigDAO.GetByKey(s.cloud115LifeCursorKey(sourceID))
	if err != nil || config == nil || strings.TrimSpace(config.ConfigVal) == "" {
		return cloud115LifeCursor{}
	}
	var cursor cloud115LifeCursor
	if err := json.Unmarshal([]byte(config.ConfigVal), &cursor); err != nil {
		logger.Warnf("MediaSyncService[loadCloud115LifeCursor] 游标解析失败: source_id=%d error=%v", sourceID, err)
		return cloud115LifeCursor{}
	}
	return cursor
}

func (s *MediaSyncService) saveCloud115LifeCursor(sourceID int, cursor cloud115LifeCursor) {
	if s.systemConfigDAO == nil {
		return
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		logger.Warnf("MediaSyncService[saveCloud115LifeCursor] 游标序列化失败: source_id=%d error=%v", sourceID, err)
		return
	}
	if err := s.systemConfigDAO.Upsert(s.cloud115LifeCursorKey(sourceID), string(data)); err != nil {
		logger.Warnf("MediaSyncService[saveCloud115LifeCursor] 游标保存失败: source_id=%d error=%v", sourceID, err)
	}
}

func syncModeName(mode string) string {
	if mode == "incremental" {
		return "增量"
	}
	return "全量"
}

func isMediaLibraryTrackedExt(ext string) bool {
	if videoExtensions[ext] {
		return true
	}
	switch ext {
	case ".srt", ".ass", ".ssa", ".vtt", ".nfo", ".jpg", ".jpeg", ".png", ".webp", ".strm":
		return true
	default:
		return false
	}
}

func isMetadataExt(ext string) bool {
	switch ext {
	case ".nfo", ".jpg", ".jpeg", ".png", ".webp", ".srt", ".ass", ".ssa", ".vtt":
		return true
	default:
		return false
	}
}

func resolveDefaultTargetPath(source *domain.MediaSource, relativePath string) string {
	base := strings.TrimSpace(source.OrganizeTargetPath)
	if base == "" {
		return filepath.ToSlash(relativePath)
	}
	return filepath.ToSlash(filepath.Join(base, relativePath))
}
