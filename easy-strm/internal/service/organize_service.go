package service

import (
	"context"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/go-redis/redis/v8"
)

const (
	defaultPreviewConcurrency = 10
	identifyCacheRedisTTL     = 7 * 24 * time.Hour
	cloud115ScanInterval      = 200 * time.Millisecond
	cloud115ListCacheTTL      = 10 * time.Second
)

// OrganizeService 自动整理服务
// 负责媒体文件的扫描、识别、更名和移动
type OrganizeService struct {
	mediaSourceService   *MediaSourceService
	tmdbService          *TmdbService
	tmdbCacheDAO         *dao.TmdbCacheDAO
	identifyCacheDAO     *dao.IdentifyCacheDAO
	renameService        *RenameService
	fileOperationService *FileOperationService
	categoryDAO          *dao.MediaCategoryDAO
	cloud115DAO          *dao.Cloud115DAO
	systemConfigDAO      SystemConfigReader
	scrapeService        *ScrapeService
	client               Cloud115Client
	redisClient          *redis.Client
	cloud115ListCacheMu  sync.RWMutex
	cloud115ListCache    map[string]cloud115ListCacheEntry
}

type cloud115ListCacheEntry struct {
	files     []domain.MediaFile
	expiresAt time.Time
}

// NewOrganizeService 创建自动整理服务实例
// 参数:
//   - mediaSourceService: 媒体源服务
//   - tmdbService: TMDB 服务
//   - renameService: 更名服务
//   - fileOperationService: 文件操作服务
//
// 返回:
//   - *OrganizeService: 自动整理服务实例
func NewOrganizeService(
	mediaSourceService *MediaSourceService,
	tmdbService *TmdbService,
	renameService *RenameService,
	fileOperationService *FileOperationService,
	categoryDAO *dao.MediaCategoryDAO,
	cloud115DAO *dao.Cloud115DAO,
	systemConfigDAO SystemConfigReader,
	scrapeService *ScrapeService,
	client Cloud115Client,
	redisClient *redis.Client,
) *OrganizeService {
	return &OrganizeService{
		mediaSourceService:   mediaSourceService,
		tmdbService:          tmdbService,
		tmdbCacheDAO:         dao.NewTmdbCacheDAO(),
		identifyCacheDAO:     dao.NewIdentifyCacheDAO(),
		renameService:        renameService,
		fileOperationService: fileOperationService,
		categoryDAO:          categoryDAO,
		cloud115DAO:          cloud115DAO,
		systemConfigDAO:      systemConfigDAO,
		scrapeService:        scrapeService,
		client:               client,
		redisClient:          redisClient,
		cloud115ListCache:    make(map[string]cloud115ListCacheEntry),
	}
}

// OrganizePreview 整理预览结果
type OrganizePreview struct {
	FileID        string `json:"file_id"`
	CloudID       string `json:"cloud_id"` // 115 内部 CID / PickCode
	FileName      string `json:"file_name"`
	FilePath      string `json:"file_path"`
	MediaType     string `json:"media_type"`
	TmdbID        int    `json:"tmdb_id"`
	Title         string `json:"title"`
	Year          int    `json:"year"`
	Season        int    `json:"season"`
	Episode       int    `json:"episode"`
	NewName       string `json:"new_name"`
	NewPath       string `json:"new_path"`
	TargetPath    string `json:"target_path"`
	SourcePath    string `json:"source_path"`
	Conflict      bool   `json:"conflict"`
	ConflictPath  string `json:"conflict_path,omitempty"`
	IdentifyError string `json:"identify_error,omitempty"`
}

// OrganizeResult 整理执行结果
type OrganizeResult struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	Success  bool   `json:"success"`
	Skipped  bool   `json:"skipped"`
	Message  string `json:"message"`
	OldPath  string `json:"old_path"`
	NewPath  string `json:"new_path"`
}

// OrganizeCandidate 整理候选文件
type OrganizeCandidate struct {
	FileID     string `json:"file_id"`
	CloudID    string `json:"cloud_id,omitempty"`
	FileName   string `json:"file_name"`
	FilePath   string `json:"file_path"`
	SourcePath string `json:"source_path"`
}

