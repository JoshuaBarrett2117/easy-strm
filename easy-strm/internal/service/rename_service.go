package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

	// 先解析文件名，后续会同时用于媒体类型推断、季集号提取与质量识别
	parsed := s.parseSeasonEpisode(originalName)
	mediaType := normalizeRenameMediaType(req.MediaType, parsed)

	// 如果没有提供模板，使用默认模板
	template := req.Template
	if template == "" {
		template = s.getDefaultTemplate(mediaType)
	}

	// 如果提供了 TMDB ID，获取媒体信息
	var title string
	var enTitle string
	var year int
	var season, episode int
	title = strings.TrimSpace(req.Title)
	year = req.Year
	season = req.Season
	episode = req.Episode

	if req.TmdbID > 0 {
		// 从 TMDB 获取详细信息
		if mediaType == "tv" {
			detail, err := s.tmdbService.GetTVDetail(req.TmdbID)
			if err == nil {
				title, enTitle = pickRenameTitlesFromDetail(detail, mediaType, title, enTitle)
				if year == 0 {
					if firstAirDate, ok := detail["first_air_date"].(string); ok && len(firstAirDate) >= 4 {
						year, _ = strconv.Atoi(firstAirDate[:4])
					}
				}
			}
		} else {
			detail, err := s.tmdbService.GetMovieDetail(req.TmdbID)
			if err == nil {
				title, enTitle = pickRenameTitlesFromDetail(detail, mediaType, title, enTitle)
				if year == 0 {
					if releaseDate, ok := detail["release_date"].(string); ok && len(releaseDate) >= 4 {
						year, _ = strconv.Atoi(releaseDate[:4])
					}
				}
			}
		}
	}

	// 只有在外部没有明确提供季集信息时，才回退到文件名解析结果。
	if season == 0 && parsed.Season > 0 {
		season = parsed.Season
	}
	if episode == 0 && parsed.Episode > 0 {
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
	newName = s.normalizeGeneratedName(newName, originalName, ext)
	// 重命名结果只允许保留文件名，避免把整理模板里的目录层级带入本地重命名
	if source.SourceType == domain.SourceTypeLocal {
		newName = filepath.Base(newName)
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
		MediaType:    mediaType,
		Season:       season,
		Episode:      episode,
		Quality:      parsed.Quality,
	}

	logger.Infof("RenameService[PreviewRename] 预览完成: %s -> %s", originalName, newName)
	return result, nil
}

func normalizeRenameMediaType(requestedMediaType string, parsed *ParsedEpisode) string {
	switch strings.ToLower(strings.TrimSpace(requestedMediaType)) {
	case "movie", "tv":
		return strings.ToLower(strings.TrimSpace(requestedMediaType))
	}

	if parsed != nil && (parsed.Season > 0 || parsed.Episode > 0) {
		return "tv"
	}
	return "movie"
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
	newName := filepath.Base(strings.TrimSpace(req.NewName))
	newPath := filepath.Join(dir, newName)

	// 如果新文件名没有扩展名，添加原扩展名
	if filepath.Ext(newName) == "" && ext != "" {
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
