package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"
	"time"
)

var (
	shareRecordURLPattern  = regexp.MustCompile(`https?://(?:www\.)?(?:115cdn\.com|115\.com)/s/[A-Za-z0-9_]+[^\s\])>]*`)
	shareAccessCodePattern = regexp.MustCompile(`(?:访问码|提取码|密码)\s*[：:]\s*([A-Za-z0-9]+)`)
)

type ShareRecordService struct {
	dao    *dao.ShareRecordDAO
	tmdb   *TmdbService
	tasks  *TaskService
	parser ShareRecordParser
}

// ShareRecordParser 获取分享中的文件列表。
type ShareRecordParser interface {
	ParseShareLink(context.Context, string, string) (*domain.ParseShareResponse, error)
}

func NewShareRecordService(d *dao.ShareRecordDAO, t *TmdbService, tasks *TaskService, parser ShareRecordParser) *ShareRecordService {
	return &ShareRecordService{dao: d, tmdb: t, tasks: tasks, parser: parser}
}
func (s *ShareRecordService) List(ctx context.Context, q domain.ShareRecordQuery) (domain.ShareRecordPage, error) {
	page, err := s.dao.List(ctx, q)
	for i := range page.Data {
		normalizeShareMaskedMedia(&page.Data[i])
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

// saveMaskedDirectories 按路径出现次数补齐统计记录，避免同名目录少计或重复扫描累加。
func (s *ShareRecordService) saveMaskedDirectories(ctx context.Context, record domain.ShareRecord, directories []domain.ShareFileInfo) error {
	existing := make(map[string]int)
	for _, media := range record.Media {
		existing[media.FileName]++
	}
	for _, directory := range directories {
		name := directory.Path
		if name == "" {
			name = directory.Name
		}
		if existing[name] > 0 {
			existing[name]--
			continue
		}
		if err := s.AddMedia(ctx, &domain.ShareMedia{ShareID: record.ID, FileName: name}); err != nil {
			return fmt.Errorf("保存脱敏目录统计失败: %w", err)
		}
	}
	return nil
}
func (s *ShareRecordService) Create(ctx context.Context, r *domain.ShareRecord) error {
	if r.MediaType == "" {
		r.MediaType = "movie"
	}
	if r.MediaType != "movie" && r.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为电影或电视剧")
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
		if strings.TrimSpace(r.Media[i].FileName) == "" {
			return fmt.Errorf("媒体文件名不能为空")
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
	if r.MediaType != "movie" && r.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为电影或电视剧")
	}
	return s.dao.Update(ctx, r)
}
func (s *ShareRecordService) Delete(ctx context.Context, id int) error { return s.dao.Delete(ctx, id) }
func (s *ShareRecordService) AddMedia(ctx context.Context, m *domain.ShareMedia) error {
	if strings.TrimSpace(m.FileName) == "" {
		return fmt.Errorf("媒体文件名不能为空")
	}
	if m.MetadataSource == "" {
		m.MetadataSource = domain.MetadataSourceAuto
	}
	return s.dao.AddMedia(ctx, m)
}
func (s *ShareRecordService) DeleteMedia(ctx context.Context, id int) error {
	return s.dao.DeleteMedia(ctx, id)
}
func (s *ShareRecordService) Identify(ctx context.Context, m domain.ShareMedia, retry bool) error {
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
	return s.dao.Identify(ctx, m, "identified", m.Result, "")
}

func (s *ShareRecordService) identifyOne(ctx context.Context, m domain.ShareMedia, retry bool) (bool, error) {
	if isMaskedSharePath(m.FileName) {
		return false, fmt.Errorf("目录名称已脱敏，已跳过识别")
	}
	if !retry && m.Status == "identified" && (m.MediaType == "" || (m.Result != nil && m.Result.MediaType == m.MediaType)) {
		return true, nil
	}
	r, e := s.tmdb.identifyFile(m.FileName, m.MetadataSource, m.MediaType)
	if e != nil {
		logger.Errorf("[ShareIdentify] TMDB识别异常 | directory=%q | source=%s | error=%v", m.FileName, m.MetadataSource, e)
		return false, s.dao.Identify(ctx, m, "failed", nil, e.Error())
	}
	// 分享识别复用整理/识别测试链路的详情补全规则，保证中文名、海报及分类元数据一致。
	s.tmdb.EnsureIdentifyMetadata(r)
	st, msg := "failed", r.Message
	if r.Success {
		st, msg = "identified", ""
	}
	logger.Infof("[ShareIdentify] TMDB识别结果 | directory=%q | success=%v | media_type=%s | tmdb_id=%d | title=%q | original_title=%q | year=%d | message=%q", m.FileName, r.Success, r.MediaType, r.TmdbID, r.Title, r.OriginalTitle, r.Year, r.Message)
	return st == "identified", s.dao.Identify(ctx, m, st, r, msg)
}

// StartBatchIdentify 创建后台识别任务并立即返回任务 ID。
func (s *ShareRecordService) StartBatchIdentify(ctx context.Context, ids []int, retry bool) (string, error) {
	return s.startIdentifyTask(ctx, ids, retry, nil)
}

// StartRecordIdentify 为单条分享创建识别任务，并只处理该分享下的全部媒体。
func (s *ShareRecordService) StartRecordIdentify(ctx context.Context, recordID int) (string, error) {
	return s.startIdentifyTask(ctx, nil, false, []int{recordID})
}

func (s *ShareRecordService) startIdentifyTask(ctx context.Context, ids []int, retry bool, recordIDs []int) (string, error) {
	if s.tasks == nil {
		return "", fmt.Errorf("任务服务未初始化")
	}
	taskID := fmt.Sprintf("share_identify_%d", time.Now().UnixNano())
	if err := s.tasks.Create(taskID, "share_identify", "分享媒体批量识别"); err != nil {
		return "", err
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "准备媒体列表", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "running"}, {"name": "识别媒体", "status": "pending"}, {"name": "汇总结果", "status": "pending"}}})
	go s.runBatchIdentify(context.Background(), taskID, ids, retry, recordIDs)
	return taskID, nil
}

