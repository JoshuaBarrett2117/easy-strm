package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
)

// MCPTool 是一个带有显式 JSON Schema 和副作用标注的 MCP 工具。
type MCPTool struct {
	Name            string
	Description     string
	InputSchema     map[string]interface{}
	ReadOnlyHint    bool
	DestructiveHint bool
	IdempotentHint  bool
	Handler         func(context.Context, map[string]json.RawMessage) (interface{}, error)
}

// MCPRegistry 管理 MCP 工具的注册、发现和分发。
type MCPRegistry struct {
	tools map[string]MCPTool
	order []string
}

// NewMCPRegistry 创建 MCP 工具注册表。
func NewMCPRegistry(tools ...MCPTool) *MCPRegistry {
	r := &MCPRegistry{tools: make(map[string]MCPTool, len(tools))}
	for _, tool := range tools {
		if tool.Name != "" && tool.Handler != nil {
			r.Register(tool)
		}
	}
	return r
}

// Register 注册一个名称唯一的工具。
func (r *MCPRegistry) Register(tool MCPTool) {
	if _, exists := r.tools[tool.Name]; !exists {
		r.order = append(r.order, tool.Name)
	}
	r.tools[tool.Name] = tool
}

// Tools 返回稳定顺序的 MCP 工具描述。
func (r *MCPRegistry) Tools() []map[string]interface{} {
	names := append([]string(nil), r.order...)
	sort.Strings(names)
	result := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		result = append(result, map[string]interface{}{
			"name": tool.Name, "description": tool.Description, "inputSchema": tool.InputSchema,
			"annotations": map[string]bool{"readOnlyHint": tool.ReadOnlyHint, "destructiveHint": tool.DestructiveHint, "idempotentHint": tool.IdempotentHint},
		})
	}
	return result
}

// Call 校验工具存在性、参数 JSON 和 destructive 操作确认，然后执行工具。
func (r *MCPRegistry) Call(ctx context.Context, name string, raw json.RawMessage) (interface{}, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	args := map[string]json.RawMessage{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if err := validateMCPArgumentKeys(tool.InputSchema, args); err != nil {
		return nil, err
	}
	if tool.DestructiveHint && !confirmationValue(args) {
		return map[string]interface{}{"confirmation_required": true, "tool": name, "message": "该工具会修改或删除数据，请传入 confirm=true 后重试"}, nil
	}
	return tool.Handler(ctx, args)
}

func validateMCPArgumentKeys(schema map[string]interface{}, args map[string]json.RawMessage) error {
	properties, _ := schema["properties"].(map[string]interface{})
	if len(properties) == 0 {
		if len(args) > 0 {
			return fmt.Errorf("unknown argument: %s", firstUnknownArgument(args))
		}
		return nil
	}
	for key := range args {
		if _, ok := properties[key]; !ok {
			return fmt.Errorf("unknown argument: %s", key)
		}
	}
	return nil
}

func firstUnknownArgument(args map[string]json.RawMessage) string {
	for key := range args {
		return key
	}
	return ""
}

func confirmationValue(args map[string]json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(args["confirm"], &value)
	return value
}

func decodeArg[T any](args map[string]json.RawMessage, key string, target *T, required bool) error {
	raw, ok := args[key]
	if !ok || len(raw) == 0 {
		if required {
			return fmt.Errorf("missing required argument: %s", key)
		}
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid argument %s: %w", key, err)
	}
	return nil
}
func requiredString(args map[string]json.RawMessage, key string) (string, error) {
	var value string
	if err := decodeArg(args, key, &value, true); err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("argument %s must not be empty", key)
	}
	return value, nil
}
func requiredInt(args map[string]json.RawMessage, key string) (int, error) {
	var v int
	return v, decodeArg(args, key, &v, true)
}
func objectSchema(properties map[string]interface{}, required ...string) map[string]interface{} {
	schema := map[string]interface{}{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
func stringProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description}
}
func intProperty(description string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": description}
}
func boolProperty(description string, defaultValue bool) map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": description, "default": defaultValue}
}

func withConfirm(properties map[string]interface{}) map[string]interface{} {
	return mergeProperties(properties, map[string]interface{}{"confirm": boolProperty("确认执行写操作", false)})
}

