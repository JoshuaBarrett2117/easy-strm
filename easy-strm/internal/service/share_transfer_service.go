package service

import (
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	shareCacheKeyPrefix = "easy_strm:share:cache:" // 分享缓存Redis key前缀
	shareCacheTTL       = 5 * time.Minute          // 分享缓存过期时间

	// 转存状态常量
	transferStatusQueued       = "queued"
	transferStatusTransferring = "transferring"
	transferStatusCompleted    = "completed"
	transferStatusPartialFail  = "partial_failed"
	transferStatusFailed       = "failed"
	transferStatusCancelled    = "cancelled"

	// 日志状态
	logStatusPending = "pending"
	logStatusSuccess = "success"
	logStatusSkipped = "skipped"
	logStatusFailed  = "failed"

	// 错误码常量
	errCodeInvalidURL       = "INVALID_URL"
	errCodePasswordRequired = "PASSWORD_REQUIRED"
	errCodeShareExpired     = "SHARE_EXPIRED"
	errCodeShareNotFound    = "SHARE_NOT_FOUND"
	errCodeParseFailed      = "PARSE_FAILED"
)

// ShareTransferService 115分享链接转存服务
// 提供分享链接解析、文件浏览和批量转存的核心业务逻辑
type ShareTransferService struct {
	client             Cloud115Client           // 115客户端接口
	taskDAO            TaskDAO                  // 任务状态DAO（接口，便于测试注入内存实现）
	logDAO             *dao.ShareTransferLogDAO // 转存日志PG DAO
	cloud115DAO        *dao.Cloud115DAO         // 115账号DAO
	redisClient        *redis.Client            // Redis客户端
	transferLimiter    *TransferLimiter         // 转存速率控制器
	organizer          PostTransferOrganizer    // 转存后自动整理依赖（支持临时 *MediaSource 直传）
	localScraper       PostTransferScraper      // 本地媒体源刮削适配器（真实 NFO 写入）
	cloud115Scraper    PostTransferScraper      // 115 云盘刮削降级适配器（本期 skipped）
	mediaSourceService *MediaSourceService      // 复用既有媒体源（organize_source_id>0 时）
}

// TaskDAO 任务状态 DAO 接口（抽象 dao.TaskRedisDAO），便于单元测试注入内存实现。
// 仅声明 ShareTransferService 实际调用的子集。
type TaskDAO interface {
	Create(taskID, taskType, taskName string) error
	Get(taskID string) (map[string]interface{}, error)
	UpdateStatus(taskID, status string) error
	UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error
	SetError(taskID, errMsg string) error
	UpdateMetadata(taskID string, metadata map[string]interface{}) error
	IsCancelled(taskID string) bool
	AddProcessedFileID(taskID string, fileID string) error
	SetCancelFlag(taskID string) error
	ClearCancelFlag(taskID string) error
}

// PostTransferOrganizer 转存后自动整理依赖接口。
// 抽象 OrganizeService.OrganizeDirectoryForSource，便于测试时注入 mock。
type PostTransferOrganizer interface {
	OrganizeDirectoryForSource(ctx context.Context, source *domain.MediaSource, sourcePath, targetPath, mediaType, template, conflictPolicy, operationMode string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride, renameItems []domain.OrganizeRenameOverride, progress func(OrganizeExecutionProgress), shouldStop func() bool) ([]OrganizeResult, error)
}

// NewShareTransferService 创建分享转存服务实例
// 参数:
//   - client: 115客户端接口
//   - taskDAO: 任务状态DAO（注入 *dao.TaskRedisDAO 即可）
//   - logDAO: 转存日志DAO
//   - cloud115DAO: 115账号DAO
//   - organizer: 转存后整理服务（注入 OrganizeService 以满足 PostTransferOrganizer）
//   - localScraper: 本地源刮削适配器
//   - cloud115Scraper: 115 云盘刮削降级适配器
//   - mediaSourceService: 媒体源服务（organize_source_id>0 时取复用源）
//
// 返回:
//   - *ShareTransferService: 服务实例
func NewShareTransferService(
	client Cloud115Client,
	taskDAO TaskDAO,
	logDAO *dao.ShareTransferLogDAO,
	cloud115DAO *dao.Cloud115DAO,
	organizer PostTransferOrganizer,
	localScraper PostTransferScraper,
	cloud115Scraper PostTransferScraper,
	mediaSourceService *MediaSourceService,
) *ShareTransferService {
	redisCli := dao.GetGlobalRedisClient()
	return &ShareTransferService{
		client:             client,
		taskDAO:            taskDAO,
		logDAO:             logDAO,
		cloud115DAO:        cloud115DAO,
		redisClient:        redisCli,
		transferLimiter:    NewTransferLimiter(redisCli),
		organizer:          organizer,
		localScraper:       localScraper,
		cloud115Scraper:    cloud115Scraper,
		mediaSourceService: mediaSourceService,
	}
}

