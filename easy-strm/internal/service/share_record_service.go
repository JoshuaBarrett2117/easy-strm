package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"errors"
	"fmt"
	neturl "net/url"
	"path"
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
	coordOnce         sync.Once
	coordinator       *ShareOperationCoordinator
	operations        *shareOperationQueue
	taskSettingsStore ShareTaskSettingsStore
	dao               *dao.ShareRecordDAO
	tmdb              *TmdbService
	tasks             *TaskService
	parser            ShareRecordParser
	autoExport        func([]int) (string, bool, error)
	autoExportContext func(context.Context, []int) (string, bool, error)
}

// ShareRecordParser 获取分享中的文件列表。
type ShareRecordParser interface {
	ParseShareLink(context.Context, string, string) (*domain.ParseShareResponse, error)
}

func NewShareRecordService(d *dao.ShareRecordDAO, t *TmdbService, tasks *TaskService, parser ShareRecordParser) *ShareRecordService {
	return &ShareRecordService{dao: d, tmdb: t, tasks: tasks, parser: parser, coordinator: NewShareOperationCoordinator()}
}

// SetAutoStrmExport 注入分享识别完成后的增量导出入口。
func (s *ShareRecordService) SetAutoStrmExport(export func([]int) (string, bool, error)) {
	s.autoExport = export
}

// SetAutoStrmExportContext 注入保留请求与任务上下文的自动导出入口。
func (s *ShareRecordService) SetAutoStrmExportContext(export func(context.Context, []int) (string, bool, error)) {
	s.autoExportContext = export
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
	if s.Coordinator().pendingShare(r.ID) {
		return fmt.Errorf("该分享正在等待或执行清理，请在任务中心查看")
	}
	release, err := s.Coordinator().acquire(ctx, nil, shareResource{key: shareKey(r.ID)}, shareResource{key: fmt.Sprintf("share-config:%d", r.ID), exclusive: true})
	if err != nil {
		return err
	}
	defer release()

	if r.MediaType != "auto" && r.MediaType != "movie" && r.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为自动、电影或电视剧")
	}
	return s.dao.Update(ctx, r)
}
func (s *ShareRecordService) AddMedia(ctx context.Context, m *domain.ShareMedia) error {
	if s.Coordinator().pendingShare(m.ShareID) {
		return fmt.Errorf("该分享正在等待或执行清理")
	}
	release, err := s.Coordinator().acquire(ctx, nil, shareResource{key: shareKey(m.ShareID)})
	if err != nil {
		return err
	}
	defer release()

	if !isShareVideoFile(strings.TrimSpace(m.FileName)) {
		return fmt.Errorf("分享文件必须是支持的视频文件")
	}
	if m.MetadataSource == "" {
		m.MetadataSource = domain.MetadataSourceAuto
	}
	return s.dao.AddMedia(ctx, m)
}
func (s *ShareRecordService) DeleteMedia(ctx context.Context, id int) error {
	file, err := s.dao.FileIdentity(ctx, id)
	if err != nil {
		return err
	}
	release, err := s.Coordinator().acquire(ctx, nil, shareResource{key: shareKey(file.ShareID)}, shareResource{key: fileKey(id), exclusive: true}, shareResource{key: fmt.Sprintf("sync-commit:%d", file.ShareID)})
	if err != nil {
		return err
	}
	defer release()
	file, err = s.dao.FileIdentity(ctx, id)
	if err != nil {
		return err
	}
	if err = s.dao.DeleteMedia(ctx, id); err == nil {
		s.Coordinator().removedFile(file)
	}
	return err
}
func (s *ShareRecordService) Identify(ctx context.Context, m domain.ShareMedia, retry bool) error {
	ctx = withShareWorkRound(ctx, retry)
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
	_, err := s.ManualIdentifyWithCount(ctx, m)
	return err
}

