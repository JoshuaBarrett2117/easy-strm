package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/google/uuid"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// 云下载服务常量
const (
	offlineTaskIDPrefix = "offline-"       // 任务中心任务ID前缀
	offlineDefaultDir   = "/云下载"           // 未指定保存目录时的默认目录
	offlineBatchSize    = 100              // 115单次云下载请求的链接数上限
	offlineQueueSize    = 1000             // 后台大批量提交队列容量
	offlinePollInterval = 10 * time.Second // 跟踪轮询115离线状态的间隔
	offlinePollTimeout  = 6 * time.Hour    // 单个跟踪任务的最长运行时间（到期后任务中心终态，115侧下载不受影响）
	offlineSyncMinGap   = 15 * time.Second // 列表查询触发同账号状态同步的最小间隔
	offlineListMaxPage  = 5                // 拉取115离线任务列表的最大页数
)

// Offline115Client 云下载所需的115客户端能力子集。
// 按调用场景单独定义（接口隔离），避免扩展 Cloud115Client 胖接口，也便于测试注入 mock。
type Offline115Client interface {
	// MkdirAll115 确保目录存在并返回目录cid
	MkdirAll115(path string, cloud115ID int, cookie string) (string, error)
	// AddOfflineTasks 批量提交离线下载链接，返回与链接顺序对应的info_hash（空串表示未被接受）
	AddOfflineTasks(uris []string, saveDirID string, cloud115ID int, cookie string) ([]string, error)
	// ListOfflineTasks 分页查询账号的离线下载任务列表（page从1开始）
	ListOfflineTasks(page int64, cloud115ID int, cookie string) (*driver.OfflineTaskResp, error)
	// DeleteOfflineTasks 删除离线下载任务，deleteFiles 为 true 时同时删除已下载文件
	DeleteOfflineTasks(hashes []string, deleteFiles bool, cloud115ID int, cookie string) error
}

// OfflineAccountStore 云下载所需的115账号读取能力（*dao.Cloud115DAO 满足该接口）
type OfflineAccountStore interface {
	GetByID(id int) (*domain.Cloud115, error)
}

// OfflineDownloadService 115云下载（离线下载）服务
// 负责提交下载链接、记录持久化、轮询跟踪115任务状态并同步任务中心进度
type OfflineDownloadService struct {
	client      Offline115Client
	taskDAO     TaskDAO
	recordDAO   *dao.OfflineDownloadTaskDAO
	cloud115DAO OfflineAccountStore

	syncMu   sync.Mutex
	lastSync map[int]time.Time // 账号维度最近一次成功同步115离线状态的时间，用于限频
	queue    chan offlineDownloadJob
}

// offlineDownloadJob 表示一个已通过入口校验、等待后台分批发送到115的大批量任务。
type offlineDownloadJob struct {
	taskID      string
	cloud115ID  int
	accountName string
	cookie      string
	directory   string
	urls        []string
	invalidURLs []string
}

// NewOfflineDownloadService 创建云下载服务实例
// 参数:
//   - client: 115客户端能力（注入 main 包 *Client）
//   - taskDAO: 任务中心状态DAO（注入 *dao.TaskRedisDAO）
//   - recordDAO: 云下载记录PG DAO
//   - cloud115DAO: 115账号读取依赖（注入 *dao.Cloud115DAO）
func NewOfflineDownloadService(client Offline115Client, taskDAO TaskDAO, recordDAO *dao.OfflineDownloadTaskDAO, cloud115DAO OfflineAccountStore) *OfflineDownloadService {
	service := &OfflineDownloadService{
		client:      client,
		taskDAO:     taskDAO,
		recordDAO:   recordDAO,
		cloud115DAO: cloud115DAO,
		lastSync:    make(map[int]time.Time),
		queue:       make(chan offlineDownloadJob, offlineQueueSize),
	}
	go service.runSubmissionQueue()
	return service
}

// ==================== 提交云下载 ====================