// ==================== 分享链接解析 ====================

// shareCodeRe 用于从115分享链接中提取shareCode
// 匹配模式：115.com/s/{shareCode} 或 115cdn.com/s/{shareCode}
var shareCodeRe = regexp.MustCompile(`(?:115cdn\.com|115\.com)/s/(\w+)`)

// ParseShareLink 解析115分享链接，获取分享文件列表
// 采用两步策略：
//  1. 正则提取URL中的shareCode（密码可从URL query参数或password参数获取）
//  2. 通过 Cloud115Client.GetShareSnap 调用 115 官方接口
//     GET https://115cdn.com/webapi/share/snap（query传参 + Referer头，由115driver封装）
//
// 成功后将结果缓存到Redis（TTL 5分钟）
//
// 参数:
//   - ctx: 上下文
//   - url: 115分享链接
//   - password: 分享密码（可选）
//
// 返回:
//   - *domain.ParseShareResponse: 解析结果
//   - error: 解析失败时返回用户可读错误
func (s *ShareTransferService) ParseShareLink(ctx context.Context, url string, password string) (*domain.ParseShareResponse, error) {
	startTime := time.Now()

	// 1. 正则提取shareCode
	matches := shareCodeRe.FindStringSubmatch(url)
	if len(matches) < 2 {
		logger.Warnf("[INFO] ShareTransfer | url=%s*** | action=parse | result=INVALID_URL | duration=%s",
			truncateStr(url, 8), time.Since(startTime).String())
		return nil, fmt.Errorf("无效的115分享链接，请确认链接格式正确")
	}
	shareCode := matches[1]

	// 脱敏日志：shareCode仅记录前4位
	maskedCode := shareCode
	if len(maskedCode) > 4 {
		maskedCode = maskedCode[:4] + "***"
	}
	logger.Infof("[INFO] ShareTransfer | shareCode=%s | action=parse | start", maskedCode)

	// 2. 如果URL中包含 password query参数且未单独提供密码，则从URL提取
	if password == "" {
		password = extractSharePassword(url)
	}

	// 3. 循环分页拉取分享文件（根目录 dirID="0"，每页最多 200 条）
	// 115driver 默认 limit=20，需主动分页才能拿全（如 26 个文件只返回前 20 个）
	const shareSnapPageSize = 200
	var allFiles []driver.ShareFile
	offset := 0
	shareTitle := ""
	var snapErr error
	for {
		snapResp, err := s.client.GetShareSnap(shareCode, password, "0",
			driver.QueryLimit(shareSnapPageSize), driver.QueryOffset(offset))
		if err != nil {
			snapErr = mapShareSnapError(err)
			break
		}
		if shareTitle == "" {
			shareTitle = snapResp.Data.Shareinfo.ShareTitle
		}
		allFiles = append(allFiles, snapResp.Data.List...)
		// 终止条件：已拿全或本页为空
		if len(allFiles) >= snapResp.Data.Count || len(snapResp.Data.List) == 0 {
			break
		}
		offset += shareSnapPageSize
	}
	if snapErr != nil {
		logger.Errorf("[INFO] ShareTransfer | shareCode=%s | action=parse | result=API_ERROR | err=%v | duration=%s",
			maskedCode, snapErr, time.Since(startTime).String())
		return nil, snapErr
	}

	// 4. 转换为领域模型
	result := &domain.ParseShareResponse{
		ShareCode:  shareCode,
		FolderName: shareTitle,
		TotalFiles: len(allFiles),
	}

	var totalSize int64
	for _, f := range allFiles {
		fileInfo := convertToShareFileInfo(f)
		result.Files = append(result.Files, fileInfo)
		totalSize += fileInfo.Size
	}
	result.TotalSize = totalSize

	// 5. 缓存到Redis
	cacheKey := shareCacheKeyPrefix + shareCode
	cacheData, _ := json.Marshal(result)
	if s.redisClient != nil {
		s.redisClient.Set(ctx, cacheKey, cacheData, shareCacheTTL)
	}

	logger.Infof("[INFO] ShareTransfer | shareCode=%s | action=parse | result=OK | files=%d | size=%d | duration=%s",
		maskedCode, result.TotalFiles, result.TotalSize, time.Since(startTime).String())

	return result, nil
}