// ManualIdentifyWithCount 保存用户选择；明确勾选时将同目录且季集可解析的待核对文件一并关联。
func (s *ShareRecordService) ManualIdentifyWithCount(ctx context.Context, m domain.ShareMedia) (int, error) {
	if isMaskedSharePath(m.FileName) {
		return 0, fmt.Errorf("目录名称已脱敏，已跳过识别")
	}
	if m.ID <= 0 || m.Version <= 0 || m.Result == nil || (m.Result.MediaType != "movie" && m.Result.MediaType != "tv") || strings.TrimSpace(m.Result.Title) == "" || (m.Result.TmdbID == 0 && m.Result.MetadataID == "") {
		return 0, fmt.Errorf("请选择有效的媒体搜索结果")
	}
	m.Result.Success = true
	m.Result.Message = "手动识别"
	m.Result.Filename = m.FileName
	if s.tmdb != nil {
		s.tmdb.EnsureIdentifyMetadata(m.Result)
	}
	episodes, err := normalizeShareEpisodes(m.Result.MediaType, m.Episodes, m.Result.SeasonNumber, m.Result.EpisodeNumber)
	if err != nil {
		return 0, err
	}
	if !m.ApplyToSeries || m.Result.MediaType != "tv" {
		if m.ShareID <= 0 {
			parent, err := s.dao.FileShareID(ctx, m.ID)
			if err != nil {
				return 0, err
			}
			m.ShareID = parent
		}
		release, err := s.Coordinator().acquire(ctx, nil, shareResource{key: shareKey(m.ShareID)}, shareResource{key: fileKey(m.ID), exclusive: true})
		if err != nil {
			return 0, err
		}
		defer release()
		fresh, err := s.dao.FileCandidate(ctx, m.ID)
		if err != nil {
			return 0, err
		}
		if fresh.ShareID != m.ShareID || fresh.Version != m.Version {
			return 0, fmt.Errorf("媒体已更新或删除，请刷新后重试")
		}
		m.FileName = fresh.FileName
		m.Result.Filename = fresh.FileName
		if err := s.dao.Identify(ctx, m, "identified", m.Result, "", episodes...); err != nil {
			return 0, err
		}
	} else {
		if s.tmdb == nil || m.ShareID <= 0 {
			return 0, fmt.Errorf("缺少剧集批量核对信息")
		}
		records, err := s.dao.ListManualBatchContext(ctx, m.ShareID)
		if err != nil {
			return 0, err
		}
		batch, err := s.buildManualSeriesBatch(m, episodes, records)
		if err != nil {
			return 0, err
		}
		resources := []shareResource{{key: shareKey(m.ShareID)}}
		for _, item := range batch {
			resources = append(resources, shareResource{key: fileKey(item.ID), exclusive: true})
		}
		release, err := s.Coordinator().acquire(ctx, nil, resources...)
		if err != nil {
			return 0, err
		}
		defer release()
		// 固定初次确认的候选集合，等待后读取版本；新出现的文件留给下一次核对。
		records, err = s.dao.ListManualBatchContext(ctx, m.ShareID)
		if err != nil {
			return 0, err
		}
		current := map[int]domain.ShareMedia{}
		for _, record := range records {
			for _, item := range record.Media {
				current[item.ID] = item
			}
		}
		for i := range batch {
			fresh, ok := current[batch[i].ID]
			if !ok || fresh.Version != batch[i].Version {
				return 0, fmt.Errorf("媒体已更新或删除，请刷新后重试")
			}
			batch[i].FileName = fresh.FileName
		}
		if err := s.dao.IdentifyBatch(ctx, batch); err != nil {
			return 0, err
		}
		if s.tmdb != nil {
			for _, item := range batch {
				s.tmdb.invalidateShareWork(ctx, item)
			}
		}
		return len(batch), nil
	}
	if s.tmdb != nil {
		s.tmdb.invalidateShareWork(ctx, m)
	}
	return 1, nil
}

