package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const shareStrmSettingsKey = "share_strm_settings"

// ShareStrmStore 封装导出来源、播放映射及跨进程转存锁。
type ShareStrmStore interface {
	StrmSources(context.Context, domain.ShareLibraryQuery, int) ([]domain.ShareStrmSource, error)
	SaveStrmEntry(context.Context, domain.ShareStrmEntry) error
	SaveExportedStrmFile(context.Context, domain.StrmFile) error
	GetStrmEntry(context.Context, string) (domain.ShareStrmEntry, error)
	LockStrmPlayback(context.Context, string) (func(), error)
}

// ShareStrmService 负责资料库导出及按需转存播放，复用现有分类、任务和115能力。
type ShareStrmService struct {
	exportDB   *sql.DB
	output     *StrmOutput
	store      ShareStrmStore
	settings   ShareTaskSettingsStore
	client     Cloud115Client
	tmdb       *TmdbService
	organizer  *OrganizeService
	tasks      *TaskService
	categories func() ([]*domain.MediaCategory, error)
	account    func(int) (*domain.Cloud115, error)
	directLink func(string, int, string, string) (string, error)
	exportMu   sync.Mutex
}

// SetExportDatabase 注入统一输出清单数据库。
func (s *ShareStrmService) SetExportDatabase(db *sql.DB) { s.exportDB = db }

// RunScheduledExport 在调度任务上下文中同步执行，不另建后台任务。
func (s *ShareStrmService) RunScheduledExport(ctx context.Context, id string) error {
	return s.RunExportQuery(ctx, id, domain.ShareLibraryQuery{})
}

// RunExportQuery 在已有任务中执行保存的筛选条件。
func (s *ShareStrmService) RunExportQuery(ctx context.Context, id string, q domain.ShareLibraryQuery) error {
	if !s.exportMu.TryLock() {
		return fmt.Errorf("分享导出正在执行")
	}
	defer s.exportMu.Unlock()
	cfg, e := s.Settings()
	if e != nil {
		return e
	}
	if e = validateShareStrmSettings(&cfg); e != nil {
		return e
	}
	return s.export(ctx, cfg, q, id)
}

// NewShareStrmService 在装配层注入持久化、外部客户端和现有业务服务。
func NewShareStrmService(store ShareStrmStore, settings ShareTaskSettingsStore, client Cloud115Client, tmdb *TmdbService, organizer *OrganizeService, tasks *TaskService, categories func() ([]*domain.MediaCategory, error), account func(int) (*domain.Cloud115, error), directLink func(string, int, string, string) (string, error)) *ShareStrmService {
	return &ShareStrmService{store: store, settings: settings, client: client, tmdb: tmdb, organizer: organizer, tasks: tasks, categories: categories, account: account, directLink: directLink}
}

// Settings 获取持久化导出配置。
func (s *ShareStrmService) Settings() (domain.ShareStrmSettings, error) {
	v := domain.ShareStrmSettings{}
	row, err := s.settings.GetByKey(shareStrmSettingsKey)
	if err != nil || row == nil {
		return v, err
	}
	err = json.Unmarshal([]byte(row.ConfigVal), &v)
	return v, err
}

func validateShareStrmSettings(v *domain.ShareStrmSettings) error {
	v.OutputPath = strings.TrimSpace(v.OutputPath)
	v.BaseURL = strings.TrimRight(strings.TrimSpace(v.BaseURL), "/")
	v.TransferPath = strings.TrimSpace(v.TransferPath)
	u, err := url.Parse(v.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("播放地址须为HTTP(S)服务地址，不包含查询参数或账号密码")
	}
	if !filepath.IsAbs(v.OutputPath) {
		return fmt.Errorf("导出目录须为服务器上的绝对路径")
	}
	if v.Cloud115ID <= 0 {
		return fmt.Errorf("请选择115转存账号")
	}
	if !strings.HasPrefix(v.TransferPath, "/") || strings.Contains(v.TransferPath, "\\") {
		return fmt.Errorf("115转存目录须为以 / 开头的路径")
	}
	return nil
}

// SaveSettings 保存导出与播放配置；新播放请求读取最新账号设置。
func (s *ShareStrmService) SaveSettings(v domain.ShareStrmSettings) error {
	if err := validateShareStrmSettings(&v); err != nil {
		return err
	}
	account, err := s.account(v.Cloud115ID)
	if err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("115账号不存在")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.settings.Upsert(shareStrmSettingsKey, string(raw))
}