// Submit 提交批量云下载请求
// 校验账号与链接后调用115离线下载接口，创建任务中心跟踪任务、持久化记录并启动状态轮询。
// 全部链接均被拒绝时不创建跟踪任务，直接返回错误（拒绝明细已落库）。
func (s *OfflineDownloadService) Submit(ctx context.Context, req domain.OfflineDownloadSubmitRequest) (*domain.OfflineDownloadSubmitResponse, error) {
	urls, invalidUrls := normalizeOfflineUrls(req.Urls)
	if len(urls) == 0 && len(invalidUrls) == 0 {
		return nil, fmt.Errorf("请提供至少一个下载链接")
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("没有有效的下载链接，仅支持 ed2k/magnet/http/https/ftp 格式")
	}

	account, err := s.cloud115DAO.GetByID(req.Cloud115ID)
	if err != nil || account == nil {
		return nil, fmt.Errorf("115账号不存在")
	}
	if strings.TrimSpace(account.Cookie) == "" {
		return nil, fmt.Errorf("115账号未登录或Cookie已失效")
	}

	directory := strings.TrimSpace(req.Directory)
	if directory == "" {
		directory = offlineDefaultDir
	}
	if len(urls)+len(invalidUrls) > offlineBatchSize {
		return s.enqueueLargeSubmission(req.Cloud115ID, account, directory, urls, invalidUrls)
	}
	saveDirID, err := s.client.MkdirAll115(directory, req.Cloud115ID, account.Cookie)
	if err != nil || strings.TrimSpace(saveDirID) == "" {
		return nil, fmt.Errorf("创建保存目录失败: %v", err)
	}

	hashes, err := s.client.AddOfflineTasks(urls, saveDirID, req.Cloud115ID, account.Cookie)

	taskId := offlineTaskIDPrefix + uuid.New().String()
	results := make([]domain.OfflineDownloadUrlResult, 0, len(urls)+len(invalidUrls))
	records := make([]domain.OfflineDownloadTask, 0, len(urls)+len(invalidUrls))
	accepted := 0

	switch {
	case err == nil:
		accepted = collectBatchResults(urls, hashes, taskId, req.Cloud115ID, saveDirID, &results, &records)
	case isOfflineDuplicateErr(err), isOfflineInvalidLinkErr(err):
		// 批量接口在存在重复/无效链接时整体失败，降级为逐链接提交以识别每个链接的受理结果
		logger.Infof("[INFO] OfflineDownload | taskId=%s | action=submit | batchRejected=%v | fallback=perUrl", taskId, err)
		accepted = s.submitUrlsOneByOne(urls, saveDirID, req.Cloud115ID, account.Cookie, taskId, &results, &records)
	default:
		return nil, normalizeOfflineErr(err)
	}

	for _, u := range invalidUrls {
		results = append(results, domain.OfflineDownloadUrlResult{
			Url:     u,
			Message: "链接格式无效，仅支持 ed2k/magnet/http/https/ftp",
		})
		records = append(records, domain.OfflineDownloadTask{
			TaskId:       taskId,
			Cloud115ID:   req.Cloud115ID,
			Url:          u,
			Status:       domain.OfflineStatusFailed,
			ErrorMessage: "链接格式无效",
		})
	}

	if accepted == 0 {
		if err := s.recordDAO.BatchInsert(ctx, records); err != nil {
			logger.Warnf("[INFO] OfflineDownload | action=submit | recordInsertErr=%v", err)
		}
		reason := firstOfflineRejectReason(results)
		if reason != "" {
			return nil, fmt.Errorf("115未接受任何链接: %s", reason)
		}
		return nil, fmt.Errorf("115未接受任何链接，云下载提交失败")
	}

	if err := s.taskDAO.Create(taskId, string(domain.TaskTypeOfflineDownload), fmt.Sprintf("115云下载 - %d个任务", accepted)); err != nil {
		return nil, fmt.Errorf("创建任务失败: %v", err)
	}
	s.taskDAO.UpdateMetadata(taskId, map[string]interface{}{
		"cloud115_id":  req.Cloud115ID,
		"account_name": account.Name,
		"directory":    directory,
	})
	s.taskDAO.UpdateProgress(taskId, accepted, 0, 0, 0)
	s.taskDAO.UpdateStatus(taskId, domain.TaskStatusRunning)

	if err := s.recordDAO.BatchInsert(ctx, records); err != nil {
		// 记录写入失败不阻断任务提交，仅损失历史列表展示
		logger.Errorf("[INFO] OfflineDownload | taskId=%s | action=submit | recordInsertErr=%v", taskId, err)
	}

	go s.trackBatch(context.Background(), taskId, req.Cloud115ID, account.Cookie)

	logger.Infof("[INFO] OfflineDownload | taskId=%s | action=submit | result=OK | accepted=%d | rejected=%d | dir=%s",
		taskId, accepted, len(urls)+len(invalidUrls)-accepted, directory)

	return &domain.OfflineDownloadSubmitResponse{
		TaskId:   taskId,
		Total:    len(urls) + len(invalidUrls),
		Accepted: accepted,
		Rejected: len(urls) + len(invalidUrls) - accepted,
		Results:  results,
	}, nil
}