func (s *OrganizeService) ListOrganizeCandidates(sourceID int, sourcePath, mediaType string, fileIDs []string) ([]OrganizeCandidate, error) {
	startedAt := time.Now()

	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	files, err := s.scanFiles(source, sourcePath, mediaType, fileIDs, false)
	if err != nil {
		return nil, fmt.Errorf("扫描文件失败: %v", err)
	}
	files = s.filterScannedFiles(source, files, fileIDs)

	candidates := make([]OrganizeCandidate, 0, len(files))
	for _, file := range files {
		candidates = append(candidates, OrganizeCandidate{
			FileID:     file.ID,
			CloudID:    file.CID,
			FileName:   file.Name,
			FilePath:   file.Path,
			SourcePath: sourcePath,
		})
	}
	logger.Infof("OrganizeService[ListOrganizeCandidates] 候选扫描完成: source_id=%d, source_type=%s, source_path=%s, selected=%d, candidates=%d, elapsed=%s",
		sourceID, source.SourceType, sourcePath, len(fileIDs), len(candidates), time.Since(startedAt))
	return candidates, nil
}

func (s *OrganizeService) PreviewOrganize(sourceID int, sourcePath, targetPath, mediaType, template string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) ([]OrganizePreview, error) {
	logger.Infof("OrganizeService[PreviewOrganize] 开始预览: source_id=%d, source_path=%s, target_path=%s", sourceID, sourcePath, targetPath)

	// 获取媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	return s.previewOrganizeForSource(source, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)
}

// previewOrganizeForSource 预览整理（直接消费已构造的 *MediaSource，不调用 GetByID）。
// 供 PreviewOrganize 与 OrganizeDirectoryForSource 复用，避免无 DB 媒体源时重复查询。
func (s *OrganizeService) previewOrganizeForSource(source *domain.MediaSource, sourcePath, targetPath, mediaType, template string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) ([]OrganizePreview, error) {
	logger.Infof("OrganizeService[PreviewOrganize] 开始预览: source_type=%s, source_path=%s, target_path=%s", source.SourceType, sourcePath, targetPath)

	// 获取文件列表 (传入想要整理的 ID 以进行扫描剪枝)
	files, err := s.scanFiles(source, sourcePath, mediaType, fileIDs, true)
	if err != nil {
		return nil, fmt.Errorf("扫描文件失败: %v", err)
	}

	logger.Infof("OrganizeService[PreviewOrganize] 扫描到 %d 个文件, 起始路径=%s", len(files), sourcePath)
	if len(files) > 0 {
		logger.Debugf("OrganizeService[PreviewOrganize] 示例扫描 ID: %s", files[0].ID)
	}

	// 按用户选中的文件ID过滤
	files = s.filterScannedFiles(source, files, fileIDs)
	logger.Infof("OrganizeService[PreviewOrganize] 过滤后剩余 %d 个文件, 目标待匹配数=%d", len(files), len(fileIDs))

	// 如果启用了智能分类，预先加载分类规则
	var categories []*domain.MediaCategory
	if useCategory && s.categoryDAO != nil {
		categories, err = s.categoryDAO.GetAll()
		if err != nil {
			return nil, fmt.Errorf("加载媒体分类失败: %v", err)
		}
	}

	overrideMap := s.buildOrganizeManualOverrideMap(manualItems)

	// 并发预览每个文件
	previews := make([]OrganizePreview, 0, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultPreviewConcurrency)

	for _, file := range files {
		wg.Add(1)
		sem <- struct{}{}

		go func(f domain.MediaFile) {
			defer wg.Done()
			defer func() { <-sem }()

			preview, err := s.previewFile(source, f, targetPath, template, categories, overrideMap)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logger.Warnf("OrganizeService[PreviewOrganize] 预览文件失败: %s, error: %v", f.Name, err)
				previews = append(previews, s.buildPreviewErrorResult(f, err))
			} else {
				previews = append(previews, *preview)
			}
		}(file)
	}
	wg.Wait()

	previews = s.appendMissingFilePreviews(previews, files, fileIDs)

	logger.Infof("OrganizeService[PreviewOrganize] 预览完成: total=%d", len(previews))
	return previews, nil
}

func (s *OrganizeService) buildPreviewErrorResult(file domain.MediaFile, err error) OrganizePreview {
	return OrganizePreview{
		FileID:        file.ID,
		CloudID:       file.CID,
		FileName:      file.Name,
		FilePath:      file.Path,
		IdentifyError: err.Error(),
	}
}

func (s *OrganizeService) OrganizeDirectory(sourceID int, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride) ([]OrganizeResult, error) {
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	return s.organizeDirectoryInternal(source, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode, fileIDs, useCategory, manualItems, renameItems, nil, nil)
}