func extractSharePassword(rawURL string) string {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("password"))
}

// mapShareSnapError 将115driver返回的share/snap API错误映射为用户可读错误
// driver封装的错误信息中包含原始响应体，可通过errno或关键字匹配具体场景：
//   - 4100026 / 4100009 / 990009: 分享不存在、链接无效或已过期
//   - 990010: 分享需要访问密码
//   - 990011 / 含"提取码": 访问密码错误
func mapShareSnapError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "4100026"), strings.Contains(msg, "shared link not found"),
		strings.Contains(msg, "4100009"), strings.Contains(msg, "shared link invalid"),
		strings.Contains(msg, "990009"):
		return fmt.Errorf("分享不存在或已过期")
	case strings.Contains(msg, "990010"):
		return fmt.Errorf("分享需要访问密码")
	case strings.Contains(msg, "990011"), strings.Contains(msg, "提取码"):
		return fmt.Errorf("访问密码错误")
	default:
		return fmt.Errorf("解析分享链接失败: %v", err)
	}
}

// ==================== 分享文件浏览 ====================

// GetShareFiles 获取分享文件列表（支持分页、类型筛选、关键词搜索）
// 优先从Redis缓存读取，缓存未命中时返回错误
//
// 参数:
//   - ctx: 上下文
//   - shareCode: 分享码
//   - password: 分享密码
//   - page: 页码（从1开始）
//   - pageSize: 每页数量
//   - typeFilter: 类型筛选（video/audio/image/folder/other，为空则不过滤）
//   - keyword: 搜索关键词（为空则不过滤）
//
// 返回:
//   - *domain.ParseShareResponse: 分页后的文件列表
//   - error: 查询失败时返回错误
func (s *ShareTransferService) GetShareFiles(ctx context.Context, shareCode, password string, page, pageSize int, typeFilter, keyword string) (*domain.ParseShareResponse, error) {
	// 1. 从Redis缓存读取
	cacheKey := shareCacheKeyPrefix + shareCode
	var cacheData *domain.ParseShareResponse

	if s.redisClient != nil {
		data, err := s.redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var parsed domain.ParseShareResponse
			if json.Unmarshal([]byte(data), &parsed) == nil {
				cacheData = &parsed
			}
		}
	}

	// 2. 缓存未命中，重新解析
	if cacheData == nil {
		logger.Infof("[INFO] ShareTransfer | shareCode=%s*** | action=getFiles | cache=miss", shareCode[:min(4, len(shareCode))])
		return nil, fmt.Errorf("分享信息已过期，请重新解析分享链接")
	}

	// 3. 过滤
	filteredFiles := cacheData.Files
	if typeFilter != "" {
		var tmp []domain.ShareFileInfo
		for _, f := range filteredFiles {
			if f.Type == typeFilter || (typeFilter == "folder" && f.IsDir) {
				tmp = append(tmp, f)
			}
		}
		filteredFiles = tmp
	}
	if keyword != "" {
		keyword = strings.ToLower(keyword)
		var tmp []domain.ShareFileInfo
		for _, f := range filteredFiles {
			if strings.Contains(strings.ToLower(f.Name), keyword) {
				tmp = append(tmp, f)
			}
		}
		filteredFiles = tmp
	}

	// 4. 分页
	totalFiltered := len(filteredFiles)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if start >= totalFiltered {
		filteredFiles = []domain.ShareFileInfo{}
	} else if end > totalFiltered {
		filteredFiles = filteredFiles[start:]
	} else {
		filteredFiles = filteredFiles[start:end]
	}

	// 5. 计算分页后的总大小
	var pageSizeBytes int64
	for _, f := range filteredFiles {
		pageSizeBytes += f.Size
	}

	return &domain.ParseShareResponse{
		ShareCode:  cacheData.ShareCode,
		FolderName: cacheData.FolderName,
		Files:      filteredFiles,
		TotalFiles: totalFiltered,
		TotalSize:  pageSizeBytes,
	}, nil
}

// ==================== 提交转存任务 ====================