// enqueueLargeSubmission 为超过115单批上限的请求创建任务并放入后台队列。
// 入队响应只表示 easy-strm 已接收任务，不把尚未发送到115的链接误报为已受理。
func (s *OfflineDownloadService) enqueueLargeSubmission(cloud115ID int, account *domain.Cloud115, directory string, urls, invalidURLs []string) (*domain.OfflineDownloadSubmitResponse, error) {
	taskID := offlineTaskIDPrefix + uuid.New().String()
	total := len(urls) + len(invalidURLs)
	if err := s.taskDAO.Create(taskID, string(domain.TaskTypeOfflineDownload), fmt.Sprintf("115云下载 - %d个任务", total)); err != nil {
		return nil, fmt.Errorf("创建任务失败: %v", err)
	}
	s.taskDAO.UpdateMetadata(taskID, map[string]interface{}{
		"cloud115_id":  cloud115ID,
		"account_name": account.Name,
		"directory":    directory,
		"queue_status": "queued",
	})
	s.taskDAO.UpdateProgress(taskID, total, 0, 0, 0)
	s.taskDAO.UpdateStatus(taskID, domain.TaskStatusRunning)

	job := offlineDownloadJob{
		taskID:      taskID,
		cloud115ID:  cloud115ID,
		accountName: account.Name,
		cookie:      account.Cookie,
		directory:   directory,
		urls:        append([]string(nil), urls...),
		invalidURLs: append([]string(nil), invalidURLs...),
	}
	select {
	case s.queue <- job:
		logger.Infof("[INFO] OfflineDownload | taskId=%s | action=enqueue | total=%d | queueDepth=%d", taskID, total, len(s.queue))
	default:
		s.taskDAO.SetError(taskID, "云下载后台队列已满，请稍后重试")
		return nil, fmt.Errorf("云下载后台队列已满，请稍后重试")
	}

	return &domain.OfflineDownloadSubmitResponse{
		TaskId:      taskID,
		Total:       total,
		Rejected:    len(invalidURLs),
		Queued:      true,
		QueuedCount: len(urls),
		Results:     []domain.OfflineDownloadUrlResult{},
	}, nil
}

// runSubmissionQueue 使用单工作协程顺序消费大批量任务，避免多个批次同时冲击115接口。
func (s *OfflineDownloadService) runSubmissionQueue() {
	for job := range s.queue {
		s.processQueuedSubmission(job)
	}
}

// processQueuedSubmission 将一个大批量任务按115单批上限拆分、顺序发送并持久化结果。
func (s *OfflineDownloadService) processQueuedSubmission(job offlineDownloadJob) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Errorf("[INFO] OfflineDownload | taskId=%s | action=queueWorker | panic=%v", job.taskID, recovered)
			s.taskDAO.SetError(job.taskID, fmt.Sprintf("云下载后台提交异常: %v", recovered))
		}
	}()

	if s.taskDAO.IsCancelled(job.taskID) {
		return
	}
	saveDirID, err := s.client.MkdirAll115(job.directory, job.cloud115ID, job.cookie)
	if err != nil || strings.TrimSpace(saveDirID) == "" {
		s.failQueuedSubmission(job, fmt.Sprintf("创建保存目录失败: %v", err))
		return
	}

	accepted := 0
	rejected := 0
	for start := 0; start < len(job.urls); start += offlineBatchSize {
		end := start + offlineBatchSize
		if end > len(job.urls) {
			end = len(job.urls)
		}
		batch := job.urls[start:end]
		results := make([]domain.OfflineDownloadUrlResult, 0, len(batch))
		records := make([]domain.OfflineDownloadTask, 0, len(batch))
		batchAccepted, batchErr := s.submitOfflineBatch(batch, saveDirID, job.cloud115ID, job.cookie, job.taskID, &results, &records)
		if batchErr != nil {
			message := normalizeOfflineErr(batchErr).Error()
			records = buildQueuedFailureRecords(job, batch, saveDirID, message)
		}
		accepted += batchAccepted
		rejected += len(batch) - batchAccepted
		if insertErr := s.recordDAO.BatchInsert(context.Background(), records); insertErr != nil {
			logger.Errorf("[INFO] OfflineDownload | taskId=%s | action=queueInsert | batchStart=%d | err=%v", job.taskID, start, insertErr)
		}
		s.taskDAO.UpdateProgress(job.taskID, len(job.urls)+len(job.invalidURLs), end+len(job.invalidURLs), accepted, rejected+len(job.invalidURLs))
	}

	if len(job.invalidURLs) > 0 {
		invalidRecords := buildQueuedFailureRecords(job, job.invalidURLs, saveDirID, "链接格式无效，仅支持 ed2k/magnet/http/https/ftp")
		if insertErr := s.recordDAO.BatchInsert(context.Background(), invalidRecords); insertErr != nil {
			logger.Errorf("[INFO] OfflineDownload | taskId=%s | action=queueInsertInvalid | err=%v", job.taskID, insertErr)
		}
	}

	s.taskDAO.UpdateMetadata(job.taskID, map[string]interface{}{
		"cloud115_id":  job.cloud115ID,
		"account_name": job.accountName,
		"directory":    job.directory,
		"queue_status": "submitted",
		"accepted":     accepted,
		"rejected":     rejected + len(job.invalidURLs),
	})
	if accepted == 0 {
		s.taskDAO.SetError(job.taskID, "115未接受任何链接，后台云下载提交失败")
		return
	}
	s.taskDAO.UpdateProgress(job.taskID, accepted, 0, 0, 0)
	go s.trackBatch(context.Background(), job.taskID, job.cloud115ID, job.cookie)
	logger.Infof("[INFO] OfflineDownload | taskId=%s | action=queueSubmitted | accepted=%d | rejected=%d", job.taskID, accepted, rejected+len(job.invalidURLs))
}