// StartExport 为当前筛选的全部作品创建导出任务，分页条件不限制导出范围。
func (s *ShareStrmService) StartExport(q domain.ShareLibraryQuery) (string, error) {
	if err := ValidateLibraryQuery(&q); err != nil {
		return "", err
	}
	cfg, err := s.Settings()
	if err != nil {
		return "", err
	}
	if err = validateShareStrmSettings(&cfg); err != nil {
		return "", err
	}
	if !s.exportMu.TryLock() {
		return "", fmt.Errorf("资料库STRM导出正在执行，请在任务中心查看")
	}
	if s.tasks == nil {
		s.exportMu.Unlock()
		return "", fmt.Errorf("任务服务未初始化")
	}
	id := "share_strm_" + uuid.NewString()
	if err = s.tasks.Create(id, "strm_generate", "分享资料库STRM导出"); err != nil {
		s.exportMu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.RegisterCancel(id, cancel)
	go func() {
		defer s.exportMu.Unlock()
		defer s.tasks.RemoveCancel(id)
		defer cancel()
		defer func() {
			if v := recover(); v != nil {
				_ = s.tasks.SetError(id, fmt.Sprint(v))
			}
		}()
		if err := s.tasks.UpdateStatus(id, "running"); err != nil {
			_ = s.tasks.SetError(id, err.Error())
			return
		}
		err := s.export(ctx, cfg, q, id)
		if ctx.Err() != nil || s.tasks.IsCancelled(id) {
			_ = s.tasks.UpdateStatus(id, "cancelled")
			return
		}
		if err != nil {
			_ = s.tasks.SetError(id, err.Error())
			return
		}
		_ = s.tasks.UpdateStatus(id, "completed")
	}()
	return id, nil
}

func (s *ShareStrmService) strmRelativePath(source domain.ShareStrmSource, file domain.ShareFileInfo, cats []*domain.MediaCategory) (string, error) {
	r := source.Result
	if !r.Success || strings.TrimSpace(r.Title) == "" || (r.MediaType != "tv" && r.MediaType != "movie") {
		return "", fmt.Errorf("作品识别信息不完整")
	}
	title := s.organizer.sanitizeFolderName(strings.NewReplacer("/", " ", "\\", " ").Replace(r.Title))
	title = strings.Trim(title, " .")
	if title == "" {
		return "", fmt.Errorf("作品名称无效")
	}
	folder := fmt.Sprintf("%s (%d)", title, r.Year)
	if r.TmdbID > 0 {
		folder += fmt.Sprintf(" {tmdb-%d}", r.TmdbID)
	} else {
		folder += " {" + uuid.NewSHA1(uuid.NameSpaceURL, []byte(source.WorkKey)).String()[:8] + "}"
	}
	category := s.organizer.matchCategoryPath(&r, cats)
	root := r.MediaType
	if category != "" {
		root = s.organizer.prependCategoryTargetPath("", category)
	}
	if root == ".." || filepath.IsAbs(root) {
		return "", fmt.Errorf("分类目录无效")
	}
	name := folder
	if r.MediaType == "tv" {
		season, episode := r.SeasonNumber, r.EpisodeNumber
		if episode <= 0 || season < 0 {
			return "", fmt.Errorf("文件缺少持久化季集映射：%s", file.Path)
		}
		name = fmt.Sprintf("%s - S%02dE%02d", title, season, episode)
		return filepath.Join(root, folder, fmt.Sprintf("Season %02d", season), name+".strm"), nil
	}
	return filepath.Join(root, folder, name+".strm"), nil
}

var shareStrmSeasonDirectory = regexp.MustCompile(`(?i)^(?:Season[ ._-]*|S|第)(\d{1,2})(?:季)?$`)
var shareStrmEpisodeNumber = regexp.MustCompile(`^\d{1,3}$`)

// strmSeasonEpisode 复用当前文件名规则；缺少片名的单集补作品名，季目录只补未显式指定的季。
func (s *ShareStrmService) strmSeasonEpisode(title, filename string) (int, int) {
	name := path.Base(shareCandidatePath(filename))
	parsed := s.tmdb.ParseFilename(name)
	if parsed.Episode == 0 {
		parsed = s.tmdb.ParseFilename(title + " " + name)
	}
	explicitSeason := false
	for _, rule := range s.tmdb.compiledRulesForFilenameParsing() {
		if rule.rule.ID == parsed.MatchedRuleID && rule.expression.SubexpIndex("season") >= 0 {
			explicitSeason = true
			break
		}
	}
	if explicitSeason {
		return parsed.Season, parsed.Episode
	}
	segments := strings.Split(shareCandidatePath(filename), "/")
	for i := len(segments) - 2; i >= 0; i-- {
		if match := shareStrmSeasonDirectory.FindStringSubmatch(strings.TrimSpace(segments[i])); match != nil {
			season, _ := strconv.Atoi(match[1])
			if parsed.Episode == 0 {
				stem := strings.TrimSuffix(name, path.Ext(name))
				if shareStrmEpisodeNumber.MatchString(stem) {
					parsed.Episode, _ = strconv.Atoi(stem)
				}
			}
			return season, parsed.Episode
		}
	}
	return parsed.Season, parsed.Episode
}

func writeShareStrm(target, content string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	// 临时文件写完再替换，播放器不会读到半个URL。
	f, err := os.CreateTemp(filepath.Dir(target), ".share-strm-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err != nil {
		_ = f.Close()
		return err
	}
	if _, err = f.WriteString(content + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), target)
}

