package service

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type MediaLibraryPipelineService struct {
	mediaSourceDAO  *dao.MediaSourceDAO
	indexDAO        *dao.MediaSyncIndexDAO
	pendingDAO      *dao.PendingMediaDAO
	strmConfigDAO   *dao.StrmConfigDAO
	strmFileDAO     *dao.StrmFileDAO
	systemConfigDAO *dao.SystemConfigDAO
	tmdbService     *TmdbService
	taskService     *TaskService
	embyService     *EmbyService
}

type MediaLibraryPipelineResult struct {
	TaskID     string `json:"task_id"`
	SourceID   int    `json:"source_id"`
	Total      int    `json:"total"`
	Identified int    `json:"identified"`
	Pending    int    `json:"pending"`
	Strm       int    `json:"strm"`
	Skipped    int    `json:"skipped"`
	Failed     int    `json:"failed"`
}

type MediaLibraryActionResult struct {
	TaskID  string                 `json:"task_id"`
	ItemID  int                    `json:"item_id"`
	Message string                 `json:"message"`
	Item    *domain.MediaSyncIndex `json:"item,omitempty"`
}

func NewMediaLibraryPipelineService(mediaSourceDAO *dao.MediaSourceDAO, indexDAO *dao.MediaSyncIndexDAO, pendingDAO *dao.PendingMediaDAO, strmConfigDAO *dao.StrmConfigDAO, strmFileDAO *dao.StrmFileDAO, systemConfigDAO *dao.SystemConfigDAO, tmdbService *TmdbService, taskService *TaskService, embyService *EmbyService) *MediaLibraryPipelineService {
	return &MediaLibraryPipelineService{
		mediaSourceDAO:  mediaSourceDAO,
		indexDAO:        indexDAO,
		pendingDAO:      pendingDAO,
		strmConfigDAO:   strmConfigDAO,
		strmFileDAO:     strmFileDAO,
		systemConfigDAO: systemConfigDAO,
		tmdbService:     tmdbService,
		taskService:     taskService,
		embyService:     embyService,
	}
}

func (s *MediaLibraryPipelineService) ProcessSource(sourceID int) (*MediaLibraryPipelineResult, error) {
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil || source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	items, err := s.indexDAO.ListBySource(sourceID, domain.SyncStatusActive)
	if err != nil {
		return nil, err
	}
	taskID := fmt.Sprintf("library_pipeline_%d_%d", sourceID, time.Now().UnixNano())
	if s.taskService != nil {
		_ = s.taskService.Create(taskID, string(domain.TaskTypeLibraryPipeline), fmt.Sprintf("媒体入库流水线-%s", source.Name))
		_, _ = s.taskService.CreateStep(taskID, "identify_metadata", "识别媒体信息", 10, fmt.Sprintf("source_id=%d", sourceID))
		_, _ = s.taskService.CreateStep(taskID, "generate_strm", "生成 STRM", 20, "按媒体源与配置生成 STRM")
		_, _ = s.taskService.CreateStep(taskID, "refresh_server", "刷新媒体服务器", 30, "刷新已关联媒体库")
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusRunning)
	}
	result := s.ProcessItems(taskID, source, items)
	if s.taskService != nil {
		_ = s.taskService.UpdateProgress(taskID, result.Total, result.Total, result.Identified+result.Strm, result.Failed)
		_ = s.taskService.UpdateMetadata(taskID, map[string]interface{}{
			"source_id":  sourceID,
			"identified": result.Identified,
			"pending":    result.Pending,
			"strm":       result.Strm,
			"skipped":    result.Skipped,
			"failed":     result.Failed,
		})
		if result.Failed > 0 {
			_ = s.taskService.SetError(taskID, fmt.Sprintf("流水线完成但有 %d 项失败", result.Failed))
		} else {
			_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusCompleted)
		}
	}
	return result, nil
}