// SubmitTransfer 提交异步转存任务
// 校验目标账号和目录后，创建Redis任务、批量写入PG日志，启动goroutine执行。
// 当 req.AutoOrganize/req.AutoScrape 为真时，仅在任务元数据追加若干字段，
// 不改变默认转存行为；终态后的整理/刮削由 executeTransfer 钩子按开关触发（向后兼容）。
//
// 参数:
//   - ctx: 上下文
//   - req: 转存请求（含 auto_organize/auto_scrape/organize_source_id/organize_target_path）
//
// 返回:
//   - *domain.TransferResponse: 任务提交结果
//   - error: 提交失败时返回错误
func (s *ShareTransferService) SubmitTransfer(ctx context.Context, req domain.TransferRequest) (*domain.TransferResponse, error) {
	// 1. 校验目标账号存在且Cookie有效
	targetAccount, err := s.cloud115DAO.GetByID(req.TargetCloud115Id)
	if err != nil || targetAccount == nil {
		return nil, fmt.Errorf("目标115账号不存在")
	}
	if targetAccount.Cookie == "" {
		return nil, fmt.Errorf("目标115账号未登录或Cookie已失效")
	}

	// 2. 确保目标目录存在
	if req.TargetDirectory != "" {
		_, err := s.client.MkdirAll115(req.TargetDirectory, req.TargetCloud115Id, targetAccount.Cookie)
		if err != nil {
			logger.Warnf("[INFO] ShareTransfer | shareCode=%s*** | action=submit | mkdirErr=%v",
				truncateShareCode(req.ShareCode), err)
			return nil, fmt.Errorf("创建目标目录失败: %v", err)
		}
	}

	// 3. 生成任务ID
	taskId := "share-transfer-" + uuid.New().String()
	logger.Infof("[INFO] ShareTransfer | shareCode=%s*** | action=submit | taskId=%s | files=%d",
		truncateShareCode(req.ShareCode), taskId, len(req.Files))

	// 4. 获取分享缓存数据（用于获取分享目录名）
	var shareFolderName string
	cacheKey := shareCacheKeyPrefix + req.ShareCode
	if s.redisClient != nil {
		data, err := s.redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			var cached domain.ParseShareResponse
			if json.Unmarshal([]byte(data), &cached) == nil {
				shareFolderName = cached.FolderName
			}
		}
	}

	// 5. 创建Redis任务
	err = s.taskDAO.Create(taskId, "share_transfer", fmt.Sprintf("分享转存 - %s", shareFolderName))
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %v", err)
	}

	// 写入任务元数据
	metadata := map[string]interface{}{
		"share_code":        req.ShareCode,
		"share_folder_name": shareFolderName,
		"target_directory":  req.TargetDirectory,
		"target_account_id": req.TargetCloud115Id,
		"conflict_strategy": req.ConflictStrategy,
		"failed_items":      []interface{}{},
		"skipped_files":     0,
		"current_file":      "",
		// —— 转存后自动整理/刮削开关与溯源字段（向后兼容：仅多写字段，不改既有行为）——
		"auto_organize":        req.AutoOrganize,
		"auto_scrape":          req.AutoScrape,
		"organize_source_id":   req.OrganizeSourceID,
		"organize_target_path": resolveOrganizeTargetPath(req),
		"organize_task_id":     "", // 终态后回填
		"scrape_task_id":       "", // 整理成功后回填
		"parent_task_id":       "", // organize/scrape 任务反向指向转存任务
	}
	s.taskDAO.UpdateMetadata(taskId, metadata)

	// 6. 批量写入PG日志（status=pending）
	logs := make([]domain.ShareTransferLog, 0, len(req.Files))
	var estimatedSize int64
	for _, f := range req.Files {
		logs = append(logs, domain.ShareTransferLog{
			TaskId:          taskId,
			ShareCode:       req.ShareCode,
			ShareFolderName: shareFolderName,
			FileName:        f.Name,
			FilePickCode:    f.Fid,
			FileSize:        f.Size,
			Status:          logStatusPending,
		})
		estimatedSize += f.Size
	}
	if err := s.logDAO.BatchInsert(ctx, logs); err != nil {
		logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=submit | logInsertErr=%v", taskId, err)
		// 日志写入失败不阻止任务提交
	}

	// 7. 更新任务文件数
	s.taskDAO.UpdateProgress(taskId, len(req.Files), 0, 0, 0)
	s.taskDAO.UpdateStatus(taskId, transferStatusQueued)

	// 8. 启动goroutine执行转存
	go s.executeTransfer(context.Background(), taskId, req, targetAccount)

	return &domain.TransferResponse{
		TaskId:        taskId,
		TotalFiles:    len(req.Files),
		EstimatedSize: estimatedSize,
	}, nil
}