func (s *ShareRecordService) buildManualSeriesBatch(target domain.ShareMedia, episodes []domain.ShareEpisode, records []domain.ShareRecord) ([]domain.ShareMedia, error) {
	parent := path.Dir(strings.ReplaceAll(target.FileName, "\\", "/"))
	batch := []domain.ShareMedia{}
	found := false
	for _, record := range records {
		if record.ID != target.ShareID {
			continue
		}
		for _, item := range record.Media {
			if item.ID == target.ID {
				if item.Version != target.Version || item.FileName != target.FileName {
					return nil, fmt.Errorf("媒体已更新，请刷新后重试")
				}
				found = true
				item.Result = target.Result
				item.Episodes = episodes
				batch = append(batch, item)
				continue
			}
			if path.Dir(strings.ReplaceAll(item.FileName, "\\", "/")) != parent || isMaskedSharePath(item.FileName) {
				continue
			}
			input := shareEpisodeInput(context.Background(), item.FileName, "tv")
			query := s.tmdb.analyzeShareQuery(input, "tv")
			candidate := domain.TmdbSearchResult{Title: target.Result.Title, OriginalTitle: target.Result.OriginalTitle, Year: target.Result.Year, MediaType: "tv"}
			parsed := s.tmdb.parseFilenameForMediaType(input, "tv")
			if parsed.Episode <= 0 || (canonicalShareTitle(parsed.Title) != canonicalShareTitle(target.Result.Title) && canonicalShareTitle(parsed.Title) != canonicalShareTitle(target.Result.OriginalTitle)) || selectVerifiedShareCandidate(query, []domain.TmdbSearchResult{candidate}) == nil {
				continue
			}
			if len(episodes) == 0 || parsed.Season != episodes[0].SeasonNumber {
				continue
			}
			item.Result = cloneShareWorkResult(target.Result)
			item.Result.Filename = item.FileName
			item.Result.SeasonNumber, item.Result.EpisodeNumber = parsed.Season, parsed.Episode
			item.Result.Message = "同剧手动批量核对"
			item.Episodes = []domain.ShareEpisode{{SeasonNumber: parsed.Season, EpisodeNumber: parsed.Episode}}
			batch = append(batch, item)
		}
	}
	if !found {
		return nil, fmt.Errorf("目标文件已更新或不在待核对队列，请刷新后重试")
	}
	return batch, nil
}