func (s *OrganizeService) OrganizeDirectoryWithProgress(sourceID int, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride, progress func(OrganizeExecutionProgress)) ([]OrganizeResult, error) {
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	return s.organizeDirectoryInternal(source, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode, fileIDs, useCategory, manualItems, renameItems, progress, nil)
}

func (s *OrganizeService) OrganizeDirectoryWithCallbacks(sourceID int, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride, progress func(OrganizeExecutionProgress), shouldStop func() bool) ([]OrganizeResult, error) {
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	return s.organizeDirectoryInternal(source, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode, fileIDs, useCategory, manualItems, renameItems, progress, shouldStop)
}

// OrganizeDirectoryForSource 接受已构造的 *MediaSource（支持临时源 / ad-hoc 转存场景），
// 行为与 OrganizeDirectory 一致，但不再调用 GetByID（避免无 DB 媒体源时误报错）。
// 适用于「转存后自动整理」中临时拼装的 115 媒体源，以及复用既有媒体源两种路径。
func (s *OrganizeService) OrganizeDirectoryForSource(
	ctx context.Context,
	source *domain.MediaSource,
	sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string,
	fileIDs []string, useCategory bool,
	manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride,
	progress func(OrganizeExecutionProgress), shouldStop func() bool,
) ([]OrganizeResult, error) {
	return s.organizeDirectoryInternal(source, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode, fileIDs, useCategory, manualItems, renameItems, progress, shouldStop)
}

// organizeDirectoryInternal 整理执行核心。直接消费 *domain.MediaSource，供 OrganizeDirectory（按 ID 查源）
// 与 OrganizeDirectoryForSource（直接传源）共用，避免重复查询媒体源。
func (s *OrganizeService) organizeDirectoryInternal(source *domain.MediaSource, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride, progress func(OrganizeExecutionProgress), shouldStop func() bool) ([]OrganizeResult, error) {
	operationMode = normalizeOrganizeOperationMode(operationMode)
	if operationMode == "" {
		return nil, fmt.Errorf("unsupported organize mode")
	}

	logger.Infof("OrganizeService[OrganizeDirectory] start: source_id=%d, source_path=%s, target_path=%s, operation_mode=%s",
		source.ID, sourcePath, targetPath, operationMode)

	previews, err := s.previewOrganizeForSource(source, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)
	if err != nil {
		return nil, err
	}

	if len(renameItems) > 0 {
		s.applyOrganizeRenameOverrides(previews, source, renameItems)
	}

	results := make([]OrganizeResult, 0, len(previews))
	total := len(previews)
	processed := 0
	successCount := 0
	failedCount := 0
	skippedCount := 0

	reportProgress := func(result *OrganizeResult) {
		if progress == nil {
			return
		}
		progress(OrganizeExecutionProgress{
			Total:     total,
			Processed: processed,
			Success:   successCount,
			Failed:    failedCount,
			Skipped:   skippedCount,
			Result:    result,
		})
	}

	for _, preview := range previews {
		if shouldStop != nil && shouldStop() {
			return results, fmt.Errorf("任务已取消")
		}

		if preview.IdentifyError != "" {
			result := OrganizeResult{
				FileID:   preview.FileID,
				FileName: preview.FileName,
				Success:  false,
				Message:  preview.IdentifyError,
			}
			results = append(results, result)
			processed++
			failedCount++
			reportProgress(&result)
			continue
		}

		result, err := s.organizeFileForSource(source, preview, conflictPolicy, operationMode)
		if err != nil {
			logger.Warnf("OrganizeService[OrganizeDirectory] organize failed: %s, error: %v", preview.FileName, err)
			failedResult := OrganizeResult{
				FileID:   preview.FileID,
				FileName: preview.FileName,
				Success:  false,
				Message:  err.Error(),
				OldPath:  preview.FilePath,
			}
			results = append(results, failedResult)
			processed++
			failedCount++
			reportProgress(&failedResult)
			continue
		}

		// 仅对持久化媒体源（ID>0）触发内置刮削；临时/ad-hoc 源（如转存整理临时源）跳过，
		// 避免对 115 路径误触发以及无谓的 DB 查询，真实刮削由 ShareTransferService 编排负责。
		if source.ID > 0 {
			s.maybeScrapeOrganizedResult(source.ID, *result)
		}
		results = append(results, *result)
		processed++
		if result.Skipped {
			skippedCount++
		} else if result.Success {
			successCount++
		} else {
			failedCount++
		}
		reportProgress(result)
	}

	logger.Infof("OrganizeService[OrganizeDirectory] completed: total=%d", len(results))
	return results, nil
}

