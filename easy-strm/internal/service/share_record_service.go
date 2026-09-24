package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	neturl "net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	shareRecordURLPattern  = regexp.MustCompile(`https?://(?:www\.)?(?:115cdn\.com|115\.com)/s/[A-Za-z0-9_]+[^\s\])>]*`)
	shareAccessCodePattern = regexp.MustCompile(`(?:访问码|提取码|密码)\s*[：:]\s*([A-Za-z0-9]+)`)
)

type ShareRecordService struct {
	enrichMu          sync.Mutex
	syncMu            sync.Mutex // 保护同步任务登记及清空操作；识别使用独立锁
	activeSyncID      string
	activeSyncKey     string
	taskSettingsStore ShareTaskSettingsStore
	identifyMu        sync.RWMutex // 清空与后台识别互斥，避免旧任务重新写入刚清空的内容
	dao               *dao.ShareRecordDAO
	tmdb              *TmdbService
	tasks             *TaskService
	parser            ShareRecordParser
	autoExport        func([]int) (string, bool, error)
}

// ShareRecordParser 获取分享中的文件列表。
type ShareRecordParser interface {
	ParseShareLink(context.Context, string, string) (*domain.ParseShareResponse, error)
}

func NewShareRecordService(d *dao.ShareRecordDAO, t *TmdbService, tasks *TaskService, parser ShareRecordParser) *ShareRecordService {
	return &ShareRecordService{dao: d, tmdb: t, tasks: tasks, parser: parser}
}

// SetAutoStrmExport 注入分享识别完成后的增量导出入口。
func (s *ShareRecordService) SetAutoStrmExport(export func([]int) (string, bool, error)) {
	s.autoExport = export
}
func (s *ShareRecordService) List(ctx context.Context, q domain.ShareRecordQuery) (domain.ShareRecordPage, error) {
	page, err := s.dao.List(ctx, q)
	for i := range page.Data {
		if !q.Summary {
			normalizeShareMaskedMedia(&page.Data[i])
		}
		normalizeShareGallery(&page.Data[i])
	}
	return page, err
}

// normalizeShareMaskedMedia 统一历史失败项和新扫描目录的展示状态，不调用识别接口。
func normalizeShareMaskedMedia(record *domain.ShareRecord) {
	record.MaskedCount = 0
	for i := range record.Media {
		media := &record.Media[i]
		if isMaskedSharePath(media.FileName) {
			media.Status = "masked"
			media.Result = nil
			media.Error = "目录名称已脱敏，已跳过识别"
			record.MaskedCount++
		}
	}
}

func (s *ShareRecordService) Create(ctx context.Context, r *domain.ShareRecord) error {
	if r.MediaType == "" {
		r.MediaType = "auto"
	}
	if r.MediaType != "auto" && r.MediaType != "movie" && r.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为自动、电影或电视剧")
	}
	parsedURL, parsedPassword, parsedName, err := parseShareRecordInput(r.URL)
	if err != nil {
		return err
	}
	r.URL = parsedURL
	if strings.TrimSpace(r.Name) == "" {
		r.Name = parsedName
	}
	if parsedPassword != "" {
		r.Password = parsedPassword
	}
	for i := range r.Media {
		if !isShareVideoFile(strings.TrimSpace(r.Media[i].FileName)) {
			return fmt.Errorf("分享文件必须是支持的视频文件")
		}
		if r.Media[i].MetadataSource == "" {
			r.Media[i].MetadataSource = domain.MetadataSourceAuto
		}
	}
	return s.dao.Create(ctx, r)
}

// parseShareRecordInput 从完整的115分享文案中提取规范链接和访问码。
func parseShareRecordInput(input string) (string, string, string, error) {
	rawURL := shareRecordURLPattern.FindString(strings.TrimSpace(input))
	if rawURL == "" {
		return "", "", "", fmt.Errorf("未识别到有效的115分享链接")
	}
	parsed, err := neturl.Parse(rawURL)
	if err != nil || shareCodeRe.FindStringSubmatch(rawURL) == nil {
		return "", "", "", fmt.Errorf("115分享链接格式不正确")
	}
	password := strings.TrimSpace(parsed.Query().Get("password"))
	if password == "" {
		if match := shareAccessCodePattern.FindStringSubmatch(input); len(match) > 1 {
			password = strings.TrimSpace(match[1])
		}
	}
	return rawURL, password, extractShareRecordName(input, rawURL), nil
}

