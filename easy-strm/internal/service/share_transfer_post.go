package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/google/uuid"
)

// ==================== 纯函数（可单测） ====================

// postTransferVideoExts 转存后整理/刮削关注的视频扩展名白名单。
var postTransferVideoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".ts": true, ".avi": true, ".mov": true,
	".m2ts": true, ".wmv": true, ".flv": true, ".webm": true, ".iso": true,
	".rmvb": true, ".m4v": true,
}

// tvSeasonEpisodeRe 匹配 S01E02 形式
var tvSeasonEpisodeRe = regexp.MustCompile(`(?i)s\d{1,2}e\d{2}`)

// tvSeasonCnRe 匹配「第x季」
var tvSeasonCnRe = regexp.MustCompile(`第\s*\d+\s*季`)

// tvSeasonEnRe 匹配 season 关键词
var tvSeasonEnRe = regexp.MustCompile(`(?i)season`)

// inferMediaTypeFromName 根据分享目录名推断媒体类型。
// 命中剧集特征（SxxExx / 第x季 / season）返回 "tv"；否则返回兜底 "all"（由预览逐文件识别，最安全）。
func inferMediaTypeFromName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "all"
	}
	if tvSeasonEpisodeRe.MatchString(name) || tvSeasonCnRe.MatchString(name) || tvSeasonEnRe.MatchString(name) {
		return "tv"
	}
	return "all"
}

// resolveOrganizeTargetPath 解析整理目标路径。
// 优先使用请求显式指定的 organize_target_path；缺省回退到目标目录 TargetDirectory（就地整理）。
func resolveOrganizeTargetPath(req domain.TransferRequest) string {
	if p := strings.TrimSpace(req.OrganizeTargetPath); p != "" {
		return p
	}
	return req.TargetDirectory
}

// buildAdhocOrganizeSource 为 ad-hoc 转存构造一个临时（不落库）的 115 媒体源，
// 用于驱动既有 OrganizeService 整理管线。不写入数据库。
//
// 参数:
//   - req: 转存请求（提供目标账号、目标目录、冲突策略、可选 organize_target_path）
//   - targetAccount: 目标 115 账号（仅取 ID 绑定到源）
//   - shareFolderName: 分享目录名（用于源命名与媒体类型推断）
//
// 返回:
//   - *domain.MediaSource: 内存构造的临时源（ID=0，SourceType=cloud115，OperationMode=move）
func buildAdhocOrganizeSource(req domain.TransferRequest, targetAccount *domain.Cloud115, shareFolderName string) *domain.MediaSource {
	cloud115ID := req.TargetCloud115Id
	if targetAccount != nil {
		cloud115ID = targetAccount.ID
	}
	conflictPolicy := req.ConflictStrategy
	if strings.TrimSpace(conflictPolicy) == "" {
		conflictPolicy = "skip"
	}
	mediaType := inferMediaTypeFromName(shareFolderName)
	name := "转存整理-" + shareFolderName
	if strings.TrimSpace(name) == "转存整理-" {
		name = "转存整理"
	}
	return &domain.MediaSource{
		ID:                 0,
		Name:               name,
		SourceType:         domain.SourceTypeCloud115,
		Path:               req.TargetDirectory,
		Cloud115ID:         &cloud115ID,
		OrganizeTargetPath: resolveOrganizeTargetPath(req),
		MediaType:          mediaType,
		ConflictPolicy:     conflictPolicy,
		OperationMode:      "move", // 115 强制 move（参考 resolveWatchOrganizeDefaults 对 cloud115 的处理）
		Enabled:            true,
	}
}

// ==================== 转存后编排（goroutine） ====================

// selectScraper 根据媒体源类型选择刮削适配器。
// 115 云盘 → Cloud115Scraper（本期降级，skipped）；本地源 → LocalScraper（真实 NFO 写入）。
func (s *ShareTransferService) selectScraper(source *domain.MediaSource) PostTransferScraper {
	if source == nil {
		return s.cloud115Scraper
	}
	if source.SourceType == domain.SourceTypeCloud115 {
		return s.cloud115Scraper
	}
	return s.localScraper
}