// ==================== 异步转存执行 ====================

// executeTransfer 在goroutine中执行转存任务
// 遍历文件列表，逐个调用RapidTransferFile执行秒传
// 支持取消检测、自动重试和进度上报
//
// 参数:
//   - ctx: 上下文
//   - taskId: 任务ID
//   - req: 转存请求
//   - targetAccount: 目标115账号信息
func (s *ShareTransferService) executeTransfer(ctx context.Context, taskId string, req domain.TransferRequest, targetAccount *domain.Cloud115) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=execute | panic=%v", taskId, r)
			s.taskDAO.SetError(taskId, fmt.Sprintf("转存任务异常中断: %v", r))
		}
	}()

	startTime := time.Now()
	logger.Infof("[INFO] ShareTransfer | taskId=%s | action=execute | start | files=%d", taskId, len(req.Files))

	// 更新状态为 transferring
	s.taskDAO.UpdateStatus(taskId, transferStatusTransferring)

	// 获取目标目录CID
	targetDirCID := "0"
	if req.TargetDirectory != "" {
		cid, err := s.client.GetCIDByPath(req.TargetDirectory, req.TargetCloud115Id, targetAccount.Cookie)
		if err != nil {
			errMsg := fmt.Sprintf("解析目标目录失败，已停止转存以避免文件错放: %v", err)
			logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=execute | getCIDErr=%v | transfer=aborted", taskId, err)
			s.taskDAO.SetError(taskId, errMsg)
			return
		}
		if cid == "" || (cid == "0" && strings.Trim(req.TargetDirectory, "/\\ ") != "") {
			errMsg := fmt.Sprintf("目标目录不存在或CID无效，已停止转存: %s", req.TargetDirectory)
			logger.Errorf("[INFO] ShareTransfer | taskId=%s | action=execute | targetDirectory=%s | cid=%s | transfer=aborted",
				taskId, req.TargetDirectory, cid)
			s.taskDAO.SetError(taskId, errMsg)
			return
		}
		targetDirCID = cid
	}

	// 注意：share/receive 接口由 115 后台处理转存，仅需目标账号登录态（targetAccount.Cookie），
	// 无需源分享账号 Cookie，因此不再获取 sourceCookie。

	var (
		processedFiles int
		successFiles   int
		failedFiles    int
		skippedFiles   int
		failedItems    []domain.FailedItem
		successFids    []string // 成功转存的文件 fid 集合，供后续整理/刮削编排使用
	)

	totalFiles := len(req.Files)

	// 更新metadata中的failed_items
	updateFailedItems := func() {
		itemsJSON, _ := json.Marshal(failedItems)
		var items []interface{}
		json.Unmarshal(itemsJSON, &items)

		task, _ := s.taskDAO.Get(taskId)
		if task != nil {
			metadata, _ := task["metadata"].(map[string]interface{})
			if metadata == nil {
				metadata = map[string]interface{}{}
			}
			metadata["failed_items"] = items
			metadata["skipped_files"] = skippedFiles
			s.taskDAO.UpdateMetadata(taskId, metadata)
		}
	}

	for _, file := range req.Files {
		// 检查取消标记
		if s.taskDAO.IsCancelled(taskId) {
			logger.Infof("[INFO] ShareTransfer | taskId=%s | action=execute | cancelled | processed=%d/%d",
				taskId, processedFiles, totalFiles)
			s.taskDAO.UpdateStatus(taskId, transferStatusCancelled)
			return
		}

		// 更新当前处理文件名
		task, _ := s.taskDAO.Get(taskId)
		if task != nil {
			metadata, _ := task["metadata"].(map[string]interface{})
			if metadata == nil {
				metadata = map[string]interface{}{}
			}
			metadata["current_file"] = file.Name
			s.taskDAO.UpdateMetadata(taskId, metadata)
		}

		// 速率控制
		if s.transferLimiter != nil {
			s.transferLimiter.Wait(req.TargetCloud115Id)
		}

		// 执行转存：调用 115 官方 share/receive 接口（基于分享文件 fid）
		fileStart := time.Now()
		err := s.client.ReceiveShare(
			req.ShareCode, req.Password, file.Fid,
			targetDirCID, req.TargetCloud115Id, targetAccount.Cookie,
		)

		// 自动重试1次（1s间隔），应对限频等临时错误
		if err != nil {
			logger.Warnf("[INFO] ShareTransfer | taskId=%s | file=%s | action=execute | retry | err=%v",
				taskId, file.Name, err)
			time.Sleep(1 * time.Second)
			err = s.client.ReceiveShare(
				req.ShareCode, req.Password, file.Fid,
				targetDirCID, req.TargetCloud115Id, targetAccount.Cookie,
			)
		}

		processedFiles++

		if err != nil {
			// 转存失败
			failedFiles++
			errMsg := err.Error()

			failedItems = append(failedItems, domain.FailedItem{
				Name:      file.Name,
				Error:     errMsg,
				Retryable: true, // share/receive 失败多为限频/网络，默认可重试
			})

			// 更新PG日志（以 fid 作为匹配键）
			s.logDAO.UpdateStatus(ctx, taskId, file.Fid, logStatusFailed, errMsg)

			logger.Errorf("[INFO] ShareTransfer | taskId=%s | file=%s | action=execute | result=FAILED | err=%v | duration=%s",
				taskId, file.Name, err, time.Since(fileStart).String())
		} else {
			// 转存成功
			successFiles++
			successFids = append(successFids, file.Fid)
			s.taskDAO.AddProcessedFileID(taskId, file.Fid)

			// 更新PG日志（以 fid 作为匹配键）
			s.logDAO.UpdateStatus(ctx, taskId, file.Fid, logStatusSuccess, "")

			logger.Infof("[INFO] ShareTransfer | taskId=%s | file=%s | action=execute | result=OK | duration=%s",
				taskId, file.Name, time.Since(fileStart).String())
		}

		// 更新Redis进度
		s.taskDAO.UpdateProgress(taskId, totalFiles, processedFiles, successFiles, failedFiles)
		updateFailedItems()
	}

	// 设置终态
	finalStatus := transferStatusCompleted
	if failedFiles > 0 && successFiles > 0 {
		finalStatus = transferStatusPartialFail
	} else if failedFiles == totalFiles {
		finalStatus = transferStatusFailed
	}

	s.taskDAO.UpdateStatus(taskId, finalStatus)
	s.taskDAO.UpdateProgress(taskId, totalFiles, processedFiles, successFiles, failedFiles)

	// 终态后触发转存后自动整理/刮削（向后兼容：仅当 auto_organize 为真且非 failed/cancelled 时）
	// failed：不触发；partial_failed：仍触发（只整理实际落盘文件）；cancelled：不触发。
	if (finalStatus == transferStatusCompleted || finalStatus == transferStatusPartialFail) && req.AutoOrganize {
		go s.runPostTransferOrganize(context.Background(), taskId, req, targetAccount, targetDirCID, successFids)
	}

	logger.Infof("[INFO] ShareTransfer | taskId=%s | action=execute | done | status=%s | success=%d | failed=%d | duration=%s",
		taskId, finalStatus, successFiles, failedFiles, time.Since(startTime).String())
}