// submitOfflineBatch 复用同步提交的批量与逐条降级规则，返回本批115实际受理数量。
func (s *OfflineDownloadService) submitOfflineBatch(urls []string, saveDirID string, cloud115ID int, cookie, taskID string, results *[]domain.OfflineDownloadUrlResult, records *[]domain.OfflineDownloadTask) (int, error) {
	hashes, err := s.client.AddOfflineTasks(urls, saveDirID, cloud115ID, cookie)
	switch {
	case err == nil:
		return collectBatchResults(urls, hashes, taskID, cloud115ID, saveDirID, results, records), nil
	case isOfflineDuplicateErr(err), isOfflineInvalidLinkErr(err):
		logger.Infof("[INFO] OfflineDownload | taskId=%s | action=queueBatchRejected | err=%v | fallback=perUrl", taskID, err)
		return s.submitUrlsOneByOne(urls, saveDirID, cloud115ID, cookie, taskID, results, records), nil
	default:
		return 0, err
	}
}

// failQueuedSubmission 在目录创建等整批前置失败时，为全部链接落失败记录并结束任务。
func (s *OfflineDownloadService) failQueuedSubmission(job offlineDownloadJob, message string) {
	allURLs := append(append([]string(nil), job.urls...), job.invalidURLs...)
	records := buildQueuedFailureRecords(job, allURLs, "", message)
	if err := s.recordDAO.BatchInsert(context.Background(), records); err != nil {
		logger.Errorf("[INFO] OfflineDownload | taskId=%s | action=queueFailInsert | err=%v", job.taskID, err)
	}
	s.taskDAO.UpdateProgress(job.taskID, len(allURLs), len(allURLs), 0, len(allURLs))
	s.taskDAO.SetError(job.taskID, message)
}

// buildQueuedFailureRecords 构建后台发送失败的逐链接记录。
func buildQueuedFailureRecords(job offlineDownloadJob, urls []string, saveDirID, message string) []domain.OfflineDownloadTask {
	records := make([]domain.OfflineDownloadTask, 0, len(urls))
	for _, url := range urls {
		records = append(records, domain.OfflineDownloadTask{
			TaskId:       job.taskID,
			Cloud115ID:   job.cloud115ID,
			Url:          url,
			Status:       domain.OfflineStatusFailed,
			ErrorMessage: message,
			SaveDirID:    saveDirID,
		})
	}
	return records
}

// collectBatchResults 汇总批量提交成功时的受理结果，生成逐链接结果与落库记录，返回受理数量
func collectBatchResults(urls []string, hashes []string, taskId string, cloud115ID int, saveDirID string, results *[]domain.OfflineDownloadUrlResult, records *[]domain.OfflineDownloadTask) int {
	accepted := 0
	for i, u := range urls {
		infoHash := ""
		if i < len(hashes) {
			infoHash = strings.TrimSpace(hashes[i])
		}
		record := domain.OfflineDownloadTask{
			TaskId:     taskId,
			Cloud115ID: cloud115ID,
			Url:        u,
			InfoHash:   infoHash,
			SaveDirID:  saveDirID,
		}
		result := domain.OfflineDownloadUrlResult{Url: u, InfoHash: infoHash}
		if infoHash != "" {
			accepted++
			result.Accepted = true
			record.Status = domain.OfflineStatusPending
		} else {
			result.Message = "115未接受该链接（链接无效或任务已存在）"
			record.Status = domain.OfflineStatusFailed
			record.ErrorMessage = result.Message
		}
		*results = append(*results, result)
		*records = append(*records, record)
	}
	return accepted
}