func (s *ShareRecordService) identifyOne(ctx context.Context, m domain.ShareMedia, retry bool) (bool, error) {
	if isMaskedSharePath(m.FileName) {
		return false, fmt.Errorf("目录名称已脱敏，已跳过识别")
	}
	if m.ID <= 0 {
		return false, fmt.Errorf("文件ID无效")
	}

	if m.ShareID <= 0 {
		parent, err := s.dao.FileShareID(ctx, m.ID)
		if err != nil {
			return false, err
		}
		m.ShareID = parent
	}
	settings, err := s.unitSettings(ctx)
	if err != nil {
		return false, err
	}
	release, err := s.Coordinator().acquire(ctx, []int{m.ShareID}, shareResource{key: shareKey(m.ShareID)}, shareResource{key: fileKey(m.ID), exclusive: true}, shareResource{key: "budget:identify", limit: settings.WorkerCount})
	if err != nil {
		return false, err
	}
	defer release()
	if _, background := ctx.Value(shareEpochContext{}).(map[int]uint64); background {
		records, err := s.dao.ListIdentifyMedia(ctx, dao.ShareIdentifyFilter{RecordIDs: []int{m.ShareID}, MediaIDs: []int{m.ID}, ForceRefresh: true})
		if err != nil {
			return false, err
		}
		if len(records) == 0 || len(records[0].Media) == 0 {
			return false, ErrShareUnitSkipped
		}
		m = records[0].Media[0]
		m.ShareID = records[0].ID
		m.MediaType = records[0].MediaType
	} else {
		fresh, err := s.dao.FileCandidate(ctx, m.ID)
		if err != nil {
			return false, err
		}
		if fresh.ShareID != m.ShareID || m.Version > 0 && fresh.Version != m.Version {
			return false, fmt.Errorf("媒体已更新或删除，请刷新后重试")
		}
		source := m.MetadataSource
		m = fresh
		if source != "" {
			m.MetadataSource = source
		}
	}

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
	if r.EpisodeNumber > 0 {
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
	logger.Debugf("[ShareIdentify] 元数据识别结果 | file=%q | success=%v | media_type=%s | tmdb_id=%d | title=%q | year=%d | episodes=%d", m.FileName, r.Success, r.MediaType, r.TmdbID, r.Title, r.Year, len(episodes))
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
	if _, ok := ctx.Value(shareSubmissionContext{}).(uint64); !ok {
		ctx = s.Coordinator().batchContext(ctx, ownerOf(ctx))
	}
	settings, err := s.unitSettings(ctx)
	if err != nil {
		return 0, 0, err
	}
	release, err := s.Coordinator().acquire(ctx, []int{recordID}, shareResource{key: shareKey(recordID)}, shareResource{key: fmt.Sprintf("sync:%d", recordID), exclusive: true}, shareResource{key: "budget:sync", limit: settings.SyncWorkers})
	if err != nil {
		return 0, 0, err
	}
	defer release()

	if s.parser == nil {
		return 0, 0, fmt.Errorf("分享解析服务未初始化")
	}
	page, err := s.dao.List(ctx, domain.ShareRecordQuery{ShareID: recordID, Page: 1, PageSize: 1})
	if err != nil {
		return 0, 0, err
	}
	if len(page.Data) != 1 {
		return 0, 0, sql.ErrNoRows
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
	ids, err := s.dao.SyncResourceIDs(ctx, recordID)
	if err != nil {
		return 0, 0, err
	}
	resources := []shareResource{{key: fmt.Sprintf("sync-commit:%d", recordID), exclusive: true}}
	for _, id := range ids {
		resources = append(resources, shareResource{key: fileKey(id), exclusive: true})
	}
	commitRelease, err := s.Coordinator().acquire(ctx, nil, resources...)
	if err != nil {
		return 0, 0, err
	}
	defer commitRelease()
	files = s.Coordinator().filterSyncFiles(ctx, recordID, files)
	count, err := s.dao.SyncFiles(ctx, recordID, token, files, page.Data[0].Version)
	return count, len(parsed.MaskedDirectories), err
}

// StartRecordSync 创建单分享文件同步任务；识别任务不会隐式调用本流程。
func (s *ShareRecordService) StartRecordSync(ctx context.Context, recordID int) (string, error) {
	return s.StartBatchSync(ctx, []int{recordID})
}

// StartBatchSync 创建分享文件同步任务，不同分享共用并发预算，同一分享串行扫描。
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
	key := "sync:" + fmt.Sprint(recordIDs)
	settings, err := s.GetTaskSettings()
	if err != nil {
		return "", err
	}
	taskID := fmt.Sprintf("share_sync_%d", time.Now().UnixNano())
	title := "批量同步分享文件"
	if len(recordIDs) == 1 {
		title = "同步分享文件"
	}
	id, created, err := s.Coordinator().registerRequest(ctx, key, taskID, func() error { return s.tasks.Create(taskID, "share_sync", title) })
	if err != nil {
		return "", err
	}
	if !created {
		return id, nil
	}
	batchCtx := context.WithValue(s.Coordinator().batchContext(logger.WithTaskID(context.WithoutCancel(ctx), taskID), taskID), shareTaskSettingsContext{}, settings)
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"record_ids": recordIDs, "phase": "准备同步"})
	go func() {
		defer s.Coordinator().finishRequest(key, taskID)
		taskCtx, cancel := newShareTaskContext(batchCtx, settings.TimeoutMinutes)
		defer cancel()
		defer s.tasks.RemoveCancel(taskID)
		s.tasks.RegisterCancel(taskID, cancel)
		_ = s.tasks.UpdateStatus(taskID, "running")
		success, failed, files, skipped := 0, 0, 0, 0
		syncedRecordIDs := make([]int, 0, len(recordIDs))
		processedRecordIDs := make([]int, 0, len(recordIDs))
		type syncResult struct {
			id, count, masked int
			err               error
		}
		jobs := make(chan int)
		results := make(chan syncResult, len(recordIDs))
		var wg sync.WaitGroup
		workers := settings.SyncWorkers
		if workers > len(recordIDs) {
			workers = len(recordIDs)
		}
		if workers < 1 {
			workers = 1
		}
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range jobs {
					if taskCtx.Err() != nil || s.tasks.IsCancelled(taskID) {
						continue
					}
					count, masked, err := s.SyncShareFiles(taskCtx, id)
					results <- syncResult{id: id, count: count, masked: masked, err: err}
				}
			}()
		}
		go func() {
			defer close(results)
			defer wg.Wait()
			for _, id := range recordIDs {
				select {
				case jobs <- id:
				case <-taskCtx.Done():
					close(jobs)
					return
				}
			}
			close(jobs)
		}()
		processed := 0
		for result := range results {
			processed++
			recordID := result.id
			processedRecordIDs = append(processedRecordIDs, recordID)
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "同步分享文件", "current_share_id": recordID, "current_index": processed, "total_shares": len(recordIDs), "worker_count": workers})
			if len(recordIDs) == 1 {
				_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"share_id": recordID, "file_count": result.count, "masked_directories": result.masked})
			}
			if errors.Is(result.err, ErrShareUnitSkipped) || errors.Is(result.err, sql.ErrNoRows) {
				skipped++
			} else if result.err != nil {
				logger.Errorf("[ShareSync] task=%s share=%d error=%v", taskID, recordID, result.err)
				if len(recordIDs) == 1 {
					_ = s.tasks.SetError(taskID, result.err.Error())
					return
				}
				failed++
			} else {
				success++
				files += result.count
				syncedRecordIDs = append(syncedRecordIDs, recordID)
			}
			_ = s.tasks.UpdateProgress(taskID, len(recordIDs), processed, success, failed)
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "同步分享文件", "current_share_id": recordID, "current_index": processed, "total_shares": len(recordIDs), "worker_count": workers, "record_ids": recordIDs, "processed_share_ids": processedRecordIDs, "synced_share_ids": syncedRecordIDs})
		}
		if taskCtx.Err() != nil {
			s.finishShareTaskContext(taskID, taskCtx.Err(), settings.TimeoutMinutes)
			return
		}
		sort.Ints(syncedRecordIDs)
		_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "同步完成", "total_shares": len(recordIDs), "synced_shares": success, "synced_share_ids": syncedRecordIDs, "failed_shares": failed, "skipped": skipped, "file_count": files})
		if failed > 0 {
			_ = s.tasks.SetError(taskID, fmt.Sprintf("同步完成，但有 %d 个分享失败", failed))
			return
		}
		_ = s.tasks.UpdateStatus(taskID, "completed")
	}()
	return taskID, nil
}