func (s *MediaLibraryPipelineService) ProcessItems(taskID string, source *domain.MediaSource, items []*domain.MediaSyncIndex) *MediaLibraryPipelineResult {
	result := &MediaLibraryPipelineResult{TaskID: taskID}
	if source != nil {
		result.SourceID = source.ID
	}
	if source == nil {
		return result
	}
	result.Total = len(items)
	if s.taskService != nil {
		_ = s.taskService.StartStep(taskID, "identify_metadata")
	}
	for _, item := range items {
		if item == nil || item.SyncStatus != domain.SyncStatusActive {
			result.Skipped++
			continue
		}
		if !isPipelineVideoFile(item.SourceName) {
			result.Skipped++
			continue
		}
		identified, pending, err := s.identifyItem(taskID, source, item)
		if err != nil {
			result.Failed++
			logger.Warnf("MediaLibraryPipelineService[ProcessItems] 识别失败: id=%d error=%v", item.ID, err)
			continue
		}
		if identified {
			result.Identified++
		}
		if pending {
			result.Pending++
		}
	}
	if s.taskService != nil {
		_ = s.taskService.CompleteStep(taskID, "identify_metadata", fmt.Sprintf("已识别 %d 项，待处理 %d 项", result.Identified, result.Pending))
		_ = s.taskService.StartStep(taskID, "generate_strm")
	}
	for _, item := range items {
		if item == nil || !isPipelineVideoFile(item.SourceName) || item.SyncStatus != domain.SyncStatusActive {
			continue
		}
		if err := s.GenerateStrmForItem(item.ID, taskID); err != nil {
			result.Failed++
			logger.Warnf("MediaLibraryPipelineService[ProcessItems] STRM 生成失败: id=%d error=%v", item.ID, err)
			continue
		}
		refreshed, _ := s.indexDAO.GetByID(item.ID)
		if refreshed != nil && refreshed.StrmPath != "" {
			result.Strm++
		}
	}
	if s.taskService != nil {
		_ = s.taskService.CompleteStep(taskID, "generate_strm", fmt.Sprintf("生成或确认 %d 个 STRM", result.Strm))
		_ = s.taskService.StartStep(taskID, "refresh_server")
	}
	if refreshResult, err := s.RefreshMediaServerForSource(source); err == nil && refreshResult != "" {
		if s.taskService != nil {
			_ = s.taskService.CompleteStep(taskID, "refresh_server", refreshResult)
		}
	} else if s.taskService != nil {
		_ = s.taskService.SkipStep(taskID, "refresh_server", "媒体服务器未启用或未配置媒体库")
	}
	return result
}

func (s *MediaLibraryPipelineService) ProcessItem(itemID int) (*MediaLibraryPipelineResult, error) {
	item, err := s.indexDAO.GetByID(itemID)
	if err != nil || item == nil {
		return nil, fmt.Errorf("媒体库条目不存在")
	}
	source, err := s.mediaSourceDAO.GetByID(item.SourceID)
	if err != nil || source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	taskID := fmt.Sprintf("library_pipeline_item_%d_%d", itemID, time.Now().UnixNano())
	if s.taskService != nil {
		_ = s.taskService.Create(taskID, string(domain.TaskTypeLibraryPipeline), fmt.Sprintf("单项入库-%s", item.SourceName))
		_, _ = s.taskService.CreateStep(taskID, "identify_metadata", "识别媒体信息", 10, item.SourceName)
		_, _ = s.taskService.CreateStep(taskID, "generate_strm", "生成 STRM", 20, item.SourceName)
		_, _ = s.taskService.CreateStep(taskID, "refresh_server", "刷新媒体服务器", 30, source.EmbyLibraryID)
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusRunning)
	}
	result := s.ProcessItems(taskID, source, []*domain.MediaSyncIndex{item})
	if s.taskService != nil {
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusCompleted)
	}
	return result, nil
}

func (s *MediaLibraryPipelineService) ProcessPendingItem(id int) (*MediaLibraryPipelineResult, error) {
	pending, err := s.pendingDAO.GetByID(id)
	if err != nil || pending == nil {
		return nil, fmt.Errorf("待处理项不存在")
	}
	index, err := s.indexDAO.GetBySourceFileID(pending.SourceID, pending.SourceFileID)
	if err != nil || index == nil {
		_, _ = s.pendingDAO.UpdateStatus(id, "failed", "同步索引不存在，请先重新同步媒体源", "")
		return nil, fmt.Errorf("同步索引不存在")
	}
	if pending.TmdbID > 0 {
		index.TmdbID = pending.TmdbID
		index.MediaType = normalizePipelineMediaType(pending.MediaType)
		index.IdentityStatus = domain.IdentityStatusIdentified
		index.LastChangeType = "manual_identified"
		_, _ = s.indexDAO.UpdatePipelineState(index)
	}
	result, err := s.ProcessItem(index.ID)
	if err != nil {
		_, _ = s.pendingDAO.UpdateStatus(id, "failed", err.Error(), "")
		return result, err
	}
	_, _ = s.pendingDAO.UpdateStatus(id, "completed", "已重新入库", result.TaskID)
	return result, nil
}

