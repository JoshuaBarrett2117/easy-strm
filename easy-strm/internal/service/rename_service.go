package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// RenameService 更名服务
// 负责文件更名规则的生成和执行
type RenameService struct {
	mediaSourceService *MediaSourceService
	tmdbService        *TmdbService
	renamePresetDAO    *dao.RenamePresetDAO
	systemConfigDAO    *dao.SystemConfigDAO
}

const (
	legacyDefaultMovieTemplate = `{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}`
	legacyDefaultTVTemplate    = `{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}`
	defaultMovieTemplate       = `{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}`
	defaultTVTemplate          = `{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}`
)

// NewRenameService 创建更名服务实例
// 参数:
//   - mediaSourceService: 媒体源服务
//   - tmdbService: TMDB 服务
//   - renamePresetDAO: 更名预设 DAO
//
// 返回:
//   - *RenameService: 更名服务实例
func NewRenameService(
	mediaSourceService *MediaSourceService,
	tmdbService *TmdbService,
	renamePresetDAO *dao.RenamePresetDAO,
	systemConfigDAO *dao.SystemConfigDAO,
) *RenameService {
	return &RenameService{
		mediaSourceService: mediaSourceService,
		tmdbService:        tmdbService,
		renamePresetDAO:    renamePresetDAO,
		systemConfigDAO:    systemConfigDAO,
	}
}