// patchTaskMetadata 合并式更新任务元数据（避免覆盖既有字段）。
// 直接调用 taskDAO.UpdateMetadata 会整体替换 metadata，故先读取再合并写回。
func (s *ShareTransferService) patchTaskMetadata(taskID string, patch map[string]interface{}) {
	if taskID == "" || len(patch) == 0 {
		return
	}
	task, err := s.taskDAO.Get(taskID)
	if err != nil || task == nil {
		return
	}
	meta, _ := task["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
	}
	for k, v := range patch {
		meta[k] = v
	}
	s.taskDAO.UpdateMetadata(taskID, meta)
}

// runPostTransferOrganize 转存完成后的自动整理编排（在独立 goroutine 中执行）。
//
// 行为约束：
//   - 仅当存在成功落盘文件（successFids 非空）时才会创建整理任务；
//   - 创建独立 organize 任务，元数据含 parent_task_id=转存任务ID、stage=organize；
//   - 回填转存任务元数据 organize_task_id；
//   - 调用 OrganizeDirectoryForSource 驱动整理，进度经回调桥接 UpdateProgress；
//   - 整理成功且开启 auto_scrape 时，继续触发 runPostTransferScrape；
//   - 整理失败仅影响整理任务，不回滚已成功的转存。
func (s *ShareTransferService) runPostTransferOrganize(ctx context.Context, taskId string, req domain.TransferRequest, targetAccount *domain.Cloud115, targetDirCID string, successFids []string) {
	// 安全兜底：没有任何成功落盘文件则不整理
	if len(successFids) == 0 {
		logger.Infof("[INFO] ShareTransfer | taskId=%s | action=postOrganize | skipped | reason=no_successful_files", taskId)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=postOrganize | panic=%v", taskId, r)
		}
	}()

	// 读取分享目录名（SubmitTransfer 已写入转存任务元数据）
	shareFolderName := ""
	if task, err := s.taskDAO.Get(taskId); err == nil && task != nil {
		if meta, ok := task["metadata"].(map[string]interface{}); ok && meta != nil {
			if v, ok := meta["share_folder_name"].(string); ok {
				shareFolderName = v
			}
		}
	}

	// 构造媒体源：优先复用 organize_source_id 指向的既有源，否则用临时 115 源
	var source *domain.MediaSource
	if req.OrganizeSourceID > 0 && s.mediaSourceService != nil {
		src, err := s.mediaSourceService.GetByID(req.OrganizeSourceID)
		if err != nil || src == nil {
			logger.Warnf("[INFO] ShareTransfer | taskId=%s | action=postOrganize | fallbackAdhoc | organize_source_id=%d | err=%v", taskId, req.OrganizeSourceID, err)
			source = buildAdhocOrganizeSource(req, targetAccount, shareFolderName)
		} else {
			source = src
		}
	} else {
		source = buildAdhocOrganizeSource(req, targetAccount, shareFolderName)
	}

	// 创建链式整理任务（独立任务，靠 parent_task_id 关联转存任务）
	organizeTaskID := "share_transfer_organize_" + uuid.New().String()
	organizeTaskName := fmt.Sprintf("转存后整理 - %s", source.Name)
	if err := s.taskDAO.Create(organizeTaskID, "share_transfer_organize", organizeTaskName); err != nil {
		logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=postOrganize | createTaskErr=%v", taskId, err)
		return
	}
	s.patchTaskMetadata(organizeTaskID, map[string]interface{}{
		"parent_task_id": taskId,
		"stage":          "organize",
		"source_id":      source.ID,
		"source_type":    source.SourceType,
		"source_path":    source.Path,
		"target_path":    source.OrganizeTargetPath,
		"media_type":     source.MediaType,
		"auto_organize":  true,
	})
	// 回填转存任务元数据（合并式，不覆盖既有字段）
	s.patchTaskMetadata(taskId, map[string]interface{}{"organize_task_id": organizeTaskID})
	s.taskDAO.UpdateStatus(organizeTaskID, "running")

	sourcePath := source.Path
	targetPath := source.OrganizeTargetPath
	mediaType := source.MediaType
	conflictPolicy := source.ConflictPolicy
	if strings.TrimSpace(conflictPolicy) == "" {
		conflictPolicy = "skip"
	}
	operationMode := source.OperationMode
	if strings.TrimSpace(operationMode) == "" {
		operationMode = "move"
	}

	onProgress := func(p OrganizeExecutionProgress) {
		s.taskDAO.UpdateProgress(organizeTaskID, p.Total, p.Processed, p.Success, p.Failed)
	}
	// 取消传播：读转存任务的取消标记（用户在转存任务上点取消可联动中止后续步骤）
	isCancelled := func() bool {
		return s.taskDAO.IsCancelled(taskId)
	}

	// 列出目标目录中实际落盘的视频文件（仅整理本次落盘文件，不碰目录中既有文件）。
	// 说明：115 分享 fid 与目标目录中文件的真实内部 ID 不一致，故以目录列表为准作为 fileIDs 剪枝。
	fileIDs := s.listTransferredFileIDs(targetDirCID, targetAccount)

	results, err := s.organizer.OrganizeDirectoryForSource(
		ctx, source, sourcePath, targetPath, mediaType, "", conflictPolicy, operationMode,
		fileIDs, true, nil, nil, onProgress, isCancelled,
	)
	if err != nil {
		logger.Errorf("[INFO] ShareTransfer | taskId=%s | organizeTaskId=%s | action=postOrganize | result=FAILED | err=%v", taskId, organizeTaskID, err)
		s.patchTaskMetadata(organizeTaskID, map[string]interface{}{"error_message": err.Error()})
		s.taskDAO.SetError(organizeTaskID, err.Error())
		return
	}

	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failedCount++
		}
	}
	s.taskDAO.UpdateProgress(organizeTaskID, len(results), len(results), successCount, failedCount)
	if failedCount > 0 && successCount == 0 {
		s.taskDAO.SetError(organizeTaskID, "整理全部失败")
		return
	}
	s.taskDAO.UpdateStatus(organizeTaskID, "completed")

	// 整理成功且开启 auto_scrape → 触发刮削（仅对成功且未跳过的文件）
	if req.AutoScrape {
		go s.runPostTransferScrape(context.Background(), taskId, req, organizeTaskID, source, results)
	}
}