func (s *MediaLibraryPipelineService) GenerateStrmForItemTask(itemID int) (*MediaLibraryActionResult, error) {
	item, err := s.indexDAO.GetByID(itemID)
	if err != nil || item == nil {
		return nil, fmt.Errorf("媒体库条目不存在")
	}
	taskID := fmt.Sprintf("library_strm_item_%d_%d", itemID, time.Now().UnixNano())
	result := &MediaLibraryActionResult{
		TaskID: taskID,
		ItemID: itemID,
	}
	if s.taskService != nil {
		_ = s.taskService.Create(taskID, string(domain.TaskTypeStrmGenerate), fmt.Sprintf("生成 STRM-%s", item.SourceName))
		_, _ = s.taskService.CreateStep(taskID, "generate_strm", "生成 STRM", 10, item.SourceName)
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusRunning)
		_ = s.taskService.StartStep(taskID, "generate_strm")
	}
	if err := s.GenerateStrmForItem(itemID, taskID); err != nil {
		if s.taskService != nil {
			_ = s.taskService.FailStep(taskID, "generate_strm", err.Error())
			_ = s.taskService.UpdateProgress(taskID, 1, 1, 0, 1)
			_ = s.taskService.SetError(taskID, err.Error())
		}
		return result, err
	}
	refreshed, _ := s.indexDAO.GetByID(itemID)
	result.Item = refreshed
	if refreshed != nil && strings.TrimSpace(refreshed.StrmPath) != "" {
		result.Message = "STRM 已生成并写入资产台账"
	} else {
		result.Message = "资源无需生成 STRM 或未配置 STRM 输出目录"
	}
	if s.taskService != nil {
		_ = s.taskService.CompleteStep(taskID, "generate_strm", result.Message)
		_ = s.taskService.UpdateProgress(taskID, 1, 1, 1, 0)
		_ = s.taskService.UpdateMetadata(taskID, map[string]interface{}{
			"item_id":     itemID,
			"source_id":   item.SourceID,
			"source_name": item.SourceName,
			"strm_path":   "",
		})
		if refreshed != nil {
			_ = s.taskService.UpdateMetadata(taskID, map[string]interface{}{
				"item_id":     itemID,
				"source_id":   refreshed.SourceID,
				"source_name": refreshed.SourceName,
				"strm_path":   refreshed.StrmPath,
			})
		}
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusCompleted)
	}
	return result, nil
}

func (s *MediaLibraryPipelineService) RefreshMediaServerForItemTask(itemID int) (*MediaLibraryActionResult, error) {
	item, err := s.indexDAO.GetByID(itemID)
	if err != nil || item == nil {
		return nil, fmt.Errorf("媒体库条目不存在")
	}
	taskID := fmt.Sprintf("library_refresh_item_%d_%d", itemID, time.Now().UnixNano())
	result := &MediaLibraryActionResult{
		TaskID: taskID,
		ItemID: itemID,
	}
	if s.taskService != nil {
		_ = s.taskService.Create(taskID, string(domain.TaskTypeEmbyRefresh), fmt.Sprintf("刷新媒体库-%s", item.SourceName))
		_, _ = s.taskService.CreateStep(taskID, "refresh_server", "刷新媒体服务器", 10, item.SourceName)
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusRunning)
		_ = s.taskService.StartStep(taskID, "refresh_server")
	}
	message, err := s.RefreshMediaServerForItem(itemID)
	if err != nil {
		if s.taskService != nil {
			_ = s.taskService.FailStep(taskID, "refresh_server", err.Error())
			_ = s.taskService.UpdateProgress(taskID, 1, 1, 0, 1)
			_ = s.taskService.SetError(taskID, err.Error())
		}
		return result, err
	}
	if message == "" {
		message = "媒体服务器未启用或未配置媒体库"
		if s.taskService != nil {
			_ = s.taskService.SkipStep(taskID, "refresh_server", message)
		}
	} else if s.taskService != nil {
		_ = s.taskService.CompleteStep(taskID, "refresh_server", message)
	}
	result.Message = message
	refreshed, _ := s.indexDAO.GetByID(itemID)
	result.Item = refreshed
	if s.taskService != nil {
		_ = s.taskService.UpdateProgress(taskID, 1, 1, 1, 0)
		_ = s.taskService.UpdateMetadata(taskID, map[string]interface{}{
			"item_id":     itemID,
			"source_id":   item.SourceID,
			"source_name": item.SourceName,
			"message":     message,
		})
		_ = s.taskService.UpdateStatus(taskID, domain.TaskStatusCompleted)
	}
	return result, nil
}

