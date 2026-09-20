package service

import (
	"fmt"
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

// GetPreset 获取指定更名预设。
func (s *RenameService) GetPreset(id int) (*dao.RenamePreset, error) {
	if id <= 0 {
		return nil, fmt.Errorf("预设 ID 必须为正数")
	}
	return s.renamePresetDAO.GetByID(id)
}

// CreatePreset 创建并校验更名预设。
func (s *RenameService) CreatePreset(preset *dao.RenamePreset) error {
	if err := validateRenamePreset(preset); err != nil {
		return err
	}
	return s.renamePresetDAO.Create(preset)
}

// UpdatePreset 更新并校验更名预设。
func (s *RenameService) UpdatePreset(preset *dao.RenamePreset) error {
	if err := validateRenamePreset(preset); err != nil {
		return err
	}
	if preset.ID <= 0 {
		return fmt.Errorf("预设 ID 必须为正数")
	}
	return s.renamePresetDAO.Update(preset)
}

// DeletePreset 删除指定更名预设。
func (s *RenameService) DeletePreset(id int) error {
	if id <= 0 {
		return fmt.Errorf("预设 ID 必须为正数")
	}
	return s.renamePresetDAO.Delete(id)
}

// ValidateTemplate 校验 Jinja 更名模板语法。
func (s *RenameService) ValidateTemplate(template string) error {
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("模板不能为空")
	}
	env := minijinja.NewEnvironment()
	if _, err := env.TemplateFromNamedString("rename-template", template); err != nil {
		return fmt.Errorf("Jinja 模板无效: %v", err)
	}
	return nil
}

func validateRenamePreset(preset *dao.RenamePreset) error {
	if preset == nil {
		return fmt.Errorf("预设不能为空")
	}
	if strings.TrimSpace(preset.Name) == "" {
		return fmt.Errorf("预设名称不能为空")
	}
	if preset.MediaType != "movie" && preset.MediaType != "tv" {
		return fmt.Errorf("媒体类型必须为 movie 或 tv")
	}
	env := minijinja.NewEnvironment()
	if _, err := env.TemplateFromNamedString("rename-template", preset.Template); err != nil {
		return fmt.Errorf("Jinja 模板无效: %v", err)
	}
	return nil
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