// runPostTransferScrape 转存后自动刮削编排（在独立 goroutine 中执行）。
//
// 行为约束：
//   - 创建独立 scrape 任务，元数据含 parent_task_id、stage=scrape；
//   - 回填转存任务元数据 scrape_task_id；
//   - 仅对整理成功且未跳过的文件（result.NewPath）刮削；
//   - 115 云盘：Cloud115Scraper 返回全部 skipped 且 error=nil → 任务 completed（带 scrape_status=skipped），不报错、不阻断；
//   - 本地源：LocalScraper 真实刮削，失败时仅影响刮削任务。
func (s *ShareTransferService) runPostTransferScrape(ctx context.Context, parentTaskID string, req domain.TransferRequest, organizeTaskID string, source *domain.MediaSource, organizeResults []OrganizeResult) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=postScrape | panic=%v", parentTaskID, r)
		}
	}()

	scrapeTaskID := "share_transfer_scrape_" + uuid.New().String()
	scrapeTaskName := fmt.Sprintf("转存后刮削 - %s", source.Name)
	if err := s.taskDAO.Create(scrapeTaskID, "share_transfer_scrape", scrapeTaskName); err != nil {
		logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=postScrape | createTaskErr=%v", parentTaskID, err)
		return
	}
	s.patchTaskMetadata(scrapeTaskID, map[string]interface{}{
		"parent_task_id":   parentTaskID,
		"stage":            "scrape",
		"source_id":        source.ID,
		"source_type":      source.SourceType,
		"organize_task_id": organizeTaskID,
		"auto_scrape":      true,
	})
	// 回填转存任务元数据（合并式）
	s.patchTaskMetadata(parentTaskID, map[string]interface{}{"scrape_task_id": scrapeTaskID})
	s.taskDAO.UpdateStatus(scrapeTaskID, "running")

	// 仅对成功且未跳过的文件刮削（使用整理后的新路径）
	var filePaths []string
	for _, r := range organizeResults {
		if r.Success && !r.Skipped && strings.TrimSpace(r.NewPath) != "" {
			filePaths = append(filePaths, r.NewPath)
		}
	}
	if len(filePaths) == 0 {
		s.patchTaskMetadata(scrapeTaskID, map[string]interface{}{
			"scrape_status":  "skipped",
			"scrape_reason":  "无成功文件需刮削",
			"scrape_total":   0,
			"scrape_success": 0,
			"scrape_skipped": 0,
		})
		s.taskDAO.UpdateStatus(scrapeTaskID, "completed")
		return
	}

	scraper := s.selectScraper(source)
	scrapeResults, err := scraper.Scrape(ctx, source, filePaths)
	if err != nil {
		// 真实刮削失败（本地源）：仅影响刮削任务，不回滚转存/整理
		logger.Errorf("[INFO] ShareTransfer | taskId=%s | scrapeTaskId=%s | action=postScrape | result=FAILED | err=%v", parentTaskID, scrapeTaskID, err)
		s.patchTaskMetadata(scrapeTaskID, map[string]interface{}{"scrape_status": "failed", "scrape_error": err.Error()})
		s.taskDAO.SetError(scrapeTaskID, err.Error())
		return
	}

	// 统计 skipped / success（115 降级：全部 skipped、error=nil → completed）
	succeeded := 0
	skipped := 0
	var firstReason string
	for _, r := range scrapeResults {
		if r.Success {
			succeeded++
		} else {
			skipped++
			if firstReason == "" {
				firstReason = r.Message
			}
		}
	}
	meta := map[string]interface{}{
		"scrape_total":   len(scrapeResults),
		"scrape_success": succeeded,
		"scrape_skipped": skipped,
	}
	if skipped > 0 && succeeded == 0 {
		meta["scrape_status"] = "skipped"
		if firstReason == "" {
			firstReason = "115 云盘暂不支持 NFO 写入"
		}
		meta["scrape_reason"] = firstReason
	} else {
		meta["scrape_status"] = "completed"
	}
	s.patchTaskMetadata(scrapeTaskID, meta)
	s.taskDAO.UpdateStatus(scrapeTaskID, "completed")
}