// ==================== 任务进度查询 ====================

// GetProgress 查询转存任务进度
// 从Redis读取任务状态并组装完整的进度响应
//
// 参数:
//   - ctx: 上下文
//   - taskId: 任务ID
//
// 返回:
//   - *domain.TransferProgressResponse: 进度信息
//   - error: 查询失败时返回错误
func (s *ShareTransferService) GetProgress(ctx context.Context, taskId string) (*domain.TransferProgressResponse, error) {
	task, err := s.taskDAO.Get(taskId)
	if err != nil {
		return nil, fmt.Errorf("获取任务状态失败: %v", err)
	}
	if task == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskId)
	}

	// 提取字段
	status, _ := task["status"].(string)
	progress, _ := task["progress"].(float64)
	totalFiles, _ := task["total_files"].(float64)
	processedFiles, _ := task["processed_files"].(float64)
	successFiles, _ := task["success_files"].(float64)
	failedFiles, _ := task["failed_files"].(float64)
	createTime, _ := task["create_time"].(string)
	updateTime, _ := task["update_time"].(string)

	// 提取metadata
	var currentFile string
	var estimatedRemaining string
	var failedItems []domain.FailedItem
	var skipped int

	if metadata, ok := task["metadata"].(map[string]interface{}); ok && metadata != nil {
		if cf, ok := metadata["current_file"].(string); ok {
			currentFile = cf
		}
		if sk, ok := metadata["skipped_files"].(float64); ok {
			skipped = int(sk)
		}
		if fiRaw, ok := metadata["failed_items"]; ok {
			fiJSON, _ := json.Marshal(fiRaw)
			json.Unmarshal(fiJSON, &failedItems)
		}
	}

	// 计算预计剩余时间
	if processedFiles > 0 && totalFiles > 0 && status == transferStatusTransferring {
		remainingFiles := int(totalFiles) - int(processedFiles)
		if remainingFiles > 0 {
			estimatedRemaining = fmt.Sprintf("约%d个文件", remainingFiles)
		}
	}

	return &domain.TransferProgressResponse{
		TaskId:             taskId,
		Status:             status,
		Progress:           int(progress),
		TotalFiles:         int(totalFiles),
		ProcessedFiles:     int(processedFiles),
		SuccessFiles:       int(successFiles),
		FailedFiles:        int(failedFiles),
		SkippedFiles:       skipped,
		FailedItems:        failedItems,
		CurrentFile:        currentFile,
		EstimatedRemaining: estimatedRemaining,
		CreateTime:         createTime,
		UpdateTime:         updateTime,
	}, nil
}