// submitUrlsOneByOne 逐链接提交离线下载，隔离单个链接的失败（重复/无效），返回受理数量。
// 对“任务已存在”的链接会尝试在115离线列表中定位现有任务并关联，使其可被继续跟踪。
func (s *OfflineDownloadService) submitUrlsOneByOne(urls []string, saveDirID string, cloud115ID int, cookie, taskId string, results *[]domain.OfflineDownloadUrlResult, records *[]domain.OfflineDownloadTask) int {
	accepted := 0
	var existingTasks []*driver.OfflineTask
	quotaExhausted := false

	for _, u := range urls {
		record := domain.OfflineDownloadTask{
			TaskId:     taskId,
			Cloud115ID: cloud115ID,
			Url:        u,
			SaveDirID:  saveDirID,
		}
		result := domain.OfflineDownloadUrlResult{Url: u}

		if quotaExhausted {
			result.Message = "115离线下载配额已用完"
			record.Status = domain.OfflineStatusFailed
			record.ErrorMessage = result.Message
			*results = append(*results, result)
			*records = append(*records, record)
			continue
		}

		hashes, addErr := s.client.AddOfflineTasks([]string{u}, saveDirID, cloud115ID, cookie)
		infoHash := ""
		if addErr == nil && len(hashes) > 0 {
			infoHash = strings.TrimSpace(hashes[0])
		}

		switch {
		case addErr == nil && infoHash != "":
			accepted++
			result.Accepted = true
			result.InfoHash = infoHash
			record.InfoHash = infoHash
			record.Status = domain.OfflineStatusPending

		case isOfflineDuplicateErr(addErr), addErr == nil:
			// 链接已存在（或115未返回hash）：尝试关联现有离线任务以便继续跟踪进度
			if existingTasks == nil {
				tasks, _, listErr := s.listAllOfflineTasks(cloud115ID, cookie)
				if listErr != nil {
					logger.Warnf("[INFO] OfflineDownload | taskId=%s | action=submit | listExistingErr=%v", taskId, listErr)
				}
				existingTasks = tasks
				if existingTasks == nil {
					existingTasks = []*driver.OfflineTask{}
				}
			}
			if matched := findMatchedOfflineTask(existingTasks, u); matched != nil {
				accepted++
				result.Accepted = true
				result.InfoHash = matched.InfoHash
				result.Message = "链接已存在离线任务，已关联现有任务"
				record.InfoHash = matched.InfoHash
				record.Name = matched.Name
				record.Size = matched.Size
				record.Percent = clampPercent(matched.Percent)
				record.Status, record.ErrorMessage = mapOfflineTaskStatus(matched)
			} else {
				result.Message = "链接已存在离线任务，但未能定位对应任务，请在115客户端查看"
				record.Status = domain.OfflineStatusFailed
				record.ErrorMessage = result.Message
			}

		case isOfflineQuotaErr(addErr):
			quotaExhausted = true
			result.Message = "115离线下载配额已用完"
			record.Status = domain.OfflineStatusFailed
			record.ErrorMessage = result.Message

		case isOfflineInvalidLinkErr(addErr):
			result.Message = "115判定链接无效"
			record.Status = domain.OfflineStatusFailed
			record.ErrorMessage = result.Message

		default:
			msg := "提交离线下载失败"
			if norm := normalizeOfflineErr(addErr); norm != nil {
				msg = norm.Error()
			}
			result.Message = msg
			record.Status = domain.OfflineStatusFailed
			record.ErrorMessage = msg
		}

		*results = append(*results, result)
		*records = append(*records, record)
	}
	return accepted
}

// listAllOfflineTasks 分页拉取115离线任务列表（最多 offlineListMaxPage 页），
// 返回任务列表与是否已完整拉取（complete 用于判断任务是否“已移除”）
func (s *OfflineDownloadService) listAllOfflineTasks(cloud115ID int, cookie string) ([]*driver.OfflineTask, bool, error) {
	tasks := make([]*driver.OfflineTask, 0)
	for page := int64(1); page <= offlineListMaxPage; page++ {
		resp, err := s.client.ListOfflineTasks(page, cloud115ID, cookie)
		if err != nil {
			return tasks, false, err
		}
		tasks = append(tasks, resp.Tasks...)
		if len(resp.Tasks) == 0 || resp.PageCount <= page {
			return tasks, true, nil
		}
	}
	return tasks, false, nil
}

// findMatchedOfflineTask 在115离线任务列表中定位与提交链接匹配的现有任务。
// 优先按链接精确匹配，其次按链接中可解析的资源hash匹配。
func findMatchedOfflineTask(tasks []*driver.OfflineTask, url string) *driver.OfflineTask {
	target := strings.ToLower(strings.TrimSpace(url))
	targetHash := extractOfflineUrlHash(target)
	var fallback *driver.OfflineTask
	for _, t := range tasks {
		if t == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(t.Url)) == target {
			return t
		}
		if targetHash != "" && strings.EqualFold(strings.TrimSpace(t.InfoHash), targetHash) && fallback == nil {
			fallback = t
		}
	}
	return fallback
}

// extractOfflineUrlHash 从链接中解析资源hash用于匹配现有115离线任务。
// magnet 链接取 btih 值，ed2k 链接取第四段hash；无法解析时返回空串。
func extractOfflineUrlHash(u string) string {
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "magnet:") {
		start := strings.Index(lower, "btih:")
		if start < 0 {
			return ""
		}
		hash := lower[start+len("btih:"):]
		if end := strings.Index(hash, "&"); end >= 0 {
			hash = hash[:end]
		}
		return strings.TrimSpace(hash)
	}
	if strings.HasPrefix(lower, "ed2k://") {
		parts := strings.Split(lower, "|")
		if len(parts) >= 4 {
			return strings.TrimSpace(parts[3])
		}
	}
	return ""
}