func extractShareRecordName(input, rawURL string) string {
	start := strings.Index(input, rawURL)
	if start < 0 {
		return ""
	}
	rest := input[start+len(rawURL):]
	if strings.HasPrefix(rest, "](") {
		if end := strings.Index(rest, ")"); end >= 0 {
			rest = rest[end+1:]
		}
	}
	for _, marker := range []string{"访问码", "提取码", "密码", "复制这段"} {
		if index := strings.Index(rest, marker); index >= 0 {
			rest = rest[:index]
		}
	}
	// 仅去除文案外围的 Markdown/空白字符，名称本身可能合法包含括号。
	return strings.TrimSpace(strings.Trim(rest, "[] ：:\r\n"))
}
func (s *ShareRecordService) Update(ctx context.Context, r *domain.ShareRecord) error {
	if r.MediaType != "auto" && r.MediaType != "movie" && r.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为自动、电影或电视剧")
	}
	return s.dao.Update(ctx, r)
}
func (s *ShareRecordService) Delete(ctx context.Context, id int) error { return s.dao.Delete(ctx, id) }
func (s *ShareRecordService) AddMedia(ctx context.Context, m *domain.ShareMedia) error {
	if !isShareVideoFile(strings.TrimSpace(m.FileName)) {
		return fmt.Errorf("分享文件必须是支持的视频文件")
	}
	if m.MetadataSource == "" {
		m.MetadataSource = domain.MetadataSourceAuto
	}
	return s.dao.AddMedia(ctx, m)
}
func (s *ShareRecordService) DeleteMedia(ctx context.Context, id int) error {
	return s.dao.DeleteMedia(ctx, id)
}
func (s *ShareRecordService) Identify(ctx context.Context, m domain.ShareMedia, retry bool, forceRefresh ...bool) error {
	ctx = withShareWorkRound(ctx, len(forceRefresh) > 0 && forceRefresh[0])
	mediaType, err := s.dao.MediaType(ctx, m.ID)
	if err != nil {
		return err
	}
	m.MediaType = mediaType
	_, err = s.identifyOne(ctx, m, retry)
	return err
}

// ManualIdentify 将用户选择的搜索结果保存为分享媒体的识别结果。
func (s *ShareRecordService) ManualIdentify(ctx context.Context, m domain.ShareMedia) error {
	if isMaskedSharePath(m.FileName) {
		return fmt.Errorf("目录名称已脱敏，已跳过识别")
	}
	if m.ID <= 0 || m.Version <= 0 || m.Result == nil || (m.Result.MediaType != "movie" && m.Result.MediaType != "tv") || strings.TrimSpace(m.Result.Title) == "" || (m.Result.TmdbID == 0 && m.Result.MetadataID == "") {
		return fmt.Errorf("请选择有效的媒体搜索结果")
	}
	m.Result.Success = true
	m.Result.Message = "手动识别"
	m.Result.Filename = m.FileName
	if s.tmdb != nil {
		s.tmdb.EnsureIdentifyMetadata(m.Result)
	}
	episodes, err := normalizeShareEpisodes(m.Result.MediaType, m.Episodes, m.Result.SeasonNumber, m.Result.EpisodeNumber)
	if err != nil {
		return err
	}
	if err := s.dao.Identify(ctx, m, "identified", m.Result, "", episodes...); err != nil {
		return err
	}
	if s.tmdb != nil {
		s.tmdb.invalidateShareWork(ctx, m)
	}
	return nil
}