// PreviewRename 预览更名结果
// 参数:
//   - req: 更名预览请求
//
// 返回:
//   - *domain.RenamePreviewResult: 预览结果
//   - error: 错误信息
func (s *RenameService) PreviewRename(req *domain.RenamePreviewRequest) (*domain.RenamePreviewResult, error) {
	logger.Infof("RenameService[PreviewRename] 开始预览: source_id=%d, file_id=%s", req.SourceID, req.FileID)

	// 获取媒体源
	source, err := s.mediaSourceService.GetByID(req.SourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	// 构建原始文件路径
	// 业务背景：FileID 可能是相对路径或绝对路径，需要智能判断
	var originalPath string
	if source.SourceType == domain.SourceTypeLocal {
		// 检查 FileID 是否已经是绝对路径
		// 修复：避免路径重复拼接（如 c:\path1\c:\path2）
		if filepath.IsAbs(req.FileID) {
			originalPath = req.FileID
		} else {
			originalPath = filepath.Join(source.Path, req.FileID)
		}
	} else {
		// 115 云盘，FileID 就是 CID 或 pickcode
		originalPath = req.FileID
	}

	// 获取原始文件名
	originalName := filepath.Base(originalPath)
	ext := filepath.Ext(originalName)

	// 如果没有提供模板，使用默认模板
	template := req.Template
	if template == "" {
		template = s.getDefaultTemplate(req.MediaType)
	}

	// 如果提供了 TMDB ID，获取媒体信息
	var title string
	var enTitle string
	var year int
	var season, episode int

	if req.TmdbID > 0 {
		// 从 TMDB 获取详细信息
		if req.MediaType == "tv" {
			detail, err := s.tmdbService.GetTVDetail(req.TmdbID)
			if err == nil {
				title, enTitle = pickRenameTitlesFromDetail(detail, req.MediaType, title, enTitle)
				if firstAirDate, ok := detail["first_air_date"].(string); ok && len(firstAirDate) >= 4 {
					year, _ = strconv.Atoi(firstAirDate[:4])
				}
			}
		} else {
			detail, err := s.tmdbService.GetMovieDetail(req.TmdbID)
			if err == nil {
				title, enTitle = pickRenameTitlesFromDetail(detail, req.MediaType, title, enTitle)
				if releaseDate, ok := detail["release_date"].(string); ok && len(releaseDate) >= 4 {
					year, _ = strconv.Atoi(releaseDate[:4])
				}
			}
		}
	}

	// 尝试从文件名解析季集信息
	parsed := s.parseSeasonEpisode(originalName)
	if parsed.Season > 0 {
		season = parsed.Season
	}
	if parsed.Episode > 0 {
		episode = parsed.Episode
	}

	// 如果没有标题，使用原始文件名（去除扩展名和标签）
	if title == "" {
		title = s.cleanTitle(strings.TrimSuffix(originalName, ext))
	}

	// 应用模板生成新文件名
	newName, err := s.applyTemplate(template, title, enTitle, year, season, episode, parsed.Quality, parsed.Source, parsed.Codec, ext, req.TmdbID)
	if err != nil {
		return nil, fmt.Errorf("渲染 Jinja2 命名模板失败: %v", err)
	}
	if filepath.Ext(filepath.Base(newName)) == "" && ext != "" {
		newName += ext
	}

	// 构建新路径
	var newPath string
	if source.SourceType == domain.SourceTypeLocal {
		newPath = filepath.Join(filepath.Dir(originalPath), newName)
	} else {
		newPath = newName
	}

	result := &domain.RenamePreviewResult{
		FileID:       req.FileID,
		OriginalName: originalName,
		NewName:      newName,
		OriginalPath: originalPath,
		NewPath:      newPath,
		TmdbID:       req.TmdbID,
		Title:        title,
		Year:         year,
		MediaType:    req.MediaType,
		Season:       season,
		Episode:      episode,
		Quality:      parsed.Quality,
	}

	logger.Infof("RenameService[PreviewRename] 预览完成: %s -> %s", originalName, newName)
	return result, nil
}

// ExecuteRename 执行更名
// 参数:
//   - req: 更名执行请求
//
// 返回:
//   - *domain.RenameExecuteResult: 执行结果
//   - error: 错误信息
func (s *RenameService) ExecuteRename(req *domain.RenameExecuteRequest) (*domain.RenameExecuteResult, error) {
	logger.Infof("RenameService[ExecuteRename] 开始执行: source_id=%d, file_id=%s", req.SourceID, req.FileID)

	// 获取媒体源
	source, err := s.mediaSourceService.GetByID(req.SourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	// 根据媒体源类型执行更名
	if source.SourceType == domain.SourceTypeLocal {
		return s.executeLocalRename(source, req)
	}

	// 115 云盘暂不支持直接更名
	return nil, fmt.Errorf("115 云盘暂不支持直接更名")
}

// executeLocalRename 执行本地文件更名
// 参数:
//   - source: 媒体源
//   - req: 更名执行请求
//
// 返回:
//   - *domain.RenameExecuteResult: 执行结果
//   - error: 错误信息
func (s *RenameService) executeLocalRename(source *domain.MediaSource, req *domain.RenameExecuteRequest) (*domain.RenameExecuteResult, error) {
	// 构建原始文件路径，检查是否已经是绝对路径
	var originalPath string
	if filepath.IsAbs(req.FileID) {
		originalPath = req.FileID
	} else {
		originalPath = filepath.Join(source.Path, req.FileID)
	}
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	newPath := filepath.Join(dir, req.NewName)

	// 如果新文件名没有扩展名，添加原扩展名
	if filepath.Ext(req.NewName) == "" && ext != "" {
		newPath = newPath + ext
	}

	// 检查原文件是否存在
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("原文件不存在: %s", originalPath)
	}

	// 检查目标文件是否已存在
	if _, err := os.Stat(newPath); err == nil {
		if !req.Overwrite {
			return nil, fmt.Errorf("目标文件已存在: %s", newPath)
		}
		// 删除已存在文件
		if err := os.Remove(newPath); err != nil {
			return nil, fmt.Errorf("删除已存在文件失败: %v", err)
		}
	}

	// 执行重命名
	if err := os.Rename(originalPath, newPath); err != nil {
		logger.Errorf("RenameService[executeLocalRename] 重命名失败: %v", err)
		return nil, fmt.Errorf("重命名失败: %v", err)
	}

	result := &domain.RenameExecuteResult{
		Success:      true,
		Message:      "重命名成功",
		OriginalPath: originalPath,
		NewPath:      newPath,
	}

	logger.Infof("RenameService[executeLocalRename] 重命名成功: %s -> %s", originalPath, newPath)
	return result, nil
}

// GetPresets 获取更名预设列表
// 参数:
//   - mediaType: 媒体类型（可选）
//
// 返回:
//   - []*dao.RenamePreset: 预设列表
//   - error: 错误信息
func (s *RenameService) GetPresets(mediaType string) ([]*dao.RenamePreset, error) {
	if mediaType != "" {
		return s.renamePresetDAO.GetByMediaType(mediaType)
	}
	return s.renamePresetDAO.GetAll()
}

// getDefaultTemplate 获取默认模板
// 参数:
//   - mediaType: 媒体类型
//
// 返回:
//   - string: 默认模板
func (s *RenameService) getDefaultTemplate(mediaType string) string {
	// 尝试从系统配置获取
	configKey := "movie_naming_template"
	if mediaType == "tv" {
		configKey = "tv_naming_template"
	}

	if s.systemConfigDAO != nil {
		if config, err := s.systemConfigDAO.GetByKey(configKey); err == nil && config != nil && config.ConfigVal != "" {
			return s.normalizeBuiltinTemplate(config.ConfigVal, mediaType)
		}
	}

	// 回退到默认值
	if mediaType == "tv" {
		return defaultTVTemplate
	}
	return defaultMovieTemplate
}

func (s *RenameService) normalizeBuiltinTemplate(template, mediaType string) string {
	switch mediaType {
	case "tv":
		if template == legacyDefaultTVTemplate {
			return defaultTVTemplate
		}
	default:
		if template == legacyDefaultMovieTemplate {
			return defaultMovieTemplate
		}
	}
	return template
}

// applyTemplate 应用模板生成文件名
// 参数:
//   - template: 模板字符串
//   - title: 标题
//   - year: 年份
//   - season: 季数
//   - episode: 集数
//   - quality: 质量
//   - source: 来源
//   - codec: 编码
//   - tmdbId: TMDB ID
//
// 返回:
//   - string: 生成的文件名
func (s *RenameService) applyTemplate(template, title, enTitle string, year, season, episode int, quality, source, codec, fileExt string, tmdbId int) (string, error) {
	env := minijinja.NewEnvironment()
	tmpl, err := env.TemplateFromNamedString("rename-template", template)
	if err != nil {
		return "", err
	}

	result, err := tmpl.Render(map[string]any{
		"title":       title,
		"name":        title,
		"en_title":    enTitle,
		"year":        year,
		"season":      season,
		"episode":     episode,
		"videoFormat": quality,
		"quality":     quality,
		"source":      source,
		"codec":       codec,
		"fileExt":     fileExt,
		"tmdbid":      tmdbId,
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result), nil
}

// parseSeasonEpisode 解析文件名中的季集信息
// 参数:
//   - name: 文件名
//
// 返回:
//   - *ParsedEpisode: 解析结果
func (s *RenameService) parseSeasonEpisode(name string) *ParsedEpisode {
	result := &ParsedEpisode{}

	// 季集模式
	patterns := []struct {
		regex   string
		handler func(matches []string)
	}{
		{
			regex: `(?i)S(\d{1,2})E(\d{1,2})`,
			handler: func(matches []string) {
				result.Season, _ = strconv.Atoi(matches[1])
				result.Episode, _ = strconv.Atoi(matches[2])
			},
		},
		{
			regex: `(?i)Season\s*(\d{1,2})\s*Episode\s*(\d{1,2})`,
			handler: func(matches []string) {
				result.Season, _ = strconv.Atoi(matches[1])
				result.Episode, _ = strconv.Atoi(matches[2])
			},
		},
		{
			regex: `(?i)(\d{1,2})x(\d{1,2})`,
			handler: func(matches []string) {
				result.Season, _ = strconv.Atoi(matches[1])
				result.Episode, _ = strconv.Atoi(matches[2])
			},
		},
		{
			regex: `(?i)EP?(\d{1,3})`,
			handler: func(matches []string) {
				result.Episode, _ = strconv.Atoi(matches[1])
				result.Season = 1
			},
		},
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern.regex)
		if matches := re.FindStringSubmatch(name); len(matches) > 0 {
			pattern.handler(matches)
			break
		}
	}

	// 提取质量信息
	qualityPatterns := map[string]string{
		`(?i)\b4K\b`:    "4K",
		`(?i)\b2160p\b`: "2160p",
		`(?i)\b1080p\b`: "1080p",
		`(?i)\b720p\b`:  "720p",
	}
	for pattern, quality := range qualityPatterns {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			result.Quality = quality
			break
		}
	}

	// 提取来源
	sourcePatterns := map[string]string{
		`(?i)\bBluRay\b`:  "BluRay",
		`(?i)\bWEB-?DL\b`: "WEB-DL",
		`(?i)\bHDTV\b`:    "HDTV",
	}
	for pattern, source := range sourcePatterns {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			result.Source = source
			break
		}
	}

	// 提取编码
	codecPatterns := map[string]string{
		`(?i)\bx264\b`: "x264",
		`(?i)\bx265\b`: "x265",
		`(?i)\bHEVC\b`: "HEVC",
	}
	for pattern, codec := range codecPatterns {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			result.Codec = codec
			break
		}
	}

	return result
}

