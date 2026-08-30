package service

import (
	"strings"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"

	"easy-strm/internal/dao"
)

// GetPresets 获取更名预设列表。
func (s *RenameService) GetPresets(mediaType string) ([]*dao.RenamePreset, error) {
	if mediaType != "" {
		return s.renamePresetDAO.GetByMediaType(mediaType)
	}
	return s.renamePresetDAO.GetAll()
}

// getDefaultTemplate 获取默认模板。
func (s *RenameService) getDefaultTemplate(mediaType string) string {
	configKey := "movie_naming_template"
	if mediaType == "tv" {
		configKey = "tv_naming_template"
	}

	if s.systemConfigDAO != nil {
		if config, err := s.systemConfigDAO.GetByKey(configKey); err == nil && config != nil && config.ConfigVal != "" {
			return s.normalizeBuiltinTemplate(config.ConfigVal, mediaType)
		}
	}

	if mediaType == "tv" {
		return defaultTVTemplate
	}
	return defaultMovieTemplate
}

func (s *RenameService) normalizeBuiltinTemplate(template, mediaType string) string {
	switch mediaType {
	case "tv":
		if template == legacyDefaultTVTemplate || template == previousDefaultTVTemplate {
			return defaultTVTemplate
		}
	default:
		if template == legacyDefaultMovieTemplate || template == previousDefaultMovieTemplate {
			return defaultMovieTemplate
		}
	}
	return template
}

// applyTemplate 应用模板生成文件名。
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