// listTransferredFileIDs 列出目标目录中实际落盘的视频文件（用于整理 fileIDs 剪枝）。
// 注意：115 分享 fid 与目标目录中文件的真实内部 ID 不一致，故以目录列表为准。
// 若列举失败或目录为空，返回 nil（调用方退化为全量扫描目标目录）。
func (s *ShareTransferService) listTransferredFileIDs(targetDirCID string, account *domain.Cloud115) []string {
	if s.client == nil || account == nil || strings.TrimSpace(targetDirCID) == "" {
		return nil
	}
	cid, err := strconv.Atoi(targetDirCID)
	if err != nil {
		return nil
	}
	resp, err := s.client.GetFileList(cid, 1, 0, 1000, account.ID, account.Cookie)
	if err != nil || resp == nil {
		return nil
	}
	ids := make([]string, 0, len(resp.Files))
	for _, f := range resp.Files {
		// 跳过目录与明确非视频文件
		if f.FileID == "" || f.Type == "folder" {
			continue
		}
		ext := strings.ToLower(extOf(f.Name))
		if !postTransferVideoExts[ext] {
			continue
		}
		ids = append(ids, f.Name)
	}
	return ids
}

// extOf 返回文件名的小写扩展名（含点），空文件名返回空字符串。
func extOf(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx < 0 {
		return ""
	}
	return name[idx:]
}