// firstOfflineRejectReason 返回逐链接结果中第一个非空拒绝原因，用于整体失败时的错误提示
func firstOfflineRejectReason(results []domain.OfflineDownloadUrlResult) string {
	for _, r := range results {
		if !r.Accepted && strings.TrimSpace(r.Message) != "" {
			return r.Message
		}
	}
	return ""
}

// ==================== 状态跟踪 ====================

// trackBatch 在goroutine中跟踪一个提交批次的115离线下载进度
// 终态条件：批次内全部任务到达终态、任务被取消、ctx结束或超过最长跟踪时长。
func (s *OfflineDownloadService) trackBatch(ctx context.Context, taskId string, cloud115ID int, cookie string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[INFO] OfflineDownload | taskId=%s | action=track | panic=%v", taskId, r)
			s.taskDAO.SetError(taskId, fmt.Sprintf("云下载跟踪异常中断: %v", r))
		}
	}()

	startTime := time.Now()
	logger.Infof("[INFO] OfflineDownload | taskId=%s | action=track | start | cloud115Id=%d", taskId, cloud115ID)

	if done := s.refreshBatchProgress(ctx, taskId, cloud115ID, cookie); done {
		return
	}

	ticker := time.NewTicker(offlinePollInterval)
	defer ticker.Stop()
	deadline := startTime.Add(offlinePollTimeout)

	for {
		select {
		case <-ctx.Done():
			logger.Infof("[INFO] OfflineDownload | taskId=%s | action=track | ctxDone", taskId)
			return
		case <-ticker.C:
		}

		if s.taskDAO.IsCancelled(taskId) {
			logger.Infof("[INFO] OfflineDownload | taskId=%s | action=track | cancelled", taskId)
			return
		}

		if done := s.refreshBatchProgress(ctx, taskId, cloud115ID, cookie); done {
			logger.Infof("[INFO] OfflineDownload | taskId=%s | action=track | done | duration=%s", taskId, time.Since(startTime).String())
			return
		}

		if time.Now().After(deadline) {
			logger.Warnf("[INFO] OfflineDownload | taskId=%s | action=track | timeout=%s", taskId, offlinePollTimeout.String())
			s.taskDAO.UpdateStatus(taskId, domain.TaskStatusCompleted)
			s.noteTrackingTimeout(taskId)
			return
		}
	}
}

// refreshBatchProgress 同步账号离线状态并汇总本批次进度到任务中心
// 返回 true 表示批次内全部任务已到达终态（任务中心状态已置为终态）。
func (s *OfflineDownloadService) refreshBatchProgress(ctx context.Context, taskId string, cloud115ID int, cookie string) bool {
	if _, err := s.SyncAccountOfflineTasks(ctx, cloud115ID, cookie, true); err != nil {
		logger.Warnf("[INFO] OfflineDownload | taskId=%s | action=track | syncErr=%v", taskId, err)
	}

	records, err := s.recordDAO.GetByTaskID(ctx, taskId)
	if err != nil {
		logger.Warnf("[INFO] OfflineDownload | taskId=%s | action=track | queryRecordsErr=%v", taskId, err)
		return false
	}

	total, done, success, failed := 0, 0, 0, 0
	for _, rec := range records {
		if rec.InfoHash == "" {
			// 未被115接受的链接不进入跟踪进度（落库时已是failed）
			continue
		}
		total++
		switch rec.Status {
		case domain.OfflineStatusCompleted:
			done++
			success++
		case domain.OfflineStatusFailed, domain.OfflineStatusRemoved, domain.OfflineStatusCancelled:
			done++
			failed++
		}
	}

	if total > 0 {
		s.taskDAO.UpdateProgress(taskId, total, done, success, failed)
	}
	if total == 0 || done < total {
		return false
	}

	finalStatus := domain.TaskStatusCompleted
	if failed == total {
		finalStatus = domain.TaskStatusFailed
	}
	s.taskDAO.UpdateStatus(taskId, finalStatus)
	return true
}

// noteTrackingTimeout 在任务元数据中记录跟踪超时说明（不改变任务终态）
func (s *OfflineDownloadService) noteTrackingTimeout(taskId string) {
	metadata := map[string]interface{}{}
	if task, _ := s.taskDAO.Get(taskId); task != nil {
		if m, ok := task["metadata"].(map[string]interface{}); ok && m != nil {
			metadata = m
		}
	}
	metadata["note"] = "跟踪超时结束：下载任务仍在115侧进行，请在云下载记录列表查看最新状态"
	s.taskDAO.UpdateMetadata(taskId, metadata)
}