func (s *ShareRecordService) identifyOne(ctx context.Context, m domain.ShareMedia, retry bool) (bool, error) {
	if isMaskedSharePath(m.FileName) {
		return false, fmt.Errorf("目录名称已脱敏，已跳过识别")
	}
	if !retry && m.Status == "identified" && (m.MediaType == "" || m.MediaType == "auto" || (m.Result != nil && m.Result.MediaType == m.MediaType)) {
		return true, nil
	}
	if s.tmdb == nil {
		return false, fmt.Errorf("元数据识别服务未初始化")
	}
	ctx = context.WithValue(ctx, shareWorkScopeKey{}, m.ShareID)
	r, e := s.tmdb.IdentifyShareFile(ctx, m.FileName, m.MetadataSource, m.MediaType)
	if e != nil {
		logger.Errorf("[ShareIdentify] 元数据识别异常 | file=%q | source=%s | error=%v", m.FileName, m.MetadataSource, e)
		return false, s.dao.RecordIdentifyError(ctx, m, e.Error())
	}
	// 作品识别内部按作品补全详情；失败结果不能进入详情补全。
	if !r.Success {
		return false, s.dao.Identify(ctx, m, "failed", r, r.Message)
	}
	parsed := s.tmdb.ParseFilename(m.FileName)
	if parsed.Episode == 0 && r.EpisodeNumber > 0 {
		parsed.Season, parsed.Episode = r.SeasonNumber, r.EpisodeNumber
	}
	episodes, episodeErr := normalizeShareEpisodes(r.MediaType, nil, parsed.Season, parsed.Episode, parsed.Episodes...)
	if episodeErr != nil {
		return false, s.dao.Identify(ctx, m, "failed", r, episodeErr.Error())
	}
	if len(episodes) > 0 {
		r.SeasonNumber = episodes[0].SeasonNumber
		r.EpisodeNumber = episodes[0].EpisodeNumber
	}
	logger.Infof("[ShareIdentify] 元数据识别结果 | file=%q | success=%v | media_type=%s | tmdb_id=%d | title=%q | year=%d | episodes=%d", m.FileName, r.Success, r.MediaType, r.TmdbID, r.Title, r.Year, len(episodes))
	return true, s.dao.Identify(ctx, m, "identified", r, "", episodes...)
}

func normalizeShareEpisodes(mediaType string, explicit []domain.ShareEpisode, season, episode int, more ...int) ([]domain.ShareEpisode, error) {
	if mediaType == "movie" {
		return nil, nil
	}
	if mediaType != "tv" {
		return nil, fmt.Errorf("媒体类型无效")
	}
	items := append([]domain.ShareEpisode(nil), explicit...)
	if len(items) == 0 {
		numbers := more
		if len(numbers) == 0 && episode > 0 {
			numbers = []int{episode}
		}
		for _, number := range numbers {
			items = append(items, domain.ShareEpisode{SeasonNumber: season, EpisodeNumber: number})
		}
	}
	seen := make(map[domain.ShareEpisode]bool, len(items))
	result := make([]domain.ShareEpisode, 0, len(items))
	for _, item := range items {
		if item.SeasonNumber < 0 || item.EpisodeNumber <= 0 {
			return nil, fmt.Errorf("季集信息无效")
		}
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("缺少季集信息")
	}
	return result, nil
}

// SyncShareFiles 完整解析一个分享，并在成功遍历后原子刷新文件可用状态。
func (s *ShareRecordService) SyncShareFiles(ctx context.Context, recordID int) (int, int, error) {
	if s.parser == nil {
		return 0, 0, fmt.Errorf("分享解析服务未初始化")
	}
	page, err := s.dao.List(ctx, domain.ShareRecordQuery{ShareID: recordID, Page: 1, PageSize: 1})
	if err != nil {
		return 0, 0, err
	}
	if len(page.Data) != 1 {
		return 0, 0, fmt.Errorf("分享不存在")
	}
	parsed, err := s.parseRecordShare(ctx, page.Data[0])
	if err != nil {
		return 0, 0, err
	}
	selected := selectShareMediaFiles(parsed.Files)
	files := make([]domain.ShareMedia, 0, len(selected))
	for _, file := range selected {
		name := file.Path
		if strings.TrimSpace(name) == "" {
			name = file.Name
		}
		files = append(files, domain.ShareMedia{ShareID: recordID, RemoteFileID: file.Fid, FileName: name, FileSize: file.Size, SHA1: file.Sha1, PickCode: file.PickCode, MetadataSource: domain.MetadataSourceAuto, Available: true})
	}
	token := fmt.Sprintf("share-%d-%d", recordID, time.Now().UnixNano())
	count, err := s.dao.SyncFiles(ctx, recordID, token, files)
	return count, len(parsed.MaskedDirectories), err
}

// StartRecordSync 创建单分享文件同步任务；识别任务不会隐式调用本流程。
func (s *ShareRecordService) StartRecordSync(ctx context.Context, recordID int) (string, error) {
	return s.StartBatchSync(ctx, []int{recordID})
}