// ParsedEpisode 解析后的季集信息
type ParsedEpisode struct {
	Season  int
	Episode int
	Quality string
	Source  string
	Codec   string
}

// cleanTitle 清理标题
// 参数:
//   - title: 原始标题
//
// 返回:
//   - string: 清理后的标题
func (s *RenameService) cleanTitle(title string) string {
	// 移除常见标签
	tagsToRemove := []string{
		`(?i)\bBluRay\b`, `(?i)\bWEB-?DL\b`, `(?i)\bHDTV\b`,
		`(?i)\bBDRip\b`, `(?i)\bDVDRip\b`, `(?i)\bHDRip\b`,
		`(?i)\bx264\b`, `(?i)\bx265\b`, `(?i)\bH\.?264\b`, `(?i)\bH\.?265\b`,
		`(?i)\bHEVC\b`, `(?i)\bAVC\b`, `(?i)\bAAC\b`, `(?i)\bAC3\b`,
		`(?i)\bDTS\b`, `(?i)\bDD5\.?1\b`, `(?i)\b5\.?1\b`,
		`(?i)\b4K\b`, `(?i)\b2160p\b`, `(?i)\b1080p\b`, `(?i)\b720p\b`,
		`(?i)\bAMZN\b`, `(?i)\bNF\b`, `(?i)\bHMAX\b`,
		`(?i)\bS\d{1,2}E\d{1,2}\b`, `(?i)\bSeason\s*\d+`,
		`(?i)\bEP?\d{1,3}\b`,
	}

	result := title
	for _, tag := range tagsToRemove {
		re := regexp.MustCompile(tag)
		result = re.ReplaceAllString(result, "")
	}

	// 清理多余空格
	result = strings.TrimSpace(result)
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")

	return result
}