// RetryBatchSyncTask 从任务元数据恢复分享同步。服务重启后原协程已不存在，因此重新创建一个可追踪任务执行。
func (s *ShareRecordService) RetryBatchSyncTask(taskID string, task map[string]interface{}) (string, error) {
	metadata, _ := task["metadata"].(map[string]interface{})
	if metadata == nil {
		return "", fmt.Errorf("任务缺少分享同步参数")
	}
	idsRaw, ok := metadata["record_ids"].([]interface{})
	if !ok {
		if ids, ok2 := metadata["record_ids"].([]int); ok2 {
			return s.StartBatchSync(context.Background(), ids)
		}
		return "", fmt.Errorf("任务缺少分享ID，无法继续")
	}
	ids := make([]int, 0, len(idsRaw))
	for _, value := range idsRaw {
		id, ok := value.(float64)
		if !ok || int(id) <= 0 {
			return "", fmt.Errorf("任务分享ID无效")
		}
		ids = append(ids, int(id))
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("任务没有可继续的分享")
	}
	processed := make(map[int]struct{})
	if raw, ok := metadata["processed_share_ids"].([]interface{}); ok {
		for _, value := range raw {
			if id, valid := value.(float64); valid {
				processed[int(id)] = struct{}{}
			}
		}
	} else if raw, ok := metadata["processed_share_ids"].([]int); ok {
		for _, id := range raw {
			processed[id] = struct{}{}
		}
	}
	remaining := ids[:0]
	for _, id := range ids {
		if _, done := processed[id]; !done {
			remaining = append(remaining, id)
		}
	}
	if len(remaining) == 0 {
		return "", fmt.Errorf("任务中的分享均已处理，无需继续")
	}
	return s.StartBatchSync(context.Background(), remaining)
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
	return s.startIdentifyTask(ctx, nil, !onlyPending, []int{recordID}, onlyPending, onlyFailed)
}