// StartBatchSync 创建选中分享的串行文件同步任务，避免并发访问同一分享账号。
func (s *ShareRecordService) StartBatchSync(ctx context.Context, recordIDs []int) (string, error) {
	if len(recordIDs) == 0 {
		return "", fmt.Errorf("请至少选择一个分享")
	}
	if s.tasks == nil {
		return "", fmt.Errorf("任务服务未初始化")
	}
	// 规范化集合，让单条、批量、乱序及重复 ID 的同一请求复用活动任务。
	recordIDs = append([]int(nil), recordIDs...)
	sort.Ints(recordIDs)
	unique := recordIDs[:0]
	for _, id := range recordIDs {
		if id <= 0 {
			return "", fmt.Errorf("分享ID无效")
		}
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	recordIDs = unique
	key := fmt.Sprint(recordIDs)
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.activeSyncID != "" {
		if s.activeSyncKey == key {
			return s.activeSyncID, nil
		}
		return "", fmt.Errorf("已有分享同步任务运行，请等待结束后再发起")
	}
	settings, err := s.GetTaskSettings()
	if err != nil {
		return "", err
	}
	taskID := fmt.Sprintf("share_sync_%d", time.Now().UnixNano())
	title := "批量同步分享文件"
	if len(recordIDs) == 1 {
		title = "同步分享文件"
	}
	if err := s.tasks.Create(taskID, "share_sync", title); err != nil {
		return "", err
	}
	s.activeSyncID, s.activeSyncKey = taskID, key
	go func() {
		defer func() {
			s.syncMu.Lock()
			s.activeSyncID, s.activeSyncKey = "", ""
			s.syncMu.Unlock()
		}()
		taskCtx, cancel := newShareTaskContext(context.Background(), settings.TimeoutMinutes)
		defer cancel()
		defer s.tasks.RemoveCancel(taskID)
		s.tasks.RegisterCancel(taskID, cancel)
		_ = s.tasks.UpdateStatus(taskID, "running")
		success, failed, files := 0, 0, 0
		syncedRecordIDs := make([]int, 0, len(recordIDs))
		for index, recordID := range recordIDs {
			if taskCtx.Err() != nil {
				s.finishShareTaskContext(taskID, taskCtx.Err(), settings.TimeoutMinutes)
				return
			}
			if s.tasks.IsCancelled(taskID) {
				return
			}
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "同步分享文件", "current_share_id": recordID, "current_index": index + 1, "total_shares": len(recordIDs)})
			count, masked, syncErr := s.SyncShareFiles(taskCtx, recordID)
			if len(recordIDs) == 1 {
				_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"share_id": recordID, "file_count": count, "masked_directories": masked})
			}
			if taskCtx.Err() != nil {
				s.finishShareTaskContext(taskID, taskCtx.Err(), settings.TimeoutMinutes)
				return
			}
			if syncErr != nil {
				logger.Errorf("[ShareSync] task=%s share=%d error=%v", taskID, recordID, syncErr)
				if len(recordIDs) == 1 {
					_ = s.tasks.SetError(taskID, syncErr.Error())
					return
				}
				failed++
			} else {
				success++
				files += count
				syncedRecordIDs = append(syncedRecordIDs, recordID)
			}
			_ = s.tasks.UpdateProgress(taskID, len(recordIDs), index+1, success, failed)
		}
		_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "同步完成", "total_shares": len(recordIDs), "synced_shares": success, "synced_share_ids": syncedRecordIDs, "failed_shares": failed, "file_count": files})
		if failed > 0 {
			_ = s.tasks.SetError(taskID, fmt.Sprintf("同步完成，但有 %d 个分享失败", failed))
			return
		}
		_ = s.tasks.UpdateStatus(taskID, "completed")
	}()
	return taskID, nil
}

// StartBatchIdentify 创建后台识别任务并立即返回任务 ID。
func (s *ShareRecordService) StartBatchIdentify(ctx context.Context, ids []int, retry bool, recordIDs []int, pendingOnly ...bool) (string, error) {
	return s.startIdentifyTask(ctx, ids, retry, recordIDs, pendingOnly...)
}

// StartRecordIdentify 为单条分享创建识别任务，并只处理该分享下的全部媒体。
func (s *ShareRecordService) StartRecordIdentify(ctx context.Context, recordID int, pendingOnly ...bool) (string, error) {
	onlyPending := len(pendingOnly) > 0 && pendingOnly[0]
	onlyFailed := len(pendingOnly) > 1 && pendingOnly[1]
	if onlyPending && onlyFailed {
		return "", fmt.Errorf("待识别与失败筛选不能同时启用")
	}
	forceRefresh := len(pendingOnly) > 2 && pendingOnly[2]
	return s.startIdentifyTask(ctx, nil, onlyFailed, []int{recordID}, onlyPending, onlyFailed, forceRefresh)
}