func mergeProperties(groups ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, group := range groups {
		for key, value := range group {
			result[key] = value
		}
	}
	return result
}

func decodeRenamePreset(args map[string]json.RawMessage, id int) (*dao.RenamePreset, error) {
	preset := &dao.RenamePreset{ID: id, Enabled: true}
	if err := decodeArg(args, "name", &preset.Name, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "media_type", &preset.MediaType, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "template", &preset.Template, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "enabled", &preset.Enabled, false); err != nil {
		return nil, err
	}
	return preset, nil
}

func decodeMediaCategory(args map[string]json.RawMessage, id int) (*domain.MediaCategory, error) {
	category := &domain.MediaCategory{ID: id, Enabled: true}
	if err := decodeArg(args, "name", &category.Name, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "media_type", &category.MediaType, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "target_path", &category.TargetPath, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "match_rules", &category.MatchRules, true); err != nil {
		return nil, err
	}
	if err := decodeArg(args, "enabled", &category.Enabled, false); err != nil {
		return nil, err
	}
	return category, nil
}

// MCPDependencies 是 MCP 暴露的最小业务依赖集合。
type MCPAuthenticator interface{ ValidateAPIKey(string) (bool, error) }

type MCPDependencies struct {
	API          MCPAuthenticator
	Tasks        *service.TaskService
	Dashboard    *service.DashboardService
	MediaSources *service.MediaSourceService
	FileManager  *service.FileManagerService
	TMDB         *service.TmdbService
	Rename       *service.RenameService
	Organize     *service.OrganizeService
	STRM         *service.StrmService
	Cloud115     *service.Cloud115Service
	Cron         *service.CronService
	Emby         *service.EmbyService
	Categories   *service.MediaCategoryService
	Settings     *service.SystemConfigService
}