func (s *ShareRecordService) startIdentifyTask(ctx context.Context, ids []int, retry bool, recordIDs []int, pendingOnly ...bool) (string, error) {
	if s.tasks == nil {
		return "", fmt.Errorf("任务服务未初始化")
	}
	settings, err := s.GetTaskSettings()
	if err != nil {
		return "", err
	}
	ids, recordIDs = sortedShareIDs(ids), sortedShareIDs(recordIDs)
	onlyPending := len(pendingOnly) > 0 && pendingOnly[0]
	onlyFailed := len(pendingOnly) > 1 && pendingOnly[1]
	key := fmt.Sprintf("identify:%v:%v:%t:%t:%t", ids, recordIDs, retry, onlyPending, onlyFailed)
	taskID := fmt.Sprintf("share_identify_%d", time.Now().UnixNano())
	id, created, err := s.Coordinator().registerRequest(ctx, key, taskID, func() error { return s.tasks.Create(taskID, "share_identify", "分享媒体批量识别") })
	if err != nil {
		return "", err
	}
	if !created {
		return id, nil
	}
	batchCtx := context.WithValue(s.Coordinator().batchContext(logger.WithTaskID(context.WithoutCancel(ctx), taskID), taskID), shareTaskSettingsContext{}, settings)
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "准备媒体列表", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "running"}, {"name": "识别媒体", "status": "pending"}, {"name": "汇总结果", "status": "pending"}}})
	go func() {
		defer s.Coordinator().finishRequest(key, taskID)
		s.runBatchIdentify(batchCtx, taskID, ids, retry, recordIDs, pendingOnly...)
	}()
	return taskID, nil
}