func (s *OrganizeService) buildOrganizeRenameOverrideMap(items []domain.OrganizeRenameOverride) map[string]domain.OrganizeRenameOverride {
	if len(items) == 0 {
		return nil
	}

	result := make(map[string]domain.OrganizeRenameOverride, len(items)*2)
	for _, item := range items {
		if fileID := strings.TrimSpace(item.FileID); fileID != "" {
			result["file:"+s.normalizePath(fileID)] = item
		}
		if cloudID := strings.TrimSpace(item.CloudID); cloudID != "" {
			result["cloud:"+s.normalizePath(cloudID)] = item
		}
	}
	return result
}

func (s *OrganizeService) matchOrganizeRenameOverride(preview OrganizePreview, overrideMap map[string]domain.OrganizeRenameOverride) *domain.OrganizeRenameOverride {
	if len(overrideMap) == 0 {
		return nil
	}

	if fileID := strings.TrimSpace(preview.FileID); fileID != "" {
		if item, ok := overrideMap["file:"+s.normalizePath(fileID)]; ok {
			return &item
		}
	}
	if cloudID := strings.TrimSpace(preview.CloudID); cloudID != "" {
		if item, ok := overrideMap["cloud:"+s.normalizePath(cloudID)]; ok {
			return &item
		}
	}
	return nil
}

func (s *OrganizeService) applyOrganizeRenameOverrides(previews []OrganizePreview, source *domain.MediaSource, renameItems []domain.OrganizeRenameOverride) {
	overrideMap := s.buildOrganizeRenameOverrideMap(renameItems)
	if len(overrideMap) == 0 || source == nil {
		return
	}

	for i := range previews {
		override := s.matchOrganizeRenameOverride(previews[i], overrideMap)
		if override == nil {
			continue
		}

		newName := strings.TrimSpace(override.NewName)
		if newName == "" {
			continue
		}

		if source.SourceType == domain.SourceTypeCloud115 {
			newName = pathpkg.Base(strings.ReplaceAll(newName, "\\", "/"))
		} else {
			newName = filepath.Base(newName)
		}
		if newName == "" {
			continue
		}

		ext := filepath.Ext(previews[i].NewName)
		if ext == "" {
			ext = filepath.Ext(previews[i].FileName)
		}
		if filepath.Ext(newName) == "" && ext != "" {
			newName += ext
		}

		previews[i].NewName = newName
		if source.SourceType == domain.SourceTypeCloud115 {
			baseTargetPath := filepath.ToSlash(strings.TrimSpace(previews[i].TargetPath))
			if baseTargetPath == "" {
				baseTargetPath = "/"
			}
			previews[i].NewPath = pathpkg.Join(baseTargetPath, newName)
			previews[i].Conflict = false
			previews[i].ConflictPath = ""
			continue
		}

		previews[i].NewPath = filepath.Join(previews[i].TargetPath, newName)
		if _, err := os.Stat(previews[i].NewPath); err == nil {
			previews[i].Conflict = true
			previews[i].ConflictPath = previews[i].NewPath
		} else {
			previews[i].Conflict = false
			previews[i].ConflictPath = ""
		}
	}
}

func (s *OrganizeService) maybeScrapeOrganizedResult(sourceID int, result OrganizeResult) {
	if s.scrapeService == nil || !result.Success || result.Skipped || strings.TrimSpace(result.NewPath) == "" {
		return
	}
	if !s.getBoolConfig("scrape_enabled_on_organize", true) {
		return
	}
	if !filepath.IsAbs(result.NewPath) {
		return
	}
	if _, err := os.Stat(result.NewPath); err != nil {
		logger.Warnf("OrganizeService[maybeScrapeOrganizedResult] skip scrape because file is missing: path=%s err=%v", result.NewPath, err)
		return
	}

	if _, _, _, err := s.scrapeService.ScrapeAbsoluteFile(sourceID, result.NewPath); err != nil {
		logger.Warnf("OrganizeService[maybeScrapeOrganizedResult] scrape failed: path=%s err=%v", result.NewPath, err)
	}
}