// SyncAccountOfflineTasks 拉取115离线任务列表并刷新该账号下本地活跃记录的状态
// force 为 false 时受限频控制（offlineSyncMinGap）；返回是否实际执行了同步。
func (s *OfflineDownloadService) SyncAccountOfflineTasks(ctx context.Context, cloud115ID int, cookie string, force bool) (bool, error) {
	if !force {
		s.syncMu.Lock()
		last, ok := s.lastSync[cloud115ID]
		s.syncMu.Unlock()
		if ok && time.Since(last) < offlineSyncMinGap {
			return false, nil
		}
	}

	if strings.TrimSpace(cookie) == "" {
		account, err := s.cloud115DAO.GetByID(cloud115ID)
		if err != nil || account == nil {
			return false, fmt.Errorf("115账号不存在")
		}
		cookie = account.Cookie
	}
	if strings.TrimSpace(cookie) == "" {
		return false, fmt.Errorf("115账号未登录或Cookie已失效")
	}

	// 分页拉取115离线任务列表，建立info_hash索引
	allTasks, complete, err := s.listAllOfflineTasks(cloud115ID, cookie)
	if err != nil {
		return false, err
	}
	taskByHash := make(map[string]*driver.OfflineTask, len(allTasks))
	for _, t := range allTasks {
		if t != nil && t.InfoHash != "" {
			taskByHash[t.InfoHash] = t
		}
	}

	active, err := s.recordDAO.GetActiveByAccount(ctx, cloud115ID)
	if err != nil {
		return false, err
	}
	for _, rec := range active {
		if rec.InfoHash == "" {
			continue
		}
		t, ok := taskByHash[rec.InfoHash]
		if !ok {
			// 仅在完整拉取列表后才判定为已移除，避免分页截断误判
			if complete {
				if err := s.recordDAO.UpdateStatus(ctx, rec.ID, domain.OfflineStatusRemoved, "任务已不在115离线列表中"); err != nil {
					logger.Warnf("[INFO] OfflineDownload | action=sync | recordId=%d | markRemovedErr=%v", rec.ID, err)
				}
			}
			continue
		}
		status, errMsg := mapOfflineTaskStatus(t)
		name := t.Name
		if name == "" {
			name = rec.Name
		}
		if err := s.recordDAO.UpdateByHash(ctx, cloud115ID, rec.InfoHash, name, t.Size, status, clampPercent(t.Percent), errMsg); err != nil {
			logger.Warnf("[INFO] OfflineDownload | action=sync | cloud115Id=%d | hash=%s | updateErr=%v", cloud115ID, rec.InfoHash, err)
		}
	}

	s.syncMu.Lock()
	s.lastSync[cloud115ID] = time.Now()
	s.syncMu.Unlock()
	return true, nil
}

// ==================== 记录查询与删除 ====================

// List 分页查询云下载记录
// 对存在未完成任务的账号触发限频状态同步，同步发生后刷新当前页数据并回填账号名称。
func (s *OfflineDownloadService) List(ctx context.Context, cloud115ID int, status string, page, pageSize int) (*domain.OfflineDownloadTaskListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	records, total, err := s.recordDAO.List(ctx, cloud115ID, status, page, pageSize)
	if err != nil {
		return nil, err
	}

	// 收集需要同步状态的账号：查询指定的账号 + 当前页存在未完成任务的账号
	accountIDs := make(map[int]struct{})
	if cloud115ID > 0 {
		accountIDs[cloud115ID] = struct{}{}
	}
	for _, rec := range records {
		if rec.Status == domain.OfflineStatusPending || rec.Status == domain.OfflineStatusDownloading {
			accountIDs[rec.Cloud115ID] = struct{}{}
		}
	}
	synced := false
	for id := range accountIDs {
		ran, syncErr := s.SyncAccountOfflineTasks(ctx, id, "", false)
		if syncErr != nil {
			logger.Warnf("[INFO] OfflineDownload | action=list | syncErr | cloud115Id=%d | err=%v", id, syncErr)
			continue
		}
		if ran {
			synced = true
		}
	}
	if synced {
		if refreshed, refreshedTotal, refreshErr := s.recordDAO.List(ctx, cloud115ID, status, page, pageSize); refreshErr == nil {
			records, total = refreshed, refreshedTotal
		}
	}

	s.fillAccountNames(records)
	return &domain.OfflineDownloadTaskListResponse{Data: records, Total: total}, nil
}

// DeleteRecord 删除云下载记录；记录对应真实115离线任务且账号可用时，先删除115侧任务
// deleteFiles 为 true 时同时删除已下载完成的云端文件
func (s *OfflineDownloadService) DeleteRecord(ctx context.Context, id int64, deleteFiles bool) error {
	rec, err := s.recordDAO.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("查询云下载记录失败: %v", err)
	}
	if rec == nil {
		return fmt.Errorf("云下载记录不存在")
	}

	if rec.InfoHash != "" && rec.Status != domain.OfflineStatusRemoved {
		account, accErr := s.cloud115DAO.GetByID(rec.Cloud115ID)
		if accErr == nil && account != nil && strings.TrimSpace(account.Cookie) != "" {
			if err := s.client.DeleteOfflineTasks([]string{rec.InfoHash}, deleteFiles, rec.Cloud115ID, account.Cookie); err != nil {
				return err
			}
		}
	}

	if err := s.recordDAO.Delete(ctx, id); err != nil {
		return fmt.Errorf("删除云下载记录失败: %v", err)
	}
	logger.Infof("[INFO] OfflineDownload | action=delete | recordId=%d | hash=%s | deleteFiles=%v", id, rec.InfoHash, deleteFiles)
	return nil
}