// NewCoreMCPRegistry 创建核心 Easy Stream MCP 工具集合。
func NewCoreMCPRegistry(d MCPDependencies) *MCPRegistry {
	r := NewMCPRegistry()
	add := func(name, description string, schema map[string]interface{}, readOnly, destructive, idempotent bool, handler func(context.Context, map[string]json.RawMessage) (interface{}, error)) {
		if handler != nil {
			r.Register(MCPTool{Name: name, Description: description, InputSchema: schema, ReadOnlyHint: readOnly, DestructiveHint: destructive, IdempotentHint: idempotent, Handler: handler})
		}
	}
	empty := objectSchema(map[string]interface{}{})
	if d.Dashboard != nil {
		add("system_status", "获取系统 Dashboard 状态与任务概览", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			return d.Dashboard.GetDashboardStats()
		})
	}
	if d.Tasks != nil {
		add("tasks_recent", "获取最近任务列表", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) { return d.Tasks.GetUnified() })
		taskSchema := objectSchema(map[string]interface{}{"task_id": stringProperty("任务 ID")}, "task_id")
		add("task_get", "获取指定任务详情", taskSchema, true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredString(a, "task_id")
			if e != nil {
				return nil, e
			}
			return d.Tasks.Get(id)
		})
		add("task_cancel", "取消指定任务；必须显式 confirm=true", objectSchema(map[string]interface{}{"task_id": stringProperty("任务 ID"), "confirm": boolProperty("确认取消", false)}, "task_id", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredString(a, "task_id")
			if e != nil {
				return nil, e
			}
			if e = d.Tasks.Cancel(id); e != nil {
				return nil, e
			}
			return map[string]interface{}{"task_id": id, "cancelled": true}, nil
		})
		add("task_resume", "恢复指定任务；必须显式 confirm=true", objectSchema(map[string]interface{}{"task_id": stringProperty("任务 ID"), "confirm": boolProperty("确认恢复", false)}, "task_id", "confirm"), false, true, false, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredString(a, "task_id")
			if e != nil {
				return nil, e
			}
			if e = d.Tasks.Resume(id); e != nil {
				return nil, e
			}
			return map[string]interface{}{"task_id": id, "resumed": true}, nil
		})
	}
	if d.MediaSources != nil {
		add("media_sources_list", "列出媒体源（不返回凭据）", objectSchema(map[string]interface{}{"sort_field": stringProperty("排序字段"), "sort_order": map[string]interface{}{"type": "string", "enum": []string{"asc", "desc"}, "default": "asc"}}), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var sf, so string
			_ = decodeArg(a, "sort_field", &sf, false)
			_ = decodeArg(a, "sort_order", &so, false)
			if so == "" {
				so = "asc"
			}
			return d.MediaSources.GetAll(sf, so)
		})
		add("media_source_get", "获取媒体源详情（不返回凭据）", objectSchema(map[string]interface{}{"source_id": intProperty("媒体源 ID")}, "source_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredInt(a, "source_id")
			if e != nil {
				return nil, e
			}
			return d.MediaSources.GetByID(id)
		})
		add("media_source_files_browse", "分页浏览媒体源文件，可按视频和关键词筛选", objectSchema(map[string]interface{}{"source_id": intProperty("媒体源 ID"), "path": stringProperty("相对路径"), "page": intProperty("页码，默认 1"), "page_size": intProperty("页大小，默认 50"), "sort_field": stringProperty("排序字段"), "sort_order": stringProperty("排序方向"), "filter": stringProperty("文件过滤器"), "search": stringProperty("搜索词")}, "source_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var id, page, size int
			var path, sf, so, filter, search string
			e := decodeArg(a, "source_id", &id, true)
			if e != nil {
				return nil, e
			}
			_ = decodeArg(a, "path", &path, false)
			_ = decodeArg(a, "page", &page, false)
			_ = decodeArg(a, "page_size", &size, false)
			_ = decodeArg(a, "sort_field", &sf, false)
			_ = decodeArg(a, "sort_order", &so, false)
			_ = decodeArg(a, "filter", &filter, false)
			_ = decodeArg(a, "search", &search, false)
			if page == 0 {
				page = 1
			}
			if size == 0 {
				size = 50
			}
			return d.MediaSources.GetFiles(id, path, page, size, sf, so, filter, search)
		})
		add("media_source_delete", "删除媒体源；必须显式 confirm=true", objectSchema(map[string]interface{}{"source_id": intProperty("媒体源 ID"), "confirm": boolProperty("确认删除", false)}, "source_id", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredInt(a, "source_id")
			if e != nil {
				return nil, e
			}
			if e = d.MediaSources.Delete(id); e != nil {
				return nil, e
			}
			return map[string]interface{}{"source_id": id, "deleted": true}, nil
		})
	}
	if d.FileManager != nil {
		add("file_manager_locations", "列出文件管理位置", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			return d.FileManager.ListLocations()
		})
		add("file_manager_browse", "浏览文件管理位置", objectSchema(map[string]interface{}{"location_type": map[string]interface{}{"type": "string", "enum": []string{"local", "cloud115"}}, "location_id": intProperty("位置 ID"), "path": stringProperty("相对路径")}, "location_type", "location_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var typ, path string
			var id int
			e := decodeArg(a, "location_type", &typ, true)
			if e != nil {
				return nil, e
			}
			e = decodeArg(a, "location_id", &id, true)
			if e != nil {
				return nil, e
			}
			_ = decodeArg(a, "path", &path, false)
			return d.FileManager.Browse(domain.FileManagerLocationRef{Type: typ, ID: id}, path)
		})
	}
	if d.TMDB != nil {
		add("tmdb_filename_parse", "解析媒体文件名，不访问外部服务", objectSchema(map[string]interface{}{"filename": stringProperty("文件名")}, "filename"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			v, e := requiredString(a, "filename")
			if e != nil {
				return nil, e
			}
			return d.TMDB.ParseFilename(v), nil
		})
		add("tmdb_search", "搜索 TMDB 电影或剧集", objectSchema(map[string]interface{}{"query": stringProperty("关键词"), "year": intProperty("年份"), "media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}, "default": "movie"}}, "query"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var q, typ string
			var year int
			e := decodeArg(a, "query", &q, true)
			if e != nil {
				return nil, e
			}
			_ = decodeArg(a, "year", &year, false)
			_ = decodeArg(a, "media_type", &typ, false)
			if typ == "tv" {
				return d.TMDB.SearchTV(q, year)
			}
			return d.TMDB.SearchMovieBySource(q, year, "auto")
		})
		add("tmdb_detail", "获取 TMDB 详情", objectSchema(map[string]interface{}{"tmdb_id": intProperty("TMDB ID"), "media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}}}, "tmdb_id", "media_type"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var id int
			var typ string
			e := decodeArg(a, "tmdb_id", &id, true)
			if e != nil {
				return nil, e
			}
			e = decodeArg(a, "media_type", &typ, true)
			if e != nil {
				return nil, e
			}
			if typ == "tv" {
				return d.TMDB.GetTVDetail(id)
			}
			return d.TMDB.GetMovieDetail(id)
		})
		add("tmdb_identify", "根据文件名识别媒体候选", objectSchema(map[string]interface{}{"filename": stringProperty("文件名"), "media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}}}, "filename"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			v, e := requiredString(a, "filename")
			if e != nil {
				return nil, e
			}
			var typ string
			_ = decodeArg(a, "media_type", &typ, false)
			if typ == "" {
				return d.TMDB.IdentifyFile(v)
			}
			return d.TMDB.IdentifyFileWithPathBySourceAndType(v, "auto", typ)
		})
	}
	if d.Rename != nil {
		add("rename_preview", "预览文件重命名，不产生文件副作用", objectSchema(map[string]interface{}{"source_id": intProperty("媒体源 ID"), "file_id": stringProperty("文件 ID"), "tmdb_id": intProperty("TMDB ID"), "media_type": stringProperty("媒体类型"), "title": stringProperty("标题"), "year": intProperty("年份"), "season": intProperty("季"), "episode": intProperty("集"), "template": stringProperty("模板")}, "source_id", "file_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var req domain.RenamePreviewRequest
			if e := jsonArgs(a, &req); e != nil {
				return nil, e
			}
			return d.Rename.PreviewRename(&req)
		})
		add("rename_presets_list", "列出更名预设", objectSchema(map[string]interface{}{"media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}}}), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var mediaType string
			if err := decodeArg(a, "media_type", &mediaType, false); err != nil {
				return nil, err
			}
			return d.Rename.GetPresets(mediaType)
		})
		add("rename_preset_get", "获取指定更名预设", objectSchema(map[string]interface{}{"preset_id": intProperty("预设 ID")}, "preset_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, err := requiredInt(a, "preset_id")
			if err != nil {
				return nil, err
			}
			return d.Rename.GetPreset(id)
		})
		presetProperties := map[string]interface{}{
			"name": stringProperty("预设名称"), "media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}},
			"template": stringProperty("Jinja 更名模板"), "enabled": boolProperty("是否启用", true),
		}
		add("rename_preset_create", "创建更名预设并校验 Jinja 模板；必须显式 confirm=true", objectSchema(withConfirm(presetProperties), "name", "media_type", "template", "confirm"), false, true, false, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			preset, err := decodeRenamePreset(a, 0)
			if err != nil {
				return nil, err
			}
			if err = d.Rename.CreatePreset(preset); err != nil {
				return nil, err
			}
			return preset, nil
		})
		add("rename_preset_update", "更新更名预设并校验 Jinja 模板；必须显式 confirm=true", objectSchema(withConfirm(mergeProperties(map[string]interface{}{"preset_id": intProperty("预设 ID")}, presetProperties)), "preset_id", "name", "media_type", "template", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, err := requiredInt(a, "preset_id")
			if err != nil {
				return nil, err
			}
			preset, err := decodeRenamePreset(a, id)
			if err != nil {
				return nil, err
			}
			if err = d.Rename.UpdatePreset(preset); err != nil {
				return nil, err
			}
			return preset, nil
		})
		add("rename_preset_delete", "删除更名预设；必须显式 confirm=true", objectSchema(map[string]interface{}{"preset_id": intProperty("预设 ID"), "confirm": boolProperty("确认删除", false)}, "preset_id", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, err := requiredInt(a, "preset_id")
			if err != nil {
				return nil, err
			}
			if err = d.Rename.DeletePreset(id); err != nil {
				return nil, err
			}
			return map[string]interface{}{"preset_id": id, "deleted": true}, nil
		})
	}
	if d.TMDB != nil {
		add("filename_recognition_rules_get", "获取文件名识别规则", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			return d.TMDB.GetFilenameRecognitionRules(), nil
		})
		add("filename_recognition_rules_validate_samples", "使用当前规则校验文件名样例", objectSchema(map[string]interface{}{"samples": map[string]interface{}{"type": "array", "items": stringProperty("文件名")}}, "samples"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var samples []string
			if err := decodeArg(a, "samples", &samples, true); err != nil {
				return nil, err
			}
			return d.TMDB.ValidateFilenameRecognitionSamples(samples)
		})
		rulesSchema := map[string]interface{}{"rules": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "additionalProperties": false, "properties": map[string]interface{}{"id": stringProperty("规则 ID"), "name": stringProperty("规则名称"), "description": stringProperty("描述"), "pattern": stringProperty("Go 正则"), "media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}}, "default_season": intProperty("默认季"), "example": stringProperty("示例文件名"), "enabled": boolProperty("是否启用", true), "priority": intProperty("优先级")}}}}
		add("filename_recognition_rules_save", "保存并立即启用文件名识别规则；必须显式 confirm=true", objectSchema(withConfirm(rulesSchema), "rules", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var rules []service.FilenameRecognitionRule
			if err := decodeArg(a, "rules", &rules, true); err != nil {
				return nil, err
			}
			return d.TMDB.SaveFilenameRecognitionRules(rules)
		})
		add("filename_recognition_rules_reset", "恢复内置文件名识别规则；必须显式 confirm=true", objectSchema(map[string]interface{}{"confirm": boolProperty("确认重置", false)}, "confirm"), false, true, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			return d.TMDB.ResetFilenameRecognitionRules()
		})
	}
	if d.Categories != nil {
		add("media_categories_list", "列出媒体分类规则", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) { return d.Categories.GetAll() })
		categoryProperties := map[string]interface{}{"name": stringProperty("分类名称"), "media_type": map[string]interface{}{"type": "string", "enum": []string{"movie", "tv"}}, "target_path": stringProperty("目标路径"), "match_rules": map[string]interface{}{"type": "object"}, "enabled": boolProperty("是否启用", true)}
		add("media_category_create", "创建媒体分类规则；必须显式 confirm=true", objectSchema(withConfirm(categoryProperties), "name", "media_type", "target_path", "match_rules", "confirm"), false, true, false, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			category, err := decodeMediaCategory(a, 0)
			if err != nil {
				return nil, err
			}
			if err = d.Categories.Create(category); err != nil {
				return nil, err
			}
			return category, nil
		})
		add("media_category_update", "更新媒体分类规则；必须显式 confirm=true", objectSchema(withConfirm(mergeProperties(map[string]interface{}{"category_id": intProperty("分类 ID")}, categoryProperties)), "category_id", "name", "media_type", "target_path", "match_rules", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, err := requiredInt(a, "category_id")
			if err != nil {
				return nil, err
			}
			category, err := decodeMediaCategory(a, id)
			if err != nil {
				return nil, err
			}
			if err = d.Categories.Update(category); err != nil {
				return nil, err
			}
			return category, nil
		})
		add("media_category_delete", "删除媒体分类规则；必须显式 confirm=true", objectSchema(map[string]interface{}{"category_id": intProperty("分类 ID"), "confirm": boolProperty("确认删除", false)}, "category_id", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, err := requiredInt(a, "category_id")
			if err != nil {
				return nil, err
			}
			if err = d.Categories.Delete(id); err != nil {
				return nil, err
			}
			return map[string]interface{}{"category_id": id, "deleted": true}, nil
		})
	}
	if d.Settings != nil {
		for _, group := range []string{"organize", "scrape"} {
			groupName := group
			add(groupName+"_rule_settings_get", "读取安全白名单内的"+groupName+"规则设置", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
				return d.Settings.GetRuleSettings(groupName)
			})
			add(groupName+"_rule_settings_update", "更新安全白名单内的"+groupName+"规则设置；必须显式 confirm=true", objectSchema(map[string]interface{}{"settings": map[string]interface{}{"type": "object"}, "confirm": boolProperty("确认更新", false)}, "settings", "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
				var settings map[string]bool
				if err := decodeArg(a, "settings", &settings, true); err != nil {
					return nil, err
				}
				return d.Settings.UpdateRuleSettings(groupName, settings)
			})
		}
	}
	if d.Organize != nil {
		add("organize_candidates", "列出可整理文件候选", objectSchema(map[string]interface{}{"source_id": intProperty("媒体源 ID"), "source_path": stringProperty("源路径"), "media_type": stringProperty("媒体类型"), "file_ids": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}}}, "source_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var id int
			var p, t string
			var ids []string
			e := decodeArg(a, "source_id", &id, true)
			if e != nil {
				return nil, e
			}
			_ = decodeArg(a, "source_path", &p, false)
			_ = decodeArg(a, "media_type", &t, false)
			_ = decodeArg(a, "file_ids", &ids, false)
			return d.Organize.ListOrganizeCandidates(id, p, t, ids)
		})
	}
	if d.STRM != nil {
		add("strm_configs_list", "列出 STRM 配置（不返回凭据）", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			return d.STRM.GetAllConfig("id", "asc")
		})
		add("strm_config_get", "获取 STRM 配置详情", objectSchema(map[string]interface{}{"config_id": intProperty("配置 ID")}, "config_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredInt(a, "config_id")
			if e != nil {
				return nil, e
			}
			return d.STRM.GetConfigByID(id)
		})
	}
	if d.Cloud115 != nil {
		add("cloud115_accounts_list", "列出 115 账号（凭据字段由领域模型脱敏）", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			accounts, err := d.Cloud115.GetAll("priority", "asc")
			if err != nil {
				return nil, err
			}
			return sanitizeCloud115Accounts(accounts), nil
		})
		add("cloud115_account_get", "获取 115 账号详情（不返回凭据）", objectSchema(map[string]interface{}{"account_id": intProperty("账号 ID")}, "account_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredInt(a, "account_id")
			if e != nil {
				return nil, e
			}
			account, err := d.Cloud115.GetByID(id)
			if err != nil {
				return nil, err
			}
			return sanitizeCloud115Account(account), nil
		})
	}
	if d.Cron != nil {
		add("cron_tasks_list", "列出定时任务", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) { return d.Cron.GetAll() })
		add("cron_task_get", "获取定时任务详情", objectSchema(map[string]interface{}{"task_id": intProperty("定时任务 ID")}, "task_id"), true, false, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			id, e := requiredInt(a, "task_id")
			if e != nil {
				return nil, e
			}
			return d.Cron.GetByID(id)
		})
	}
	if d.Emby != nil {
		add("emby_status", "检查 Emby 连接状态", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) {
			ok, info, e := d.Emby.CheckConnection()
			return map[string]interface{}{"enabled": d.Emby.IsEnabled(), "connected": ok, "system": info}, e
		})
		add("emby_libraries", "列出 Emby 媒体库", empty, true, false, true, func(context.Context, map[string]json.RawMessage) (interface{}, error) { return d.Emby.ListLibraries() })
		add("emby_refresh", "刷新 Emby 媒体库；必须显式 confirm=true", objectSchema(map[string]interface{}{"library_id": stringProperty("媒体库 ID，可为空表示全部"), "confirm": boolProperty("确认刷新", false)}, "confirm"), false, true, true, func(_ context.Context, a map[string]json.RawMessage) (interface{}, error) {
			var id string
			_ = decodeArg(a, "library_id", &id, false)
			if id != "" {
				return d.Emby.RefreshLibrary(id)
			}
			return d.Emby.RefreshAll()
		})
	}
	return r
}

func sanitizeCloud115Accounts(accounts []*domain.Cloud115) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(accounts))
	for _, account := range accounts {
		result = append(result, sanitizeCloud115Account(account))
	}
	return result
}

func sanitizeCloud115Account(account *domain.Cloud115) map[string]interface{} {
	if account == nil {
		return nil
	}
	return map[string]interface{}{"id": account.ID, "name": account.Name, "cookie_source": account.CookieSource, "expires_in": account.ExpiresIn, "transfer_account_id": account.TransferAccountID, "transfer_directory": account.TransferDirectory, "account_type": account.AccountType, "quota_used": account.QuotaUsed, "priority": account.Priority, "status": account.Status, "cooling_start_time": account.CoolingStartTime, "transfer_method": account.TransferMethod, "alist_configured": account.AlistUrl != ""}
}

func jsonArgs[T any](args map[string]json.RawMessage, target *T) error {
	b, e := json.Marshal(args)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, target)
}
