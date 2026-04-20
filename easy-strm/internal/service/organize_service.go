package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
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

// OrganizePreviewTask 异步预览任务
type OrganizePreviewTask struct {
	TaskID     string        `json:"task_id"`
	SourceID   int           `json:"source_id"`
	SourcePath string        `json:"source_path"`
	TargetPath string        `json:"target_path"`
	MediaType  string        `json:"media_type"`
	FileIDs    []string      `json:"file_ids"`
	Status     string        `json:"status"` // pending, processing, completed, failed
	Progress   *TaskProgress `json:"progress,omitempty"`
	Result     *TaskResult   `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// TaskProgress 任务进度
type TaskProgress struct {
	Total       int    `json:"total"`
	Processed   int    `json:"processed"`
	CurrentFile string `json:"current_file,omitempty"`
}

// TaskResult 任务结果
type TaskResult struct {
	Previews []OrganizePreview `json:"previews"`
	Summary  *TaskSummary      `json:"summary"`
}

// TaskSummary 任务汇总
type TaskSummary struct {
	Total       int `json:"total"`
	Conflicts   int `json:"conflicts"`
	Failed      int `json:"failed"`
	Processable int `json:"processable"`
}

const (
	organizeOperationMove     = "move"
	organizeOperationCopy     = "copy"
	organizeOperationHardLink = "hardlink"
	organizeOperationSymLink  = "symlink"
)

func normalizeOrganizeOperationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", organizeOperationMove:
		return organizeOperationMove
	case organizeOperationCopy:
		return organizeOperationCopy
	case organizeOperationHardLink:
		return organizeOperationHardLink
	case organizeOperationSymLink:
		return organizeOperationSymLink
	default:
		return ""
	}
}

func organizeOperationSuccessMessage(mode string) string {
	switch normalizeOrganizeOperationMode(mode) {
	case organizeOperationCopy:
		return "整理并复制成功"
	case organizeOperationHardLink:
		return "整理并创建硬链接成功"
	case organizeOperationSymLink:
		return "整理并创建软链接成功"
	default:
		return "整理并移动成功"
	}
}

func organizeOperationActionName(mode string) string {
	switch normalizeOrganizeOperationMode(mode) {
	case organizeOperationCopy:
		return "复制"
	case organizeOperationHardLink:
		return "创建硬链接"
	case organizeOperationSymLink:
		return "创建软链接"
	default:
		return "移动"
	}
}

// PreviewOrganize 预览整理结果
// 参数:
//   - sourceID: 媒体源ID
//   - sourcePath: 源目录路径
//   - targetPath: 目标目录路径
//   - mediaType: 媒体类型 (movie/tv/all)
//   - template: 重命名模板（可空）
//   - fileIDs: 指定要处理的文件ID列表（可空，为空时处理目录下所有文件）
//   - useCategory: 是否使用媒体分类自动生成目标目录
//
// 返回:
//   - []OrganizePreview: 预览结果列表
//   - error: 错误信息
//
// ListOrganizeCandidates 列出整理候选视频文件，不触发识别预览。
func (s *OrganizeService) ListOrganizeCandidates(sourceID int, sourcePath, mediaType string, fileIDs []string) ([]OrganizeCandidate, error) {
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	files, err := s.scanFiles(source, sourcePath, mediaType, fileIDs)
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

	// 获取文件列表 (传入想要整理的 ID 以进行扫描剪枝)
	files, err := s.scanFiles(source, sourcePath, mediaType, fileIDs)
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
				previews = append(previews, OrganizePreview{
					FileID:        f.ID,
					FileName:      f.Name,
					FilePath:      f.Path,
					IdentifyError: err.Error(),
				})
			} else {
				previews = append(previews, *preview)
			}
		}(file)
	}
	wg.Wait()

	logger.Infof("OrganizeService[PreviewOrganize] 预览完成: total=%d", len(previews))
	return previews, nil
}

// GenerateTaskKey 生成任务Key
func GenerateTaskKey(sourceID int, sourcePath string, fileIDs []string) string {
	key := fmt.Sprintf("organize:preview:%d:%s", sourceID, sourcePath)
	if len(fileIDs) > 0 {
		key += ":" + strings.Join(fileIDs, ",")
	}
	return key
}

func (s *OrganizeService) persistPreviewTask(ctx context.Context, taskKey string, task *OrganizePreviewTask) error {
	if s.redisClient == nil {
		return fmt.Errorf("redis client is not initialized")
	}
	if task == nil {
		return fmt.Errorf("preview task is nil")
	}

	task.UpdatedAt = time.Now()
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal preview task failed: %w", err)
	}

	if err := s.redisClient.Set(ctx, taskKey, string(taskJSON), 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("persist preview task failed: %w", err)
	}
	return nil
}

// StartPreviewTask 启动异步预览任务
func (s *OrganizeService) StartPreviewTask(sourceID int, sourcePath, targetPath, mediaType, template string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) (string, error) {
	if s.redisClient == nil {
		return "", fmt.Errorf("预览任务存储未初始化")
	}

	taskID := fmt.Sprintf("preview_%d_%d", sourceID, time.Now().UnixNano())
	taskKey := fmt.Sprintf("organize:task:%s", taskID)

	task := OrganizePreviewTask{
		TaskID:     taskID,
		SourceID:   sourceID,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		MediaType:  mediaType,
		FileIDs:    fileIDs,
		Status:     "pending",
		Progress:   &TaskProgress{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	ctx := context.Background()
	if err := s.persistPreviewTask(ctx, taskKey, &task); err != nil {
		return "", fmt.Errorf("保存任务失败: %v", err)
	}

	go s.runPreviewTask(taskID, sourceID, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)

	return taskID, nil
}

// runPreviewTask 后台运行预览任务
func (s *OrganizeService) runPreviewTask(taskID string, sourceID int, sourcePath, targetPath, mediaType, template string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) {
	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	ctx := context.Background()

	task := OrganizePreviewTask{
		TaskID:     taskID,
		SourceID:   sourceID,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		MediaType:  mediaType,
		FileIDs:    fileIDs,
		Status:     "pending",
		Progress:   &TaskProgress{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	taskJSON, err := s.redisClient.Get(ctx, taskKey).Result()
	if err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 获取任务失败: %v", err)
		return
	}

	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 解析任务失败: %v", err)
		task.Status = "failed"
		task.Error = fmt.Sprintf("预览任务数据损坏: %v", err)
		if persistErr := s.persistPreviewTask(ctx, taskKey, &task); persistErr != nil {
			logger.Errorf("OrganizeService[runPreviewTask] 更新任务失败: %v", persistErr)
		}
		return
	}

	defer func() {
		if panicErr := recover(); panicErr != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("预览任务异常中断: %v", panicErr)
			if persistErr := s.persistPreviewTask(ctx, taskKey, &task); persistErr != nil {
				logger.Errorf("OrganizeService[runPreviewTask] 更新异常任务失败: %v", persistErr)
			}
		}
	}()

	task.Status = "processing"
	if err := s.persistPreviewTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 更新任务状态失败: %v", err)
	}

	previews, err := s.PreviewOrganize(sourceID, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		conflictCount := 0
		failedCount := 0
		for _, preview := range previews {
			if preview.Conflict {
				conflictCount++
			}
			if preview.IdentifyError != "" {
				failedCount++
			}
		}

		task.Status = "completed"
		task.Result = &TaskResult{
			Previews: previews,
			Summary: &TaskSummary{
				Total:       len(previews),
				Conflicts:   conflictCount,
				Failed:      failedCount,
				Processable: len(previews) - failedCount,
			},
		}
	}

	if err := s.persistPreviewTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 保存最终任务失败: %v", err)
	}
}

// GetPreviewTaskStatus 获取预览任务状态
func (s *OrganizeService) GetPreviewTaskStatus(taskID string) (*OrganizePreviewTask, error) {
	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	ctx := context.Background()

	taskJSON, err := s.redisClient.Get(ctx, taskKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("获取任务状态失败: %v", err)
	}

	var task OrganizePreviewTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return nil, fmt.Errorf("解析任务失败: %v", err)
	}

	return &task, nil
}

// CheckRestorableTask 检查是否有可恢复的任务
func (s *OrganizeService) CheckRestorableTask(sourceID int, sourcePath string, fileIDs []string) (*OrganizePreviewTask, error) {
	// StartPreviewTask 使用的是 organize:task:preview_<sourceID>_<timestamp>
	pattern := fmt.Sprintf("organize:task:preview_%d_*", sourceID)
	ctx := context.Background()

	keys, err := s.redisClient.Keys(ctx, pattern).Result()
	if err != nil || len(keys) == 0 {
		return nil, nil
	}

	for _, key := range keys {
		taskJSON, err := s.redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var task OrganizePreviewTask
		if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
			continue
		}

		if task.Status == "processing" || task.Status == "pending" {
			return &task, nil
		}
	}

	return nil, nil
}

// OrganizeDirectory 整理目录
// 参数:
//   - sourceID: 媒体源ID
//   - sourcePath: 源目录路径
//   - targetPath: 目标目录路径
//   - conflictPolicy: 冲突处理策略 (skip: 跳过, overwrite: 覆盖, auto_rename: 自动重命名)
//   - moveFiles: 是否删除源文件 (true: 移动, false: 复制)
//   - fileIDs: 指定要处理的文件ID列表（可空）
//   - useCategory: 是否使用媒体分类自动生成目标目录
//
// 返回:
//   - []OrganizeResult: 整理结果列表
//   - error: 错误信息
func (s *OrganizeService) OrganizeDirectory(sourceID int, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) ([]OrganizeResult, error) {
	operationMode = normalizeOrganizeOperationMode(operationMode)
	if operationMode == "" {
		return nil, fmt.Errorf("unsupported organize mode")
	}

	logger.Infof("OrganizeService[OrganizeDirectory] start: source_id=%d, source_path=%s, target_path=%s, operation_mode=%s",
		sourceID, sourcePath, targetPath, operationMode)

	previews, err := s.PreviewOrganize(sourceID, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)
	if err != nil {
		return nil, err
	}

	results := make([]OrganizeResult, 0, len(previews))
	for _, preview := range previews {
		if preview.IdentifyError != "" {
			results = append(results, OrganizeResult{
				FileID:   preview.FileID,
				FileName: preview.FileName,
				Success:  false,
				Message:  preview.IdentifyError,
			})
			continue
		}

		result, err := s.organizeFile(sourceID, preview, conflictPolicy, operationMode)
		if err != nil {
			logger.Warnf("OrganizeService[OrganizeDirectory] organize failed: %s, error: %v", preview.FileName, err)
			results = append(results, OrganizeResult{
				FileID:   preview.FileID,
				FileName: preview.FileName,
				Success:  false,
				Message:  err.Error(),
				OldPath:  preview.FilePath,
			})
			continue
		}

		s.maybeScrapeOrganizedResult(sourceID, *result)
		results = append(results, *result)
	}

	logger.Infof("OrganizeService[OrganizeDirectory] completed: total=%d", len(results))
	return results, nil
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

func (s *OrganizeService) filterFilesByIDs(files []domain.MediaFile, fileIDs []string) []domain.MediaFile {
	if len(fileIDs) == 0 {
		return files
	}

	// 统一规范化用户选中的 ID
	targets := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		targets = append(targets, s.normalizePath(id))
	}

	filtered := make([]domain.MediaFile, 0)
	for _, file := range files {
		fID := s.normalizePath(file.ID)
		cloudID := s.normalizePath(file.CID)
		match := false
		for _, target := range targets {
			// 直接匹配文件名或目录名
			if fID == target {
				match = true
				break
			}
			// 115 云盘场景下，允许使用文件/目录内部 ID 精确匹配
			if cloudID != "" && cloudID == target {
				match = true
				break
			}
			// 目录包含匹配
			if strings.HasPrefix(fID, target+"/") {
				match = true
				break
			}
		}

		if match {
			logger.Infof("OrganizeService[filterFilesByIDs] 匹配成功: %s -> Normalized: %s (在选择范围内)", file.ID, fID)
			filtered = append(filtered, file)
		} else {
			// 为了调试，抽样打印不匹配的情况
			if len(filtered) == 0 && len(files) > 0 {
				logger.Debugf("OrganizeService[filterFilesByIDs] 未匹配: fID=%s, targets=%v", fID, targets)
			}
		}
	}

	return filtered
}

func (s *OrganizeService) filterScannedFiles(source *domain.MediaSource, files []domain.MediaFile, fileIDs []string) []domain.MediaFile {
	if source != nil && source.SourceType == domain.SourceTypeCloud115 {
		return files
	}
	return s.filterFilesByIDs(files, fileIDs)
}

func (s *OrganizeService) normalizePath(p string) string {
	// 统合 slash 格式
	p = filepath.ToSlash(p)
	// 移除可能存在的驱动器盘符 (Windows 环境处理)
	if len(p) > 1 && p[1] == ':' {
		p = p[2:]
	}
	// 去除首尾连续斜杠
	p = strings.Trim(p, "/")
	return p
}

// isPathRelevant 判定当前扫描出的相对路径是否与待选 ID 相关
func (s *OrganizeService) isPathRelevant(fPath string, targets []string) (isExact bool, isAncestor bool) {
	if len(targets) == 0 {
		return false, true // 如果没有指定 ID，则默认为全量扫描
	}

	fPath = s.normalizePath(fPath)
	for _, t := range targets {
		if fPath == t {
			return true, false // 刚好是选中的目标（如选中的文件或选中的目录）
		}
		// 目标路径以当前路径开头且后面紧跟斜杠，说明当前是目标的祖先
		if strings.HasPrefix(t, fPath+"/") {
			return false, true
		}
	}
	return false, false
}

// scanFiles 扫描文件
func (s *OrganizeService) isPathOrIDRelevant(fPath string, ids []string, targets []string) (isExact bool, isAncestor bool) {
	isExact, isAncestor = s.isPathRelevant(fPath, targets)
	if isExact || isAncestor {
		return isExact, isAncestor
	}
	if len(targets) == 0 {
		return false, true
	}

	for _, id := range ids {
		if id == "" {
			continue
		}
		normalizedID := s.normalizePath(id)
		for _, target := range targets {
			if normalizedID == target {
				return true, false
			}
		}
	}
	return false, false
}

func (s *OrganizeService) scanFiles(source *domain.MediaSource, path, mediaType string, fileIDs []string) ([]domain.MediaFile, error) {
	// 格式化目标 ID 以便匹配
	targets := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		targets = append(targets, s.normalizePath(id))
	}

	// 根据媒体源类型扫描
	if source.SourceType == domain.SourceTypeLocal {
		return s.scanLocalFiles(source.Path, path, mediaType, targets)
	} else if source.SourceType == domain.SourceTypeCloud115 {
		return s.scanCloud115Files(source, path, mediaType, targets)
	}
	return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
}

// scanLocalFiles 扫描本地文件 (已添加路径剪枝)
func (s *OrganizeService) scanLocalFiles(basePath, relativePath, mediaType string, targets []string) ([]domain.MediaFile, error) {
	fullPath := filepath.Join(basePath, relativePath)

	// 读取目录
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	files := make([]domain.MediaFile, 0)
	for _, entry := range entries {
		filePath := filepath.Join(relativePath, entry.Name())
		isExact, isAncestor := s.isPathRelevant(filePath, targets)

		if !isExact && !isAncestor {
			continue // 路径无关，跳过扫描
		}

		if entry.IsDir() {
			// 如果是完全匹配的目录，则其下的所有内容都视为选中 (传入空 targets 代表全量扫描子集)
			subTargets := targets
			if isExact {
				subTargets = nil
			}
			// 递归扫描子目录
			subFiles, err := s.scanLocalFiles(basePath, filePath, mediaType, subTargets)
			if err != nil {
				logger.Warnf("OrganizeService[scanLocalFiles] 扫描子目录失败: %s, error: %v", filePath, err)
				continue
			}
			files = append(files, subFiles...)
		} else {
			// 检查文件类型
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if !s.isOrganizeVideoFile(ext) {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			files = append(files, domain.MediaFile{
				ID:          filePath,
				Name:        entry.Name(),
				Path:        filePath,
				Size:        info.Size(),
				Type:        s.getFileType(ext),
				IsDirectory: false,
				Extension:   ext,
				ModifyTime:  info.ModTime(),
			})
		}
	}

	return files, nil
}

// scanCloud115Files 扫描115云盘文件 (支持批量扫描剪枝)
func (s *OrganizeService) scanCloud115Files(source *domain.MediaSource, cidStr, mediaType string, targets []string) ([]domain.MediaFile, error) {
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("未绑定115账号")
	}

	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("获取账号信息失败: %v", err)
	}

	cid := "0"
	// 如果 cidStr 是路径格式，先转为 CID
	if cidStr != "" && cidStr != "/" && (strings.Contains(cidStr, "/") || !s.isNumeric(cidStr)) {
		realCID, err := s.client.GetCIDByPath(cidStr, cloud115.ID, cloud115.Cookie)
		if err != nil {
			logger.Warnf("OrganizeService[scanCloud115Files] 无法解析路径 %s 为 CID, 尝试作为 CID 继续: %v", cidStr, err)
			cid = cidStr
		} else {
			cid = realCID
		}
	} else if cidStr != "" {
		cid = cidStr
	}

	logger.Infof("OrganizeService[scanCloud115Files] 开始执行 115 智能扫描, CID=%s", cid)
	// 115 递归扫描，起始路径设为空，ID 由 relativePath 组成
	return s.scanCloud115Recursive(cid, "", cloud115.ID, cloud115.Cookie, mediaType, targets)
}

// scanCloud115Recursive 递归扫描115云盘 (已添加定向扫描逻辑)
func (s *OrganizeService) scanCloud115Recursive(currentCID, relativePath string, cloud115ID int, cookie, mediaType string, targets []string) ([]domain.MediaFile, error) {
	cidInt, _ := strconv.Atoi(currentCID)
	resp, err := s.client.GetFileList(cidInt, 1, 0, 1000, cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	files := make([]domain.MediaFile, 0)
	for _, f := range resp.Files {
		// 生成用于匹配的相对路径 ID
		fPath := filepath.Join(relativePath, f.Name)
		categoryID := string(f.CategoryID)
		fileID := f.FileID
		isDir := fileID == "" || f.Type == "folder"
		if isDir && fileID == "" {
			fileID = categoryID
		}

		isExact, isAncestor := s.isPathOrIDRelevant(fPath, []string{fileID, categoryID}, targets)
		if !isExact && !isAncestor {
			// 该文件夹/文件与用户选中项无关，直接剪枝并跳过 API 调用
			logger.Debugf("OrganizeService[scanCloud115Recursive] 路径无关，跳过扫描: %s", fPath)
			continue
		}

		if isDir {
			// 如果该文件夹被完全选中，则子目录不再需要 targets 过滤 (实现全量扫描)
			subTargets := targets
			if isExact {
				subTargets = nil
			}
			// 递归扫子目录
			subFiles, err := s.scanCloud115Recursive(fileID, fPath, cloud115ID, cookie, mediaType, subTargets)
			if err == nil {
				files = append(files, subFiles...)
			}
			time.Sleep(200 * time.Millisecond) // 防风控
		} else {
			ext := strings.ToLower(filepath.Ext(f.Name))
			if !s.isOrganizeVideoFile(ext) {
				continue
			}

			files = append(files, domain.MediaFile{
				ID:          fPath,
				Name:        f.Name,
				Path:        fPath,
				Type:        s.getFileType(ext),
				IsDirectory: false,
				Extension:   ext,
				CID:         f.FileID, // 115 文件的唯一 ID
			})
		}
	}
	return files, nil
}

func (s *OrganizeService) isNumeric(str string) bool {
	_, err := strconv.Atoi(str)
	return err == nil
}

// resolve115CID 解析路径为 115 CID, 逻辑同 MkdirAll115 但包含缓存或业务验证
func (s *OrganizeService) resolve115CID(path string, cloud115ID int, cookie string) (string, error) {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" || path == "/" {
		return "0", nil
	}
	if s.isNumeric(path) {
		return path, nil
	}
	return s.client.MkdirAll115(path, cloud115ID, cookie)
}

// previewFile 预览单个文件
func (s *OrganizeService) buildOrganizeTargetPath(targetPath, generatedName string) (string, string, string) {
	folderName := ""
	newName := generatedName
	if strings.Contains(generatedName, "/") || strings.Contains(generatedName, "\\") {
		folderName = filepath.Dir(generatedName)
		if folderName == "." {
			folderName = ""
		}
		newName = filepath.Base(generatedName)
	}

	finalTargetPath := targetPath
	if folderName != "" {
		finalTargetPath = filepath.Join(targetPath, s.sanitizeFolderName(folderName))
	}
	return finalTargetPath, newName, filepath.Join(finalTargetPath, newName)
}

func (s *OrganizeService) buildCloud115OrganizeTargetPath(targetPath, generatedName string) (string, string, string) {
	normalizedName := strings.ReplaceAll(generatedName, "\\", "/")
	folderName := ""
	newName := pathpkg.Base(normalizedName)
	if strings.Contains(normalizedName, "/") {
		folderName = pathpkg.Dir(normalizedName)
		if folderName == "." {
			folderName = ""
		}
	}

	finalTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
	if finalTargetPath == "" {
		finalTargetPath = "/"
	}
	if folderName != "" {
		finalTargetPath = pathpkg.Join(finalTargetPath, s.sanitizeFolderName(folderName))
	}
	return finalTargetPath, newName, pathpkg.Join(finalTargetPath, newName)
}

// prependCategoryTargetPath 将分类目录作为用户目标目录下的前缀，避免分类配置覆盖本次选择的根目录。
func (s *OrganizeService) prependCategoryTargetPath(targetPath, categoryPath string) string {
	categoryPath = strings.TrimSpace(categoryPath)
	if categoryPath == "" {
		return targetPath
	}

	if volume := filepath.VolumeName(categoryPath); volume != "" {
		categoryPath = strings.TrimPrefix(categoryPath, volume)
	}
	categoryPath = strings.TrimLeft(filepath.ToSlash(categoryPath), "/")
	if categoryPath == "" {
		return targetPath
	}

	parts := strings.Split(categoryPath, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "" {
			return filepath.Join(targetPath, s.sanitizeFolderName(part))
		}
	}
	return targetPath
}

func (s *OrganizeService) previewFile(source *domain.MediaSource, file domain.MediaFile, targetPath, template string, categories []*domain.MediaCategory, overrideMap map[string]domain.OrganizeManualOverride) (*OrganizePreview, error) {
	identifyResult, err := s.getPreferredIdentifyResult(file, s.matchOrganizeManualOverride(file, overrideMap), source.ID)
	if err != nil {
		return nil, err
	}

	// 如果启用了分类，则动态匹配出 targetPath
	if len(categories) > 0 {
		matchedPath := s.matchCategoryPath(identifyResult, categories)
		if matchedPath != "" {
			targetPath = s.prependCategoryTargetPath(targetPath, matchedPath)
		} else {
			logger.Warnf("OrganizeService[previewFile] 未命中任何分类策略，沿用请求目标目录: %s", targetPath)
		}
	}

	// 生成更名预览
	renameReq := &domain.RenamePreviewRequest{
		SourceID:  source.ID,
		FileID:    file.ID,
		TmdbID:    identifyResult.TmdbID,
		MediaType: identifyResult.MediaType,
		Template:  template,
	}

	renameResult, err := s.renameService.PreviewRename(renameReq)
	if err != nil {
		return nil, fmt.Errorf("生成更名预览失败: %v", err)
	}

	finalTargetPath, newName, newPath := s.buildOrganizeTargetPath(targetPath, renameResult.NewName)
	if source.SourceType == domain.SourceTypeCloud115 {
		finalTargetPath, newName, newPath = s.buildCloud115OrganizeTargetPath(targetPath, renameResult.NewName)
	}
	renameResult.NewName = newName
	conflict := false
	if source.SourceType == domain.SourceTypeLocal {
		_, statErr := os.Stat(newPath)
		conflict = statErr == nil
	}

	return &OrganizePreview{
		FileID:     file.ID,
		CloudID:    file.CID, // 透传 115 内部 ID
		FileName:   file.Name,
		FilePath:   file.Path,
		MediaType:  identifyResult.MediaType,
		TmdbID:     identifyResult.TmdbID,
		Title:      identifyResult.Title,
		Year:       identifyResult.Year,
		Season:     identifyResult.SeasonNumber,
		Episode:    identifyResult.EpisodeNumber,
		NewName:    renameResult.NewName,
		NewPath:    newPath,
		TargetPath: finalTargetPath, // 使用包含 Title 的路径作为实际目标
		Conflict:   conflict,
		ConflictPath: func() string {
			if conflict {
				return newPath
			}
			return ""
		}(),
	}, nil
}

func (s *OrganizeService) getPreferredIdentifyResult(file domain.MediaFile, manualOverride *domain.OrganizeManualOverride, sourceID int) (*domain.TmdbIdentifyResult, error) {
	if manualOverride != nil {
		result := s.buildManualIdentifyResult(file, *manualOverride)
		if s.tmdbService != nil {
			s.tmdbService.EnsureIdentifyMetadata(result)
		}
		s.saveIdentifyResultToCache(file, result, sourceID, true)
		return result, nil
	}

	if cached := s.getCachedIdentifyResult(file); cached != nil {
		if s.tmdbService != nil {
			s.tmdbService.EnsureIdentifyMetadata(cached)
		}
		return cached, nil
	}

	identifyResult, err := s.tmdbService.IdentifyFile(file.Name)
	if err != nil {
		return nil, fmt.Errorf("TMDB 识别失败: %v", err)
	}
	if !identifyResult.Success {
		return nil, fmt.Errorf("识别失败: %s", identifyResult.Message)
	}

	if s.tmdbService != nil {
		s.tmdbService.EnsureIdentifyMetadata(identifyResult)
	}
	s.saveIdentifyResultToCache(file, identifyResult, sourceID, false)

	return identifyResult, nil
}

func (s *OrganizeService) buildOrganizeManualOverrideMap(items []domain.OrganizeManualOverride) map[string]domain.OrganizeManualOverride {
	if len(items) == 0 {
		return nil
	}

	result := make(map[string]domain.OrganizeManualOverride, len(items)*2)
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

func (s *OrganizeService) matchOrganizeManualOverride(file domain.MediaFile, overrideMap map[string]domain.OrganizeManualOverride) *domain.OrganizeManualOverride {
	if len(overrideMap) == 0 {
		return nil
	}

	if fileID := strings.TrimSpace(file.ID); fileID != "" {
		if item, ok := overrideMap["file:"+s.normalizePath(fileID)]; ok {
			return &item
		}
	}
	if cloudID := strings.TrimSpace(file.CID); cloudID != "" {
		if item, ok := overrideMap["cloud:"+s.normalizePath(cloudID)]; ok {
			return &item
		}
	}
	return nil
}

func (s *OrganizeService) buildManualIdentifyResult(file domain.MediaFile, item domain.OrganizeManualOverride) *domain.TmdbIdentifyResult {
	mediaType := strings.TrimSpace(item.MediaType)
	if mediaType == "" {
		mediaType = "movie"
	}

	return &domain.TmdbIdentifyResult{
		Success:       true,
		Message:       "使用手动修改的识别结果",
		Filename:      file.Name,
		MediaType:     mediaType,
		TmdbID:        item.TmdbID,
		Title:         strings.TrimSpace(item.Title),
		OriginalTitle: strings.TrimSpace(item.OriginalTitle),
		Year:          item.Year,
		SeasonNumber:  item.Season,
		EpisodeNumber: item.Episode,
	}
}

func (s *OrganizeService) getCachedIdentifyResult(file domain.MediaFile) *domain.TmdbIdentifyResult {
	if s.identifyCacheDAO == nil && (s.tmdbCacheDAO == nil || s.tmdbService == nil) {
		return nil
	}

	fileHash := dao.FileHash(file.Name)

	if cached := s.getIdentifyCacheFromRedis(fileHash); cached != nil {
		logger.Debugf("OrganizeService[getCachedIdentifyResult] Redis命中: %s", file.Name)
		return cached
	}

	if cached := s.getIdentifyCacheFromDB(fileHash); cached != nil {
		s.saveIdentifyCacheToRedis(fileHash, cached)
		logger.Debugf("OrganizeService[getCachedIdentifyResult] 数据库命中: %s", file.Name)
		return cached
	}

	if s.tmdbCacheDAO != nil && s.tmdbService != nil {
		keys := make([]string, 0, 3)
		for _, key := range []string{file.ID, file.CID, strings.ToLower(strings.TrimSpace(file.Name))} {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			seen := false
			for _, existing := range keys {
				if existing == key {
					seen = true
					break
				}
			}
			if !seen {
				keys = append(keys, key)
			}
		}

		for _, key := range keys {
			for _, mediaType := range []string{"movie", "tv"} {
				cache, err := s.tmdbCacheDAO.GetByQueryKey(key, mediaType)
				if err != nil || cache == nil {
					continue
				}

				result := &domain.TmdbIdentifyResult{
					Success:       true,
					Message:       "使用已有识别结果",
					Filename:      file.Name,
					MediaType:     cache.MediaType,
					TmdbID:        cache.TmdbID,
					Title:         cache.Title,
					OriginalTitle: cache.OriginalTitle,
					Year:          cache.Year,
					SeasonNumber:  cache.SeasonNumber,
					EpisodeNumber: cache.EpisodeNumber,
				}
				applyCachedMetadata(result, cache.RawData)
				s.tmdbService.enrichCachedIdentifyMetadata(result, cache)
				return result
			}
		}
	}

	return nil
}

func (s *OrganizeService) getIdentifyCacheFromRedis(fileHash string) *domain.TmdbIdentifyResult {
	if s.redisClient == nil {
		return nil
	}
	key := fmt.Sprintf("identify:cache:%s", fileHash)
	ctx := context.Background()
	value, err := s.redisClient.Get(ctx, key).Result()
	if err != nil || value == "" {
		return nil
	}
	var cache struct {
		MediaType     string `json:"media_type"`
		TmdbID        int    `json:"tmdb_id"`
		Title         string `json:"title"`
		OriginalTitle string `json:"original_title"`
		Year          int    `json:"year"`
		SeasonNumber  int    `json:"season_number"`
		EpisodeNumber int    `json:"episode_number"`
		PosterURL     string `json:"poster_path"`
		IsManual      bool   `json:"is_manual"`
	}
	if err := json.Unmarshal([]byte(value), &cache); err != nil {
		logger.Warnf("OrganizeService[getIdentifyCacheFromRedis] JSON解析失败: %v", err)
		return nil
	}
	return &domain.TmdbIdentifyResult{
		Success:       true,
		Message:       "使用已有识别结果",
		Filename:      "",
		MediaType:     cache.MediaType,
		TmdbID:        cache.TmdbID,
		Title:         cache.Title,
		OriginalTitle: cache.OriginalTitle,
		Year:          cache.Year,
		SeasonNumber:  cache.SeasonNumber,
		EpisodeNumber: cache.EpisodeNumber,
	}
}

func (s *OrganizeService) getIdentifyCacheFromDB(fileHash string) *domain.TmdbIdentifyResult {
	cache, err := s.identifyCacheDAO.GetByFileHash(fileHash)
	if err != nil || cache == nil {
		return nil
	}
	return &domain.TmdbIdentifyResult{
		Success:       true,
		Message:       "使用已有识别结果",
		Filename:      cache.FileName,
		MediaType:     cache.MediaType,
		TmdbID:        cache.TmdbID,
		Title:         cache.Title,
		OriginalTitle: cache.OriginalTitle,
		Year:          cache.Year,
		SeasonNumber:  cache.SeasonNumber,
		EpisodeNumber: cache.EpisodeNumber,
	}
}

func (s *OrganizeService) saveIdentifyCacheToRedis(fileHash string, result *domain.TmdbIdentifyResult) {
	if s.redisClient == nil {
		return
	}
	key := fmt.Sprintf("identify:cache:%s", fileHash)
	data := map[string]interface{}{
		"media_type":     result.MediaType,
		"tmdb_id":        result.TmdbID,
		"title":          result.Title,
		"original_title": result.OriginalTitle,
		"year":           result.Year,
		"season_number":  result.SeasonNumber,
		"episode_number": result.EpisodeNumber,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Warnf("OrganizeService[saveIdentifyCacheToRedis] JSON序列化失败: %v", err)
		return
	}
	ctx := context.Background()
	if err := s.redisClient.Set(ctx, key, string(jsonData), identifyCacheRedisTTL).Err(); err != nil {
		logger.Warnf("OrganizeService[saveIdentifyCacheToRedis] 保存Redis失败: %v", err)
	}
}

func (s *OrganizeService) saveIdentifyCacheToDB(fileHash, fileName string, result *domain.TmdbIdentifyResult, sourceID int, isManual bool) {
	cache := &dao.IdentifyCache{
		FileHash:      fileHash,
		FileName:      fileName,
		MediaType:     result.MediaType,
		TmdbID:        result.TmdbID,
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		SeasonNumber:  result.SeasonNumber,
		EpisodeNumber: result.EpisodeNumber,
		IsManual:      isManual,
		SourceID:      sourceID,
	}
	if err := s.identifyCacheDAO.CreateOrUpdate(cache); err != nil {
		logger.Warnf("OrganizeService[saveIdentifyCacheToDB] 保存数据库失败: %v", err)
	}
}

func (s *OrganizeService) saveIdentifyResultToCache(file domain.MediaFile, result *domain.TmdbIdentifyResult, sourceID int, isManual bool) {
	if s.identifyCacheDAO == nil {
		return
	}
	fileHash := dao.FileHash(file.Name)
	s.saveIdentifyCacheToRedis(fileHash, result)
	s.saveIdentifyCacheToDB(fileHash, file.Name, result, sourceID, isManual)
}

// sanitizeFolderName 移除文件名中不支持的特殊字符
func (s *OrganizeService) sanitizeFolderName(name string) string {
	replacer := strings.NewReplacer(
		":", " -",
		"?", "",
		"*", "",
		"<", "",
		">", "",
		"|", "",
		"\"", "",
	)
	return strings.TrimSpace(replacer.Replace(name))
}

// matchCategoryPath 匹配并返回分类定义的目录
func (s *OrganizeService) matchCategoryPath(identifyResult *domain.TmdbIdentifyResult, categories []*domain.MediaCategory) string {
	targetTitleStr := fmt.Sprintf("%s %s", identifyResult.Title, identifyResult.OriginalTitle)
	targetTitleStr = strings.ToLower(targetTitleStr)
	defaultPath := ""
	bestPath := ""
	bestScore := -1

	for _, cat := range categories {
		if !cat.Enabled || cat.MediaType != identifyResult.MediaType {
			continue
		}

		rule := cat.GetMatchRule()
		if rule.Default {
			if defaultPath == "" {
				defaultPath = cat.TargetPath
			}
			continue
		}

		matched, score := s.matchCategoryRuleScore(identifyResult, targetTitleStr, rule)
		if matched && score > bestScore {
			bestScore = score
			bestPath = cat.TargetPath
		}
	}

	if bestPath != "" {
		return bestPath
	}
	return defaultPath
}

func (s *OrganizeService) matchCategoryRule(identifyResult *domain.TmdbIdentifyResult, targetTitleStr string, rule *domain.CategoryMatchRule) bool {
	matched, _ := s.matchCategoryRuleScore(identifyResult, targetTitleStr, rule)
	return matched
}

func (s *OrganizeService) matchCategoryRuleScore(identifyResult *domain.TmdbIdentifyResult, targetTitleStr string, rule *domain.CategoryMatchRule) (bool, int) {
	hasCondition := false
	score := 0

	if len(rule.Keywords) > 0 {
		hasCondition = true
		keywordMatched := false
		for _, kw := range rule.Keywords {
			if kw == "" {
				continue
			}
			if strings.Contains(targetTitleStr, strings.ToLower(kw)) {
				keywordMatched = true
				break
			}
		}
		if !keywordMatched {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Keywords), 60)
	}

	if len(rule.GenreIDs) > 0 {
		hasCondition = true
		if !hasAnyInt(identifyResult.GenreIDs, rule.GenreIDs) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.GenreIDs), 50)
	}

	if len(rule.Countries) > 0 {
		hasCondition = true
		if !hasAnyString(normalizeCategoryCodes(identifyResult.Countries), normalizeCategoryCodes(rule.Countries)) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Countries), 40)
	}

	if len(rule.Languages) > 0 {
		hasCondition = true
		if !hasAnyString([]string{strings.ToLower(identifyResult.Language)}, normalizeLanguageCodes(rule.Languages)) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Languages), 35)
	}

	if len(rule.Years) > 0 {
		hasCondition = true
		if !hasAnyInt([]int{identifyResult.Year}, rule.Years) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Years), 20)
	}

	if len(rule.Genres) > 0 {
		hasCondition = true
		genreMatched := false
		for _, genre := range rule.Genres {
			if strings.Contains(targetTitleStr, strings.ToLower(genre)) {
				genreMatched = true
				break
			}
		}
		if !genreMatched {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Genres), 10)
	}

	return hasCondition, score
}

func (s *OrganizeService) categoryRuleGroupScore(valueCount int, weight int) int {
	if valueCount <= 0 {
		return 0
	}
	specificityBonus := 100 - valueCount
	if specificityBonus < 1 {
		specificityBonus = 1
	}
	return weight*1000 + specificityBonus
}

func hasAnyInt(values []int, candidates []int) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func hasAnyString(values []string, candidates []string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value != "" && value == candidate {
				return true
			}
		}
	}
	return false
}

func normalizeCategoryCodes(values []string) []string {
	codes := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToUpper(value))
		if value != "" {
			codes = append(codes, value)
		}
	}
	return codes
}

func normalizeLanguageCodes(values []string) []string {
	codes := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value != "" {
			codes = append(codes, value)
		}
	}
	return codes
}

// organizeCloud115File 整理 115云盘 单个文件
func (s *OrganizeService) organizeCloud115File(source *domain.MediaSource, preview OrganizePreview, conflictPolicy, operationMode string) (*OrganizeResult, error) {
	operationMode = normalizeOrganizeOperationMode(operationMode)
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("未绑定115账号")
	}

	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("获取账号信息失败: %v", err)
	}

	// 这里的 targetPath 需要解析为目标目录的 CID
	targetPath := preview.TargetPath
	if targetPath == "" {
		targetPath = "/"
	}

	targetCID, err := s.resolve115CID(targetPath, *source.Cloud115ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("解析或创建115目标目录失败: %v", err)
	}

	fileID := preview.CloudID // 使用 115 云盘的真实内部 ID (CID/PickCode)
	if fileID == "" {
		fileID = preview.FileID // 备用
	}

	// 1. 重命名 (如果跟旧名不同)
	if preview.NewName != "" && preview.NewName != preview.FileName {
		err = s.client.RenameFile(fileID, preview.NewName, *source.Cloud115ID, cloud115.Cookie)
		if err != nil {
			return nil, fmt.Errorf("115重命名失败: %v", err)
		}
		time.Sleep(200 * time.Millisecond) // 防风控
	}

	// 2. 移动或复制
	switch operationMode {
	case organizeOperationMove:
		err = s.client.MoveFile115(fileID, targetCID, *source.Cloud115ID, cloud115.Cookie)
		if err != nil {
			return nil, fmt.Errorf("115移动文件失败: %v", err)
		}
		logger.Infof("OrganizeService[organizeCloud115File] 移动成功 %s -> CID %s", preview.FileName, targetCID)
	case organizeOperationCopy:
		err = s.client.CopyFile(fileID, targetCID, *source.Cloud115ID, cloud115.Cookie)
		if err != nil {
			return nil, fmt.Errorf("115复制文件失败: %v", err)
		}
		logger.Infof("OrganizeService[organizeCloud115File] 复制成功 %s -> CID %s", preview.FileName, targetCID)
	case organizeOperationHardLink, organizeOperationSymLink:
		return nil, fmt.Errorf("115云盘暂不支持%s整理", organizeOperationActionName(operationMode))
	default:
		return nil, fmt.Errorf("不支持的整理方式: %s", operationMode)
	}
	time.Sleep(200 * time.Millisecond) // 防风控

	return &OrganizeResult{
		FileID:   preview.FileID,
		FileName: preview.NewName, // 最后使用的新名字
		Success:  true,
		Skipped:  false,
		Message:  fmt.Sprintf("云盘整理并%s成功", organizeOperationActionName(operationMode)),
		OldPath:  preview.FilePath,
		NewPath:  preview.NewPath,
	}, nil
}

// organizeFile 整理单个文件
func (s *OrganizeService) organizeFile(sourceID int, preview OrganizePreview, conflictPolicy, operationMode string) (*OrganizeResult, error) {
	operationMode = normalizeOrganizeOperationMode(operationMode)
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	if source.SourceType != domain.SourceTypeLocal && source.SourceType != domain.SourceTypeCloud115 {
		return nil, fmt.Errorf("当前仅支持本地和115源文件整理")
	}

	// 115云盘处理逻辑
	if source.SourceType == domain.SourceTypeCloud115 {
		return s.organizeCloud115File(source, preview, conflictPolicy, operationMode)
	}

	// 以下为本地源处理逻辑
	sourcePath := filepath.Join(source.Path, preview.FilePath)
	targetPath := preview.NewPath

	if _, err := os.Stat(sourcePath); err != nil {
		return nil, fmt.Errorf("源文件不存在: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("创建目标目录失败: %v", err)
	}
	if conflictPolicy == "" {
		conflictPolicy = "skip"
	}
	if _, err := os.Stat(targetPath); err == nil {
		switch conflictPolicy {
		case "skip":
			return &OrganizeResult{
				FileID:   preview.FileID,
				FileName: preview.FileName,
				Success:  true,
				Skipped:  true,
				Message:  "目标文件已存在，已跳过",
				OldPath:  sourcePath,
				NewPath:  targetPath,
			}, nil
		case "overwrite":
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return nil, fmt.Errorf("删除冲突文件失败: %v", removeErr)
			}
		case "suffix":
			targetPath, err = s.buildUniqueTargetPath(targetPath)
			if err != nil {
				return nil, fmt.Errorf("生成冲突文件新路径失败: %v", err)
			}
		default:
			return nil, fmt.Errorf("不支持的冲突策略: %s", conflictPolicy)
		}
	}

	switch operationMode {
	case organizeOperationMove:
		if err := s.mediaSourceService.MoveFile(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("移动文件失败: %v", err)
		}
	case organizeOperationCopy:
		if err := s.mediaSourceService.CopyFile(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("复制文件失败: %v", err)
		}
	case organizeOperationHardLink:
		if err := s.mediaSourceService.CreateHardLink(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("创建硬链接失败: %v", err)
		}
	case organizeOperationSymLink:
		if err := s.mediaSourceService.CreateSymbolicLink(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("创建软链接失败: %v", err)
		}
	default:
		return nil, fmt.Errorf("不支持的整理方式: %s", operationMode)
	}

	return &OrganizeResult{
		FileID:   preview.FileID,
		FileName: preview.NewName,
		Success:  true,
		Skipped:  false,
		Message:  organizeOperationSuccessMessage(operationMode),
		OldPath:  sourcePath,
		NewPath:  targetPath,
	}, nil
}

func (s *OrganizeService) buildUniqueTargetPath(targetPath string) (string, error) {
	ext := filepath.Ext(targetPath)
	base := strings.TrimSuffix(filepath.Base(targetPath), ext)
	dir := filepath.Dir(targetPath)

	for i := 1; i <= 999; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("未找到可用的冲突后缀路径")
}

// isMediaFile 判断是否为媒体文件
func (s *OrganizeService) isMediaFile(ext string) bool {
	mediaExts := map[string]bool{
		".mp4":  true,
		".mkv":  true,
		".avi":  true,
		".mov":  true,
		".wmv":  true,
		".flv":  true,
		".webm": true,
		".m4v":  true,
		".rmvb": true,
		".rm":   true,
		".mp3":  true,
		".flac": true,
		".wav":  true,
		".aac":  true,
		".m4a":  true,
	}
	return mediaExts[ext]
}

func (s *OrganizeService) isOrganizeVideoFile(ext string) bool {
	return s.getFileType(ext) == "video"
}

// getFileType 获取文件类型
func (s *OrganizeService) getFileType(ext string) string {
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
		".rmvb": true, ".rm": true,
	}
	audioExts := map[string]bool{
		".mp3": true, ".flac": true, ".wav": true, ".aac": true, ".m4a": true,
	}

	if videoExts[ext] {
		return "video"
	}
	if audioExts[ext] {
		return "audio"
	}
	return "file"
}

// BatchIdentify 批量识别文件
// 参数:
//   - sourceID: 媒体源ID
//   - fileIDs: 文件ID列表
//
// 返回:
//   - []domain.TmdbIdentifyResult: 识别结果列表
//   - error: 错误信息
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
		identifyResult, err := s.tmdbService.IdentifyFile(fileName)
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