func pickRenameTitlesFromDetail(detail map[string]interface{}, mediaType, fallbackTitle, fallbackOriginalTitle string) (string, string) {
	title := fallbackTitle
	originalTitle := fallbackOriginalTitle

	if mediaType == "tv" {
		if value, ok := detail["name"].(string); ok && value != "" {
			title = value
		}
		if value, ok := detail["original_name"].(string); ok && value != "" {
			originalTitle = value
		}
	} else {
		if value, ok := detail["title"].(string); ok && value != "" {
			title = value
		}
		if value, ok := detail["original_title"].(string); ok && value != "" {
			originalTitle = value
		}
	}

	if translations, ok := detail["translations"].(map[string]interface{}); ok {
		if items, ok := translations["translations"].([]interface{}); ok {
			for _, item := range items {
				entry, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				lang, _ := entry["iso_639_1"].(string)
				if lang != "zh" {
					continue
				}
				data, ok := entry["data"].(map[string]interface{})
				if !ok {
					continue
				}
				if mediaType == "tv" {
					if value, ok := data["name"].(string); ok && value != "" {
						title = value
						break
					}
				} else {
					if value, ok := data["title"].(string); ok && value != "" {
						title = value
						break
					}
				}
			}
		}
	}

	if title == "" {
		title = originalTitle
	}
	if originalTitle == "" {
		originalTitle = title
	}
	return title, originalTitle
}