func (s *ShareRecordService) runBatchIdentify(ctx context.Context, taskID string, ids []int, retry bool, recordIDs []int) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
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
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "获取分享媒体", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "running"}, {"name": "获取分享媒体", "status": "running"}, {"name": "识别媒体", "status": "pending"}, {"name": "汇总结果", "status": "pending"}}})
	if s.parser != nil {
		parseErrors := make([]string, 0)
		parseTotal := len(page.Data)
		if parseTotal > 0 {
			// 解析阶段也提供可见进度，避免大分享在网络读取期间长期停留在 0%。
			_ = s.tasks.UpdateProgressPercent(taskID, 1)
		}
		for recordIndex, record := range page.Data {
			if len(recordIDs) > 0 && !contains(recordIDs, record.ID) {
				continue
			}
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "获取分享媒体", "current_share": record.Name, "current_share_id": record.ID, "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "获取分享媒体", "status": "running", "message": fmt.Sprintf("正在解析分享 %d", record.ID)}, {"name": "识别媒体", "status": "pending"}, {"name": "汇总结果", "status": "pending"}}})
			logger.Infof("ShareRecordService[runBatchIdentify] task=%s parsing share=%d name=%q", taskID, record.ID, record.Name)
			parsed, parseErr := s.parser.ParseShareLink(ctx, record.URL, record.Password)
			if parseErr != nil {
				message := fmt.Sprintf("分享 %d（%s）解析失败: %v", record.ID, record.Name, parseErr)
				parseErrors = append(parseErrors, message)
				logger.Errorf("ShareRecordService[runBatchIdentify] task=%s %s", taskID, message)
				if parseTotal > 0 {
					percent := 1 + int(float64(recordIndex+1)/float64(parseTotal)*19)
					_ = s.tasks.UpdateProgressPercent(taskID, percent)
				}
				continue
			}
			logger.Infof("ShareRecordService[runBatchIdentify] task=%s share=%d parsed_items=%d", taskID, record.ID, len(parsed.Files))
			if err := s.saveMaskedDirectories(ctx, record, parsed.MaskedDirectories); err != nil {
				_ = s.tasks.SetError(taskID, err.Error())
				return
			}
			existing := map[string]bool{}
			for _, media := range record.Media {
				existing[media.FileName] = true
			}
			for _, file := range parsed.Files {
				fileName := file.Path
				if fileName == "" {
					fileName = file.Name
				}
				if (file.IsDir && file.Type != "media") || (file.Type != "" && file.Type != "video" && file.Type != "media") || existing[fileName] {
					continue
				}
				if addErr := s.AddMedia(ctx, &domain.ShareMedia{ShareID: record.ID, FileName: fileName, MetadataSource: domain.MetadataSourceAuto}); addErr != nil {
					logger.Errorf("ShareRecordService[runBatchIdentify] task=%s share=%d add_media=%q error=%v", taskID, record.ID, fileName, addErr)
				}
			}
			// 解析阶段占任务前 20%，识别阶段由文件处理进度接管。
			if parseTotal > 0 {
				percent := 1 + int(float64(recordIndex+1)/float64(parseTotal)*19)
				_ = s.tasks.UpdateProgressPercent(taskID, percent)
			}
		}
		page, err = s.List(ctx, domain.ShareRecordQuery{Page: 1, PageSize: 200})
		if err != nil {
			_ = s.tasks.SetError(taskID, err.Error())
			return
		}
		if len(parseErrors) > 0 {
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"parse_errors": parseErrors})
			if len(recordIDs) > 0 {
				_ = s.tasks.SetError(taskID, strings.Join(parseErrors, "; "))
				return
			}
		}
	}
	items := make([]domain.ShareMedia, 0)
	mediaTotal, maskedTotal := 0, 0
	for _, record := range page.Data {
		if len(recordIDs) > 0 && !contains(recordIDs, record.ID) {
			continue
		}
		for _, media := range record.Media {
			if isMaskedSharePath(media.FileName) {
				maskedTotal++
				logger.Infof("[ShareIdentify] 跳过已有脱敏媒体 | task=%s | directory=%q", taskID, media.FileName)
				continue
			}
			mediaTotal++
			if (len(ids) == 0 || contains(ids, media.ID)) && (retry || media.Status != "identified" || media.Result == nil || media.Result.MediaType != record.MediaType) {
				media.MediaType = record.MediaType
				items = append(items, media)
			}
		}
	}
	_ = s.tasks.UpdateProgress(taskID, len(items), 0, 0, 0)
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"masked": maskedTotal})
	if len(items) == 0 {
		if mediaTotal == 0 && maskedTotal == 0 {
			_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "准备媒体列表", "current_file": "", "success": 0, "failed": 0, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "failed", "message": "未从分享目录中解析到媒体候选"}, {"name": "识别媒体", "status": "pending"}, {"name": "汇总结果", "status": "pending"}}})
			_ = s.tasks.SetError(taskID, "未从分享目录中解析到媒体候选，请检查目录层级和分享内容")
			return
		}
		// 没有待识别媒体时任务仍是正常完成，进度应显示100%，避免出现“完成但0%”的误导状态。
		_ = s.tasks.UpdateProgressPercent(taskID, 100)
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "识别媒体", "current_file": "", "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "running"}, {"name": "汇总结果", "status": "pending"}}})
	success, failed := 0, 0
	for index, media := range items {
		if err := ctx.Err(); err != nil {
			_ = s.tasks.SetError(taskID, "分享识别任务超时或已取消: "+err.Error())
			return
		}
		if s.tasks.IsCancelled(taskID) {
			return
		}
		ok, identifyErr := s.identifyOne(ctx, media, retry)
		if identifyErr != nil || !ok {
			failed++
		} else {
			success++
		}
		_ = s.tasks.UpdateProgress(taskID, len(items), index+1, success, failed)
		_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "识别媒体", "current_file": media.FileName, "retry_failed": retry, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "running", "message": fmt.Sprintf("正在处理 %d/%d", index+1, len(items))}, {"name": "汇总结果", "status": "pending"}}})
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "汇总结果", "current_file": "", "success": success, "failed": failed, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "completed"}, {"name": "汇总结果", "status": "running"}}})
	if failed > 0 {
		_ = s.tasks.SetError(taskID, fmt.Sprintf("识别完成，但有 %d 项失败", failed))
		return
	}
	_ = s.tasks.UpdateMetadata(taskID, map[string]interface{}{"phase": "汇总结果", "current_file": "", "success": success, "failed": failed, "steps": []map[string]interface{}{{"name": "准备媒体列表", "status": "completed"}, {"name": "识别媒体", "status": "completed"}, {"name": "汇总结果", "status": "completed"}}})
	_ = s.tasks.UpdateStatus(taskID, "completed")
	logger.Infof("ShareRecordService[runBatchIdentify] task=%s completed success=%d failed=%d", taskID, success, failed)
}
func (s *ShareRecordService) BatchIdentify(ctx context.Context, ids []int, retry bool) (domain.ShareIdentifySummary, error) {
	p, e := s.List(ctx, domain.ShareRecordQuery{Page: 1, PageSize: 200})
	if e != nil {
		return domain.ShareIdentifySummary{}, e
	}
	sum := domain.ShareIdentifySummary{}
	for _, r := range p.Data {
		for _, m := range r.Media {
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