func (s *MediaLibraryPipelineService) identifyItem(taskID string, source *domain.MediaSource, item *domain.MediaSyncIndex) (bool, bool, error) {
	item.LastTaskID = taskID
	if item.TmdbID > 0 {
		item.IdentityStatus = domain.IdentityStatusIdentified
		item.MediaType = normalizePipelineMediaType(item.MediaType)
		_, err := s.indexDAO.UpdatePipelineState(item)
		return true, false, err
	}
	if s.tmdbService == nil || !s.tmdbService.HasUsableAPIKey() {
		item.IdentityStatus = domain.IdentityStatusFailed
		item.LastChangeType = "identify_failed"
		if _, err := s.indexDAO.UpdatePipelineState(item); err != nil {
			return false, false, err
		}
		_, err := s.pendingDAO.CreateOrUpdateOpen(buildPendingFromIndex(item, "TMDB API Key 未配置或不可用", taskID))
		return false, true, err
	}
	identifyInput := item.SourcePath
	if identifyInput == "" {
		identifyInput = item.SourceName
	}
	identified, err := s.tmdbService.IdentifyFileWithPath(identifyInput)
	if err != nil || identified == nil || !identified.Success {
		reason := "识别失败"
		if err != nil {
			reason = err.Error()
		} else if identified != nil && identified.Message != "" {
			reason = identified.Message
		}
		item.IdentityStatus = domain.IdentityStatusFailed
		item.LastChangeType = "identify_failed"
		if _, updateErr := s.indexDAO.UpdatePipelineState(item); updateErr != nil {
			return false, false, updateErr
		}
		_, pendingErr := s.pendingDAO.CreateOrUpdateOpen(buildPendingFromIndex(item, reason, taskID))
		return false, true, pendingErr
	}
	item.TmdbID = identified.TmdbID
	item.MediaType = normalizePipelineMediaType(identified.MediaType)
	item.IdentityStatus = domain.IdentityStatusIdentified
	item.LastChangeType = "identified"
	_, err = s.indexDAO.UpdatePipelineState(item)
	return true, false, err
}

func (s *MediaLibraryPipelineService) GenerateStrmForItem(itemID int, taskID string) error {
	item, err := s.indexDAO.GetByID(itemID)
	if err != nil || item == nil {
		return fmt.Errorf("媒体库条目不存在")
	}
	if item.SourceType != domain.SourceTypeCloud115 || !isPipelineVideoFile(item.SourceName) {
		return nil
	}
	source, err := s.mediaSourceDAO.GetByID(item.SourceID)
	if err != nil || source == nil {
		return fmt.Errorf("媒体源不存在")
	}
	outputRoot, relativePath, strmConfigID := s.resolveStrmOutput(source, item)
	if strings.TrimSpace(outputRoot) == "" {
		return nil
	}
	strmPath := buildPipelineStrmPath(outputRoot, relativePath, item.SourceName)
	if err := os.MkdirAll(filepath.Dir(strmPath), 0755); err != nil {
		return fmt.Errorf("创建 STRM 目录失败: %v", err)
	}
	content := s.buildStrmContent(item)
	if err := os.WriteFile(strmPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 STRM 失败: %v", err)
	}
	item.StrmPath = strmPath
	item.LastTaskID = taskID
	item.LastChangeType = "strm_generated"
	if _, err := s.indexDAO.UpdatePipelineState(item); err != nil {
		return err
	}
	if strmConfigID > 0 && s.strmFileDAO != nil {
		_, _ = s.strmFileDAO.Upsert(strmConfigID, item.SourceName, item.SourcePath, item.SourcePickCode, item.SourceSHA1, item.SourceSize, strmPath)
	}
	return nil
}