// ==================== 辅助函数 ====================

// fillAccountNames 批量回填账号名称，账号已删除时展示兜底文案
func (s *OfflineDownloadService) fillAccountNames(records []domain.OfflineDownloadTask) {
	if len(records) == 0 {
		return
	}
	names := make(map[int]string)
	for i := range records {
		id := records[i].Cloud115ID
		name, ok := names[id]
		if !ok {
			name = "账号已删除"
			if account, err := s.cloud115DAO.GetByID(id); err == nil && account != nil {
				name = account.Name
			}
			names[id] = name
		}
		records[i].AccountName = name
	}
}

// offlineUrlPrefixes 115离线下载支持的链接类型前缀
var offlineUrlPrefixes = []string{"ed2k://", "magnet:", "http://", "https://", "ftp://"}

// normalizeOfflineUrls 修剪空白、去空行并去重链接，拆分出有效链接与无效链接
func normalizeOfflineUrls(raw []string) (valid []string, invalid []string) {
	seen := make(map[string]struct{}, len(raw))
	for _, line := range raw {
		u := strings.TrimSpace(line)
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		if isSupportedOfflineUrl(u) {
			valid = append(valid, u)
		} else {
			invalid = append(invalid, u)
		}
	}
	return valid, invalid
}

// isSupportedOfflineUrl 判断链接是否为115离线下载支持的类型
func isSupportedOfflineUrl(u string) bool {
	lower := strings.ToLower(u)
	for _, prefix := range offlineUrlPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// mapOfflineTaskStatus 将115离线任务状态映射为本地记录状态
func mapOfflineTaskStatus(t *driver.OfflineTask) (status string, errMsg string) {
	switch {
	case t.IsDone():
		return domain.OfflineStatusCompleted, ""
	case t.IsFailed():
		return domain.OfflineStatusFailed, "115离线下载失败"
	case t.IsRunning():
		return domain.OfflineStatusDownloading, ""
	default:
		return domain.OfflineStatusPending, ""
	}
}

// clampPercent 将进度百分比规范到0-100区间
func clampPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

// offlineErrorMsgRegexp 用于从115错误响应体中提取 error_msg 字段
var offlineErrorMsgRegexp = regexp.MustCompile(`"error_msg"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// isOfflineDuplicateErr 判断错误是否为115返回的“任务已存在”（errcode 10008）。
// driver 无法解析该接口的 errcode 字段（仅识别 errno/errNo），会退化为 ErrUnexpected，
// 因此在 errors.Is 之外补充字符串兜底匹配。
func isOfflineDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrOfflineTaskExisted) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "任务已存在") || strings.Contains(msg, `"errcode":10008`)
}

// isOfflineInvalidLinkErr 判断错误是否为115返回的“无效链接”（errcode 10004）
func isOfflineInvalidLinkErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrOfflineInvalidLink) {
		return true
	}
	return strings.Contains(err.Error(), `"errcode":10004`)
}

// isOfflineQuotaErr 判断错误是否为115返回的“离线次数用尽”（errcode 10010）
func isOfflineQuotaErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrOfflineNoTimes) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, `"errcode":10010`) || strings.Contains(msg, "quota has been used up")
}

// normalizeOfflineErr 将115离线接口错误归一化为可读的中文错误，避免向前端泄漏加密响应体
func normalizeOfflineErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case isOfflineDuplicateErr(err):
		return fmt.Errorf("链接已存在离线任务，请勿重复提交")
	case isOfflineInvalidLinkErr(err):
		return fmt.Errorf("无效的下载链接，请检查链接是否有效")
	case isOfflineQuotaErr(err):
		return fmt.Errorf("115账号离线下载次数已用完")
	case errors.Is(err, driver.ErrNotLogin):
		return fmt.Errorf("115账号未登录或Cookie已失效")
	}
	if msg := extract115ErrorMsg(err.Error()); msg != "" {
		return fmt.Errorf("115返回错误: %s", msg)
	}
	return err
}

// extract115ErrorMsg 从115返回的JSON错误体中提取 error_msg 字段，无法解析时返回空串
func extract115ErrorMsg(raw string) string {
	m := offlineErrorMsgRegexp.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	msg := strings.ReplaceAll(m[1], `\"`, `"`)
	msg = strings.ReplaceAll(msg, `\\`, `\`)
	return strings.TrimSpace(msg)
}