func (s *ShareRecordService) startIdentifyTask(ctx context.Context, ids []int, retry bool, recordIDs []int, pendingOnly ...bool) (string, error) {
	if s.tasks == nil {
		return "", fmt.Errorf("任务服务未初始化")
	}
	if !s.identifyMu.TryLock() {
		return "", fmt.Errorf("已有分享识别任务运行，请等待结束后再发起")
	}
	taskID := fmt.Sprintf("share_identify_%d", time.Now().UnixNano())
	if err := s.tasks.Create(taskID, "share_identify", "分享媒体批量识别"); err != nil {
		s.identifyMu.Unlock()
		return "", err
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "准备媒体列表", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "running"}, {"name": "识别媒体", "status": "pending"}, {"name": "汇总结果", "status": "pending"}}})
	go func() {
		defer s.identifyMu.Unlock()
		s.runBatchIdentify(ctx, taskID, ids, retry, recordIDs, pendingOnly...)
	}()
	return taskID, nil
}

func (s *ShareRecordService) runBatchIdentify(ctx context.Context, taskID string, ids []int, retry bool, recordIDs []int, pendingOnly ...bool) {
	onlyPending := len(pendingOnly) > 0 && pendingOnly[0]
	onlyFailed := len(pendingOnly) > 1 && pendingOnly[1]
	settings, settingsErr := s.GetTaskSettings()
	if settingsErr != nil {
		_ = s.tasks.SetError(taskID, settingsErr.Error())
		return
	}
	ctx, cancel := newShareTaskContext(ctx, settings.TimeoutMinutes)
	ctx = withShareWorkRound(ctx, len(pendingOnly) > 2 && pendingOnly[2])
	defer cancel()
	defer s.tasks.RemoveCancel(taskID)
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("分享识别任务异常中断: %v", recovered)
			logger.Errorf("ShareRecordService[runBatchIdentify] task=%s panic=%v", taskID, recovered)
			_ = s.tasks.SetError(taskID, message)
		}
	}()
	s.tasks.RegisterCancel(taskID, cancel)
	_ = s.tasks.UpdateStatus(taskID, "running")
	logger.Infof("ShareRecordService[runBatchIdentify] task=%s start records=%v media=%v retry=%v", taskID, recordIDs, ids, retry)
	page, err := s.List(ctx, domain.ShareRecordQuery{Page: 1, PageSize: 200})
	if err != nil {
		_ = s.tasks.SetError(taskID, err.Error())
		return
	}
	// 识别阶段只读取已经同步落库的文件，绝不隐式解析分享链接。
	for number := 2; len(page.Data) < page.Total; number++ {
		next, loadErr := s.List(ctx, domain.ShareRecordQuery{Page: number, PageSize: 200})
		if loadErr != nil {
			_ = s.tasks.SetError(taskID, loadErr.Error())
			return
		}
		if len(next.Data) == 0 {
			break
		}
		page.Data = append(page.Data, next.Data...)
	}
	cancelledShares := make(map[int]bool)
	prepareShareEpisodeInputs(ctx, page.Data)
	if s.tmdb != nil {
		s.tmdb.seedShareWorks(ctx, page.Data)
	}
	items := make([]domain.ShareMedia, 0)
	maskedTotal := 0
	for _, record := range page.Data {
		if len(recordIDs) > 0 && !contains(recordIDs, record.ID) {
			continue
		}
		if record.ShareCancelled || cancelledShares[record.ID] {
			cancelledShares[record.ID] = true
			continue
		}
		for _, media := range record.Media {
			media.ShareID = record.ID
			if !media.Available || media.Status == "ignored" {
				continue
			}
			if isMaskedSharePath(media.FileName) {
				maskedTotal++
				logger.Infof("[ShareIdentify] 跳过已有脱敏媒体 | task=%s | directory=%q", taskID, media.FileName)
				continue
			}
			if (len(ids) == 0 || contains(ids, media.ID)) && (bypassShareRecognitionCache(ctx) || shouldIdentifyShareMedia(media, record.MediaType, retry, onlyPending, onlyFailed)) {
				media.MediaType = record.MediaType
				items = append(items, media)
			}
		}
	}
	_ = s.tasks.UpdateProgress(taskID, len(items), 0, 0, 0)
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "worker_count": settings.WorkerCount, "cancelled_shares": len(cancelledShares), "masked": maskedTotal})
	if len(items) == 0 {
		// 没有待识别媒体时任务仍是正常完成，进度应显示100%，避免出现“完成但0%”的误导状态。
		_ = s.tasks.UpdateProgressPercent(taskID, 100)
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "识别媒体", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "running"}, {"name": "汇总结果", "status": "pending"}}})
	success, failed := 0, 0
	identifiedFileIDs := make([]int, 0, len(items))
	for index, media := range items {
		if err := ctx.Err(); err != nil {
			s.finishShareTaskContext(taskID, err, settings.TimeoutMinutes)
			return
		}
		if s.tasks.IsCancelled(taskID) {
			return
		}
		ok, identifyErr := s.identifyOne(ctx, media, bypassShareRecognitionCache(ctx))
		if ctx.Err() != nil {
			s.finishShareTaskContext(taskID, ctx.Err(), settings.TimeoutMinutes)
			return
		}
		if identifyErr != nil || !ok {
			failed++
		} else {
			success++
			identifiedFileIDs = append(identifiedFileIDs, media.ID)
		}
		_ = s.tasks.UpdateProgress(taskID, len(items), index+1, success, failed)
		_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "识别媒体", "current_file": media.FileName, "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "running", "message": fmt.Sprintf("正在处理 %d/%d", index+1, len(items))}, {"name": "汇总结果", "status": "pending"}}})
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "汇总结果", "current_file": "", "success": success, "failed": failed, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "completed"}, {"name": "汇总结果", "status": "running"}}})
	if failed > 0 {
		_ = s.tasks.SetError(taskID, fmt.Sprintf("识别完成，但有 %d 项失败", failed))
		return
	}
	if s.autoExport != nil && len(identifiedFileIDs) > 0 {
		exportID, started, exportErr := s.autoExport(identifiedFileIDs)
		if exportErr != nil {
			logger.Warnf("ShareRecordService[runBatchIdentify] task=%s 自动导出STRM失败: %v", taskID, exportErr)
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"strm_export_error": exportErr.Error()})
		} else if started {
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"strm_export_task_id": exportID, "strm_export_file_count": len(identifiedFileIDs)})
		}
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "汇总结果", "current_file": "", "success": success, "failed": failed, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "completed"}, {"name": "汇总结果", "status": "completed"}}})
	_ = s.tasks.UpdateStatus(taskID, "completed")
	logger.Infof("ShareRecordService[runBatchIdentify] task=%s completed success=%d failed=%d", taskID, success, failed)
}
func (s *ShareRecordService) BatchIdentify(ctx context.Context, ids []int, retry bool) (domain.ShareIdentifySummary, error) {
	ctx = withShareWorkRound(ctx, false)
	p, e := s.List(ctx, domain.ShareRecordQuery{Page: 1, PageSize: 200})
	if e != nil {
		return domain.ShareIdentifySummary{}, e
	}
	sum := domain.ShareIdentifySummary{}
	prepareShareEpisodeInputs(ctx, p.Data)
	if s.tmdb != nil {
		s.tmdb.seedShareWorks(ctx, p.Data)
	}
	for _, r := range p.Data {
		for _, m := range r.Media {
			m.ShareID = r.ID
			if len(ids) > 0 && !contains(ids, m.ID) {
				continue
			}
			if isMaskedSharePath(m.FileName) || (!retry && m.Status == "identified") {
				sum.Skipped++
				continue
			}
			if e = s.Identify(ctx, m, retry); e != nil {
				sum.Failed++
			} else {
				sum.Success++
			}
		}
	}
	return sum, nil
}
func contains(a []int, v int) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

// ListMedia 获取已识别媒体分页，保留完整任务扫描的原有读取入口。
func (s *ShareRecordService) ListMedia(ctx context.Context, id, page, size int, duplicates bool) (domain.ShareMediaPage, error) {
	return s.dao.ListMedia(ctx, id, page, size, duplicates)
}

// ListFiles 获取分享文件分页。
func (s *ShareRecordService) ListFiles(ctx context.Context, id, page, size int) (domain.ShareMediaPage, error) {
	return s.dao.ListFiles(ctx, id, page, size)
}

// shouldIdentifyShareMedia 继续任务仅处理未尝试候选，不重试失败或覆盖已识别记录。
func shouldIdentifyShareMedia(media domain.ShareMedia, mediaType string, retry, pendingOnly bool, failedOnly ...bool) bool {
	if retry || (len(failedOnly) > 0 && failedOnly[0]) {
		return media.Status == "failed"
	}
	if pendingOnly {
		return media.Status == "pending" || media.Status == ""
	}
	return media.Status != "identified" || media.Result == nil || (mediaType != "auto" && media.Result.MediaType != mediaType)
}