func (s *MediaLibraryPipelineService) RefreshMediaServerForItem(itemID int) (string, error) {
	item, err := s.indexDAO.GetByID(itemID)
	if err != nil || item == nil {
		return "", fmt.Errorf("媒体库条目不存在")
	}
	source, err := s.mediaSourceDAO.GetByID(item.SourceID)
	if err != nil || source == nil {
		return "", fmt.Errorf("媒体源不存在")
	}
	return s.RefreshMediaServerForSource(source)
}

func (s *MediaLibraryPipelineService) RefreshMediaServerForSource(source *domain.MediaSource) (string, error) {
	if s.embyService == nil || source == nil || strings.TrimSpace(source.EmbyLibraryID) == "" || !s.embyService.IsEnabled() {
		return "", nil
	}
	result, err := s.embyService.RefreshLibrary(source.EmbyLibraryID)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return result.Message, nil
}

func (s *MediaLibraryPipelineService) resolveStrmOutput(source *domain.MediaSource, item *domain.MediaSyncIndex) (string, string, int) {
	relativePath := item.SourcePath
	if source != nil && strings.TrimSpace(source.Path) != "" {
		normalizedSourcePath := filepath.ToSlash(strings.TrimSpace(source.Path))
		normalizedItemPath := filepath.ToSlash(item.SourcePath)
		relativePath = strings.TrimPrefix(normalizedItemPath, strings.TrimRight(normalizedSourcePath, "/")+"/")
	}
	if source != nil && source.Cloud115ID != nil && s.strmConfigDAO != nil {
		configs, err := s.strmConfigDAO.GetAll("id", "asc")
		if err == nil {
			if cfg := findBestMatchingStrmConfig(configs, *source.Cloud115ID, item.TargetPath); cfg != nil && strings.TrimSpace(cfg.LocalPath) != "" {
				return cfg.LocalPath, relativePath, cfg.ID
			}
		}
	}
	if source != nil && strings.TrimSpace(source.OrganizeTargetPath) != "" {
		return filepath.Join(source.OrganizeTargetPath, ".strm"), relativePath, 0
	}
	return "", relativePath, 0
}

func (s *MediaLibraryPipelineService) buildStrmContent(item *domain.MediaSyncIndex) string {
	sourcePath := item.SourcePath
	if sourcePath == "" {
		sourcePath = item.SourceName
	}
	serverURL := "http://localhost:8082"
	if s.systemConfigDAO != nil {
		if cfg, err := s.systemConfigDAO.GetByKey("server_url"); err == nil && cfg != nil && strings.TrimSpace(cfg.ConfigVal) != "" {
			serverURL = strings.TrimRight(cfg.ConfigVal, "/")
		}
	}
	return fmt.Sprintf("%s/api/direct-link?path=%s", serverURL, url.QueryEscape(sourcePath))
}

func buildPendingFromIndex(item *domain.MediaSyncIndex, reason, taskID string) *domain.PendingMediaItem {
	return &domain.PendingMediaItem{
		SourceKind:    "sync",
		SourceID:      item.SourceID,
		SourceFileID:  item.SourceFileID,
		SourcePath:    item.SourcePath,
		Title:         strings.TrimSuffix(item.SourceName, filepath.Ext(item.SourceName)),
		MediaType:     normalizePipelineMediaType(item.MediaType),
		Status:        "pending",
		Reason:        reason,
		RelatedTaskID: taskID,
	}
}

func isPipelineVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".m4v", ".ts", ".m2ts", ".webm", ".rmvb":
		return true
	default:
		return false
	}
}

func normalizePipelineMediaType(mediaType string) string {
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))
	if mediaType == "anime" {
		return "tv"
	}
	if mediaType == "tv" || mediaType == "movie" {
		return mediaType
	}
	return "movie"
}

func buildPipelineStrmPath(outputRoot, relativePath, sourceName string) string {
	relativePath = filepath.ToSlash(relativePath)
	dir := filepath.Dir(relativePath)
	if dir == "." || dir == "/" {
		dir = ""
	}
	base := strings.TrimSuffix(sourceName, filepath.Ext(sourceName)) + ".strm"
	return filepath.Join(outputRoot, filepath.FromSlash(dir), base)
}