// ==================== 取消转存 ====================

// CancelTransfer 取消正在执行的转存任务
// 设置取消标记，正在运行的goroutine会在下次迭代检测并停止
//
// 参数:
//   - ctx: 上下文
//   - taskId: 任务ID
//
// 返回:
//   - *domain.CancelResponse: 取消结果
//   - error: 取消失败时返回错误
func (s *ShareTransferService) CancelTransfer(ctx context.Context, taskId string) (*domain.CancelResponse, error) {
	task, err := s.taskDAO.Get(taskId)
	if err != nil {
		return nil, fmt.Errorf("获取任务状态失败: %v", err)
	}
	if task == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskId)
	}

	status, _ := task["status"].(string)
	if status != transferStatusQueued && status != transferStatusTransferring {
		return nil, fmt.Errorf("任务状态不允许取消: %s", status)
	}

	// 设置取消标记
	s.taskDAO.SetCancelFlag(taskId)

	// 更新状态
	s.taskDAO.UpdateStatus(taskId, transferStatusCancelled)

	processedFiles, _ := task["processed_files"].(float64)
	totalFiles, _ := task["total_files"].(float64)

	logger.Infof("[INFO] ShareTransfer | taskId=%s | action=cancel | completed=%d/%d", taskId, int(processedFiles), int(totalFiles))

	return &domain.CancelResponse{
		TaskId:         taskId,
		Status:         transferStatusCancelled,
		CompletedFiles: int(processedFiles),
		CancelledFiles: int(totalFiles) - int(processedFiles),
		Message:        "转存任务已取消",
	}, nil
}

// ==================== 重试失败文件 ====================