// Playback 仅转存映射指定的一个文件，复用目标目录中的文件后获取即时直链。
func (s *ShareStrmService) Playback(ctx context.Context, id, ua string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cfg, err := s.Settings()
	if err != nil {
		return "", err
	}
	if err = validateShareStrmSettings(&cfg); err != nil {
		return "", err
	}
	entry, err := s.store.GetStrmEntry(ctx, id)
	if err != nil {
		return "", err
	}
	account, err := s.account(cfg.Cloud115ID)
	if err != nil {
		return "", err
	}
	if account == nil {
		return "", fmt.Errorf("115转存账号不存在")
	}
	unlock, err := s.store.LockStrmPlayback(ctx, fmt.Sprintf("share-strm:%d:%s:%s", cfg.Cloud115ID, cfg.TransferPath, id))
	if err != nil {
		return "", err
	}
	defer unlock()
	dir := path.Join(cfg.TransferPath, id)
	cid, err := s.client.MkdirAll115(dir, account.ID, account.Cookie)
	if err != nil {
		return "", fmt.Errorf("创建转存目录失败：%w", err)
	}
	// 每个映射独占目录，以列表查找精确文件名；网络错误不能被当成文件不存在。
	pick, err := s.findTransferred(ctx, cid, entry.FileName, account)
	if err != nil {
		return "", err
	}
	if pick == "" {
		// 等锁期间另一请求可能已经缓存文件ID，重新读取后再按需定位。
		entry, err = s.store.GetStrmEntry(ctx, id)
		if err != nil {
			return "", err
		}
		if entry.FileID == "" {
			entry.FileID, err = s.resolveStrmFileID(ctx, entry)
			if err != nil {
				return "", err
			}
			if err = s.store.SaveStrmEntry(ctx, entry); err != nil {
				return "", err
			}
		}
		if err = s.client.ReceiveShare(entry.ShareCode, entry.Password, entry.FileID, cid, account.ID, account.Cookie); err != nil {
			return "", fmt.Errorf("115分享转存失败：%w", err)
		}
		for attempt := 0; attempt < 5; attempt++ {
			pick, err = s.findTransferred(ctx, cid, entry.FileName, account)
			if err != nil {
				return "", err
			}
			if pick != "" {
				break
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
		if pick == "" {
			return "", fmt.Errorf("115已接受转存，但文件暂未可见，请稍后重试")
		}
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	link, err := s.directLink(pick, account.ID, account.Cookie, ua)
	if err != nil {
		return "", fmt.Errorf("获取115直链失败：%w", err)
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("115返回无效直链")
	}
	return link, nil
}

func (s *ShareStrmService) findTransferred(ctx context.Context, cid, name string, account *domain.Cloud115) (string, error) {
	var directory int
	if _, err := fmt.Sscan(cid, &directory); err != nil {
		return "", fmt.Errorf("115目录ID无效")
	}
	for offset := 0; ; offset += 100 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		files, err := s.client.GetFileList(directory, 0, offset, 100, account.ID, account.Cookie)
		if err != nil {
			return "", err
		}
		if files == nil {
			return "", fmt.Errorf("115文件列表为空响应")
		}
		for _, f := range files.Files {
			if f.Name == name && f.Type != "folder" && f.FileID != "" {
				if f.PickCode == "" {
					return "", fmt.Errorf("视频已转存，但115未返回提取码，请稍后重试")
				}
				return f.PickCode, nil
			}
		}
		if len(files.Files) < 100 {
			return "", nil
		}
	}
}