func (s *OrganizeService) getBoolConfig(key string, defaultValue bool) bool {
	if s.systemConfigDAO == nil {
		return defaultValue
	}
	config, err := s.systemConfigDAO.GetByKey(key)
	if err != nil || config == nil {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(config.ConfigVal)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func (s *OrganizeService) BatchIdentify(sourceID int, fileIDs []string) ([]domain.TmdbIdentifyResult, error) {
	logger.Infof("OrganizeService[BatchIdentify] 开始批量识别: source_id=%d, count=%d", sourceID, len(fileIDs))

	// 获取媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	results := make([]domain.TmdbIdentifyResult, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		// 获取文件名
		fileName := filepath.Base(fileID)

		// TMDB 识别
		identifyResult, err := s.tmdbService.IdentifyFileWithPath(fileID)
		if err != nil {
			logger.Warnf("OrganizeService[BatchIdentify] 识别失败: %s, error: %v", fileName, err)
			results = append(results, domain.TmdbIdentifyResult{
				Success:  false,
				Message:  err.Error(),
				Filename: fileName,
			})
		} else {
			results = append(results, *identifyResult)
		}
	}

	logger.Infof("OrganizeService[BatchIdentify] 批量识别完成: total=%d", len(results))
	return results, nil
}

// BatchRenamePreview 批量更名预览
// 参数:
//   - items: 预览请求列表
//
// 返回:
//   - []domain.RenamePreviewResult: 预览结果列表
//   - error: 错误信息
//
// BatchIdentifyDirectory 递归扫描目录后批量识别视频文件
func (s *OrganizeService) BatchIdentifyDirectory(sourceID int, sourcePath string, fileIDs []string) ([]domain.TmdbIdentifyResult, error) {
	candidates, err := s.ListOrganizeCandidates(sourceID, sourcePath, "all", fileIDs)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []domain.TmdbIdentifyResult{}, nil
	}

	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.FileID)
	}
	return s.BatchIdentify(sourceID, ids)
}

// ScrapeDirectory 递归扫描目录后批量刮削视频文件
func (s *OrganizeService) ScrapeDirectory(sourceID int, sourcePath string, fileIDs []string) ([]ScrapeResult, error) {
	if s.scrapeService == nil {
		return nil, fmt.Errorf("刮削服务未初始化")
	}

	candidates, err := s.ListOrganizeCandidates(sourceID, sourcePath, "all", fileIDs)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []ScrapeResult{}, nil
	}

	filePaths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		filePaths = append(filePaths, candidate.FilePath)
	}
	return s.scrapeService.ScrapeFiles(sourceID, filePaths)
}

func (s *OrganizeService) BatchRenamePreview(items []domain.RenamePreviewRequest) ([]domain.RenamePreviewResult, error) {
	logger.Infof("OrganizeService[BatchRenamePreview] 开始批量预览: count=%d", len(items))

	results := make([]domain.RenamePreviewResult, 0, len(items))
	for _, item := range items {
		result, err := s.renameService.PreviewRename(&item)
		if err != nil {
			logger.Warnf("OrganizeService[BatchRenamePreview] 预览失败: file_id=%s, error: %v", item.FileID, err)
			results = append(results, domain.RenamePreviewResult{
				OriginalName: item.FileID,
				NewName:      "",
			})
		} else {
			results = append(results, *result)
		}
	}

	logger.Infof("OrganizeService[BatchRenamePreview] 批量预览完成: total=%d", len(results))
	return results, nil
}

// BatchRenameExecute 批量执行更名
// 参数:
//   - items: 执行请求列表
//
// 返回:
//   - []domain.RenameExecuteResult: 执行结果列表
//   - error: 错误信息
func (s *OrganizeService) BatchRenameExecute(items []domain.RenameExecuteRequest) ([]domain.RenameExecuteResult, error) {
	logger.Infof("OrganizeService[BatchRenameExecute] 开始批量执行: count=%d", len(items))

	results := make([]domain.RenameExecuteResult, 0, len(items))
	for _, item := range items {
		result, err := s.renameService.ExecuteRename(&item)
		if err != nil {
			logger.Warnf("OrganizeService[BatchRenameExecute] 执行失败: file_id=%s, error: %v", item.FileID, err)
			results = append(results, domain.RenameExecuteResult{
				Success: false,
				Message: err.Error(),
			})
		} else {
			results = append(results, *result)
		}
	}

	logger.Infof("OrganizeService[BatchRenameExecute] 批量执行完成: total=%d", len(results))
	return results, nil
}

// GetRenamePresets 获取更名预设列表
// 参数:
//   - mediaType: 媒体类型（可选）
//
// 返回:
//   - []*dao.RenamePreset: 预设列表
//   - error: 错误信息
func (s *OrganizeService) GetRenamePresets(mediaType string) ([]*dao.RenamePreset, error) {
	return s.renameService.GetPresets(mediaType)
}