// RetryTransfer 重试失败的转存文件
// 读取任务metadata中的failed_items，重新提交转存
//
// 参数:
//   - ctx: 上下文
//   - taskId: 原任务ID
//
// 返回:
//   - *domain.RetryResponse: 重试结果
//   - error: 重试失败时返回错误
func (s *ShareTransferService) RetryTransfer(ctx context.Context, taskId string) (*domain.RetryResponse, error) {
	task, err := s.taskDAO.Get(taskId)
	if err != nil {
		return nil, fmt.Errorf("获取任务状态失败: %v", err)
	}
	if task == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskId)
	}

	metadata, _ := task["metadata"].(map[string]interface{})
	if metadata == nil {
		return nil, fmt.Errorf("任务元数据不存在")
	}

	// 提取失败文件
	failedItemsRaw, ok := metadata["failed_items"]
	if !ok {
		return nil, fmt.Errorf("没有可重试的失败文件")
	}

	var failedItems []domain.FailedItem
	fiJSON, _ := json.Marshal(failedItemsRaw)
	if err := json.Unmarshal(fiJSON, &failedItems); err != nil {
		return nil, fmt.Errorf("解析失败文件列表失败: %v", err)
	}

	if len(failedItems) == 0 {
		return &domain.RetryResponse{
			TaskId:       taskId,
			RetriedFiles: 0,
			Message:      "没有可重试的失败文件",
		}, nil
	}

	// 获取原任务参数
	shareCode, _ := metadata["share_code"].(string)
	targetDir, _ := metadata["target_directory"].(string)
	targetAccountID, _ := metadata["target_account_id"].(float64)
	conflictStrategy, _ := metadata["conflict_strategy"].(string)

	// 获取原任务的PG日志以匹配文件信息
	logs, err := s.logDAO.GetByTaskId(ctx, taskId)
	if err != nil {
		logger.Warnf("[INFO] ShareTransfer | taskId=%s | action=retry | getLogsErr=%v", taskId, err)
	}

	// 构建重试文件列表
	var retryFiles []domain.ShareTransferFileItem
	for _, item := range failedItems {
		if !item.Retryable {
			continue
		}
		// 从日志中匹配文件名获取pickcode
		var pickCode string
		var fileSize int64
		for _, log := range logs {
			if log.FileName == item.Name {
				pickCode = log.FilePickCode
				fileSize = log.FileSize
				break
			}
		}
		if pickCode != "" {
			retryFiles = append(retryFiles, domain.ShareTransferFileItem{
				PickCode: pickCode,
				Name:     item.Name,
				Size:     fileSize,
			})
		}
	}

	if len(retryFiles) == 0 {
		return &domain.RetryResponse{
			TaskId:       taskId,
			RetriedFiles: 0,
			Message:      "所有失败文件均不可重试",
		}, nil
	}

	// 清除取消标记
	s.taskDAO.ClearCancelFlag(taskId)

	// 重新提交转存
	retryReq := domain.TransferRequest{
		ShareCode:        shareCode,
		TargetCloud115Id: int(targetAccountID),
		TargetDirectory:  targetDir,
		Files:            retryFiles,
		ConflictStrategy: conflictStrategy,
	}

	// 获取目标账号
	targetAccount, err := s.cloud115DAO.GetByID(retryReq.TargetCloud115Id)
	if err != nil || targetAccount == nil {
		return nil, fmt.Errorf("目标115账号不存在")
	}

	// 更新任务状态并重新执行
	s.taskDAO.UpdateStatus(taskId, transferStatusTransferring)
	go s.executeTransfer(context.Background(), taskId, retryReq, targetAccount)

	logger.Infof("[INFO] ShareTransfer | taskId=%s | action=retry | retriedFiles=%d", taskId, len(retryFiles))

	return &domain.RetryResponse{
		TaskId:       taskId,
		RetriedFiles: len(retryFiles),
		Message:      fmt.Sprintf("已重新提交%d个失败文件进行转存", len(retryFiles)),
	}, nil
}

// ==================== 内部类型与辅助函数 ====================

// convertToShareFileInfo 将115driver的ShareFile（share/snap接口文件项）转换为领域模型
// 字段映射（115真实API）：n→Name, s→Size, ico→扩展名, fid→文件ID, sha→Sha1, cid→CategoryID
// 注意：share/snap接口的 fid 可能返回为整数（Go string 无法直接反序列化），
// 因此需要通过 json.RawMessage 二次提取，确保 Fid 始终为字符串。
func convertToShareFileInfo(f driver.ShareFile) domain.ShareFileInfo {
	cid := string(f.CategoryID)
	isDir := cid != "" && cid != "0"

	// 类型判断：根据文件名后缀识别媒体类型，无法识别时归为other
	fileType := "other"
	if isDir {
		fileType = "folder"
	} else {
		switch strings.ToLower(filepath.Ext(f.FileName)) {
		case ".mkv", ".mp4":
			fileType = "video"
		case ".mp3", ".flac":
			fileType = "audio"
		case ".jpg", ".png":
			fileType = "image"
		}
	}

	// 提取 fid：优先使用已解析的 FileID（字符串）；
	// 若为空则通过 json.Marshal 回退到原始 JSON 提取（兼容 115 API 返回整数的场景）
	fidStr := f.FileID
	if fidStr == "" {
		raw, err := json.Marshal(f)
		if err == nil {
			var tmp struct {
				FID interface{} `json:"fid"`
			}
			if json.Unmarshal(raw, &tmp) == nil && tmp.FID != nil {
				fidStr = fmt.Sprintf("%v", tmp.FID)
			}
		}
	}

	return domain.ShareFileInfo{
		Name:     f.FileName,
		Size:     int64(f.Size),
		Type:     fileType,
		Path:     "",
		PickCode: "",
		Fid:      fidStr,
		Sha1:     f.Sha1,
		IsDir:    isDir,
	}
}

// truncateShareCode 脱敏分享码（前4位+***）
func truncateShareCode(code string) string {
	if len(code) > 4 {
		return code[:4] + "***"
	}
	return code + "***"
}

// truncateStr 截断字符串到指定长度
func truncateStr(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// min 返回两个整数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