func (s *ShareRecordService) runBatchIdentify(ctx context.Context, taskID string, ids []int, retry bool, recordIDs []int, pendingOnly ...bool) {
	onlyPending := len(pendingOnly) > 0 && pendingOnly[0]
	onlyFailed := len(pendingOnly) > 1 && pendingOnly[1]
	if _, ok := ctx.Value(shareEpochContext{}).(map[int]uint64); !ok {
		ctx = s.Coordinator().batchContext(ctx, taskID)
	}
	settings, settingsErr := s.unitSettings(ctx)
	if settingsErr != nil {
		_ = s.tasks.SetError(taskID, settingsErr.Error())
		return
	}
	ctx, cancel := newShareTaskContext(ctx, settings.TimeoutMinutes)
	ctx = withShareWorkRound(context.WithValue(ctx, shareTaskSettingsContext{}, settings), retry)
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
	preparationStart := time.Now()
	records, err := s.dao.ListIdentifyMedia(ctx, dao.ShareIdentifyFilter{RecordIDs: recordIDs, MediaIDs: ids, PendingOnly: onlyPending, FailedOnly: retry || onlyFailed, ForceRefresh: bypassShareRecognitionCache(ctx)})
	if err != nil {
		_ = s.tasks.SetError(taskID, err.Error())
		return
	}
	contextIDs := make([]int, 0, len(records))
	for _, record := range records {
		contextIDs = append(contextIDs, record.ID)
	}
	evidence, err := s.dao.ListIdentifyContext(ctx, contextIDs)
	if err != nil {
		_ = s.tasks.SetError(taskID, err.Error())
		return
	}
	cancelledShares := make(map[int]bool)
	prepareShareEpisodeInputs(ctx, evidence)
	if s.tmdb != nil {
		s.tmdb.seedShareWorks(ctx, evidence)
	}
	observeShareMetric(ctx, "db_prepare", time.Since(preparationStart))
	items := make([]domain.ShareMedia, 0)
	maskedTotal := 0
	for _, record := range records {
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
			if shouldIdentifyShareMedia(media, record.MediaType, retry, onlyPending, onlyFailed) {
				media.MediaType = record.MediaType
				items = append(items, media)
			}
		}
	}
	_ = s.tasks.UpdateProgress(taskID, len(items), 0, 0, 0)
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "masked": maskedTotal})
	if len(items) == 0 {
		// 没有待识别媒体时任务仍是正常完成，进度应显示100%，避免出现“完成但0%”的误导状态。
		_ = s.tasks.UpdateProgressPercent(taskID, 100)
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "识别媒体", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "running"}, {"name": "汇总结果", "status": "pending"}}})
	success, failed, skipped := 0, 0, 0
	identifiedFileIDs := make([]int, 0, len(items))
	type identifyResult struct {
		media domain.ShareMedia
		ok    bool
		err   error
	}
	jobs := make(chan domain.ShareMedia)
	results := make(chan identifyResult, len(items))
	var wg sync.WaitGroup
	workers := settings.WorkerCount
	if workers < 1 {
		workers = settings.IdentifyWorkers
	}
	if workers > len(items) {
		workers = len(items)
	}
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for media := range jobs {
				if ctx.Err() != nil || s.tasks.IsCancelled(taskID) {
					continue
				}
				ok, err := s.identifyOne(ctx, media, retry)
				results <- identifyResult{media: media, ok: ok, err: err}
			}
		}()
	}
	go func() {
		defer close(results)
		defer wg.Wait()
		for _, media := range items {
			select {
			case jobs <- media:
			case <-ctx.Done():
				close(jobs)
				return
			}
		}
		close(jobs)
	}()
	processed := 0
	for result := range results {
		processed++
		if errors.Is(result.err, ErrShareUnitSkipped) || errors.Is(result.err, sql.ErrNoRows) {
			skipped++
		} else if result.err != nil || !result.ok {
			failed++
		} else {
			success++
			identifiedFileIDs = append(identifiedFileIDs, result.media.ID)
		}
		_ = s.tasks.UpdateProgress(taskID, len(items), processed, success, failed)
		_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "识别媒体", "current_file": result.media.FileName, "retry_failed": retry, "worker_count": workers, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "running", "message": fmt.Sprintf("正在处理 %d/%d", processed, len(items))}, {"name": "汇总结果", "status": "pending"}}})
	}
	if ctx.Err() != nil {
		s.finishShareTaskContext(taskID, ctx.Err(), settings.TimeoutMinutes)
		return
	}
	sort.Ints(identifiedFileIDs)
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "汇总结果", "current_file": "", "success": success, "failed": failed, "skipped": skipped, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "completed"}, {"name": "汇总结果", "status": "running"}}})
	if failed > 0 {
		_ = s.tasks.SetError(taskID, fmt.Sprintf("识别完成，但有 %d 项失败", failed))
		return
	}
	autoExport := s.autoExport
	if s.autoExportContext != nil {
		autoExport = func(ids []int) (string, bool, error) { return s.autoExportContext(ctx, ids) }
	}
	if autoExport != nil && len(identifiedFileIDs) > 0 {
		exportID, started, exportErr := autoExport(identifiedFileIDs)
		if exportErr != nil {
			logger.Warnf("ShareRecordService[runBatchIdentify] task=%s 自动导出STRM失败: %v", taskID, exportErr)
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"strm_export_error": exportErr.Error()})
		} else if started {
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"strm_export_task_id": exportID, "strm_export_file_count": len(identifiedFileIDs)})
		}
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"timeout_minutes": settings.TimeoutMinutes, "cancelled_shares": len(cancelledShares), "phase": "汇总结果", "current_file": "", "success": success, "failed": failed, "skipped": skipped, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "completed"}, {"name": "汇总结果", "status": "completed"}}})
	_ = s.tasks.UpdateStatus(taskID, "completed")
	logger.Infof("ShareRecordService[runBatchIdentify] task=%s completed success=%d failed=%d", taskID, success, failed)
}
func (s *ShareRecordService) BatchIdentify(ctx context.Context, ids []int, retry bool) (domain.ShareIdentifySummary, error) {
	ctx = withShareWorkRound(ctx, retry)
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

// ListReviewItems 查询手动核对队列，并统一校验筛选和分页参数。
func (s *ShareRecordService) ListReviewItems(ctx context.Context, statuses, keyword string, shareID, mediaID, page, size int) (domain.ShareReviewPage, error) {
	if page < 1 || page > 10000000 || shareID < 0 || mediaID < 0 || size < 1 || size > 100 {
		return domain.ShareReviewPage{}, fmt.Errorf("分页参数无效")
	}
	allowed := map[string]bool{"failed": true, "pending": true}
	parts := strings.Split(statuses, ",")
	selected := make([]string, 0, 2)
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		if !allowed[part] {
			return domain.ShareReviewPage{}, fmt.Errorf("核对状态无效")
		}
		seen[part] = true
		selected = append(selected, part)
	}
	if len(selected) == 0 {
		selected = []string{"failed", "pending"}
	}
	result, err := s.dao.ListReviewItems(ctx, selected, strings.TrimSpace(keyword), shareID, mediaID, page, size)
	if err != nil {
		return result, err
	}
	for i := range result.Data {
		item := &result.Data[i]
		if s.tmdb != nil {
			parsed := s.tmdb.ParseFilename(item.FileName)
			item.ParsedTitle, item.ParsedYear = parsed.Title, parsed.Year
			if len(item.Episodes) == 0 && parsed.Episode > 0 {
				numbers := parsed.Episodes
				if len(numbers) == 0 {
					numbers = []int{parsed.Episode}
				}
				for _, number := range numbers {
					item.Episodes = append(item.Episodes, domain.ShareEpisode{SeasonNumber: parsed.Season, EpisodeNumber: number})
				}
			}
			if item.MediaType == "" || item.MediaType == "auto" {
				item.MediaType = parsed.MediaType
			}
		}
	}
	return result, nil
}

// shouldIdentifyShareMedia 继续任务仅处理未尝试候选，不重试失败或覆盖已识别记录。
func shouldIdentifyShareMedia(media domain.ShareMedia, mediaType string, retry, pendingOnly bool, failedOnly ...bool) bool {
	if retry || (len(failedOnly) > 0 && failedOnly[0]) {
		return media.Status == "failed"
	}
	if pendingOnly {
		return media.Status == "pending" || media.Status == ""
	}
	return retry || media.Status != "identified" || media.Result == nil || (mediaType != "auto" && media.Result.MediaType != mediaType)
}
