package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// MediaSourceController 媒体源控制器
type MediaSourceController struct {
	mediaSourceService *service.MediaSourceService
	cloud115Service    *service.Cloud115Service
	watchService       watchLifecycle
	client             service.Cloud115Client
	tmdbCacheDAO       *dao.TmdbCacheDAO
}

type watchLifecycle interface {
	StartWatching(sourceID int) error
	StopWatching(sourceID int)
	IsWatching(sourceID int) bool
}

// NewMediaSourceController 创建媒体源控制器实例
func NewMediaSourceController(mediaSourceService *service.MediaSourceService, cloud115Service *service.Cloud115Service, watchService watchLifecycle, client service.Cloud115Client) *MediaSourceController {
	return &MediaSourceController{
		mediaSourceService: mediaSourceService,
		cloud115Service:    cloud115Service,
		watchService:       watchService,
		client:             client,
		tmdbCacheDAO:       dao.NewTmdbCacheDAO(),
	}
}

// GetList 获取媒体源列表
// GET /api/media/sources
func (c *MediaSourceController) GetList(ctx *gin.Context) {
	sortField := ctx.DefaultQuery("sort_field", "priority")
	sortOrder := ctx.DefaultQuery("sort_order", "asc")

	list, err := c.mediaSourceService.GetAll(sortField, sortOrder)
	if err != nil {
		logger.Errorf("MediaSourceController[GetList] 获取列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取媒体源列表失败")
		return
	}
	for _, source := range list {
		c.populateWatchStatus(source)
	}

	SuccessResp(ctx, gin.H{
		"data":  list,
		"total": len(list),
	})
}

// GetByID 根据ID获取媒体源
// GET /api/media/sources/:id
func (c *MediaSourceController) GetByID(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	source, err := c.mediaSourceService.GetByID(id)
	if err != nil || source == nil {
		logger.Errorf("MediaSourceController[GetByID] 获取详情失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}
	c.populateWatchStatus(source)

	SuccessResp(ctx, source)
}

// Create 创建媒体源
// POST /api/media/sources
func (c *MediaSourceController) Create(ctx *gin.Context) {
	var req struct {
		Name               string `json:"name" binding:"required"`
		SourceType         string `json:"source_type" binding:"required"`
		Path               string `json:"path" binding:"required"`
		WatchPath          string `json:"watch_path"`
		Cloud115ID         *int   `json:"cloud115_id"`
		Priority           int    `json:"priority"`
		Enabled            *bool  `json:"enabled"`
		OrganizeTargetPath string `json:"organize_target_path"`
		MediaType          string `json:"media_type"`
		ConflictPolicy     string `json:"conflict_policy"`
		OperationMode      string `json:"operation_mode"`
		AutoOrganize       bool   `json:"auto_organize"`
		WatchEnabled       bool   `json:"watch_enabled"`
		WatchInterval      int    `json:"watch_interval"`
		EmbyLibraryID      string `json:"emby_library_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("MediaSourceController[Create] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 设置默认值
	if req.Priority == 0 {
		req.Priority = 10
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 验证本地路径（路径不存在时仅警告，不阻止创建）
	// 业务背景：用户可能先配置媒体源，稍后再创建目录
	if req.SourceType == domain.SourceTypeLocal {
		if err := c.mediaSourceService.ValidateLocalPath(req.Path); err != nil {
			logger.Warnf("MediaSourceController[Create] 本地路径不存在或无法访问，但仍允许创建: %s, 错误: %v", req.Path, err)
			// 不返回错误，允许创建媒体源
		}
		if req.WatchPath != "" && req.WatchPath != req.Path {
			if err := c.mediaSourceService.ValidateLocalPath(req.WatchPath); err != nil {
				ErrorResp(ctx, http.StatusBadRequest, "监控目录无效")
				return
			}
		}
	}

	source, err := c.mediaSourceService.Create(
		req.Name,
		req.SourceType,
		req.Path,
		req.WatchPath,
		req.Cloud115ID,
		req.Priority,
		enabled,
		req.OrganizeTargetPath,
		req.MediaType,
		req.ConflictPolicy,
		req.OperationMode,
		req.AutoOrganize,
		req.WatchEnabled,
		req.WatchInterval,
		req.EmbyLibraryID,
	)
	if err != nil {
		logger.Errorf("MediaSourceController[Create] 创建失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("MediaSourceController[Create] 创建媒体源成功: %s (ID: %d)", source.Name, source.ID)
	c.syncWatchState(source)
	c.populateWatchStatus(source)
	SuccessResp(ctx, gin.H{
		"message": "创建成功",
		"data":    source,
	})
}

// Update 更新媒体源
// PUT /api/media/sources/:id
// 支持部分字段更新：只需提供需要更新的字段即可
func (c *MediaSourceController) Update(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	// 更新请求体：所有字段均为可选，支持部分更新
	// 业务背景：用户可能只想更新名称或优先级，无需提供完整信息
	var req struct {
		Name               *string `json:"name"`
		SourceType         *string `json:"source_type"`
		Path               *string `json:"path"`
		WatchPath          *string `json:"watch_path"`
		Cloud115ID         *int    `json:"cloud115_id"`
		Priority           *int    `json:"priority"`
		Enabled            *bool   `json:"enabled"`
		OrganizeTargetPath *string `json:"organize_target_path"`
		MediaType          *string `json:"media_type"`
		ConflictPolicy     *string `json:"conflict_policy"`
		OperationMode      *string `json:"operation_mode"`
		AutoOrganize       *bool   `json:"auto_organize"`
		WatchEnabled       *bool   `json:"watch_enabled"`
		WatchInterval      *int    `json:"watch_interval"`
		EmbyLibraryID      *string `json:"emby_library_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("MediaSourceController[Update] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 获取现有媒体源信息，用于填充未提供的字段
	existingSource, err := c.mediaSourceService.GetByID(id)
	if err != nil || existingSource == nil {
		logger.Errorf("MediaSourceController[Update] 获取媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	// 合并更新数据：未提供的字段保留原值
	name := existingSource.Name
	if req.Name != nil {
		name = *req.Name
	}

	sourceType := existingSource.SourceType
	if req.SourceType != nil {
		sourceType = *req.SourceType
	}

	path := existingSource.Path
	if req.Path != nil {
		path = *req.Path
	}

	watchPath := existingSource.WatchPath
	if req.WatchPath != nil {
		watchPath = *req.WatchPath
	}

	cloud115ID := existingSource.Cloud115ID
	if req.Cloud115ID != nil {
		cloud115ID = req.Cloud115ID
	}

	priority := existingSource.Priority
	if req.Priority != nil {
		priority = *req.Priority
	}

	enabled := existingSource.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	organizeTargetPath := existingSource.OrganizeTargetPath
	if req.OrganizeTargetPath != nil {
		organizeTargetPath = *req.OrganizeTargetPath
	}

	mediaType := existingSource.MediaType
	if req.MediaType != nil {
		mediaType = *req.MediaType
	}

	conflictPolicy := existingSource.ConflictPolicy
	if req.ConflictPolicy != nil {
		conflictPolicy = *req.ConflictPolicy
	}

	operationMode := existingSource.OperationMode
	if req.OperationMode != nil {
		operationMode = *req.OperationMode
	}

	autoOrganize := existingSource.AutoOrganize
	if req.AutoOrganize != nil {
		autoOrganize = *req.AutoOrganize
	}

	watchEnabled := existingSource.WatchEnabled
	if req.WatchEnabled != nil {
		watchEnabled = *req.WatchEnabled
	}

	watchInterval := existingSource.WatchInterval
	if req.WatchInterval != nil {
		watchInterval = *req.WatchInterval
	}

	embyLibraryID := existingSource.EmbyLibraryID
	if req.EmbyLibraryID != nil {
		embyLibraryID = *req.EmbyLibraryID
	}

	// 验证本地路径（路径不存在时仅警告，不阻止更新）
	// 业务背景：用户可能先配置媒体源，稍后再创建目录
	if sourceType == domain.SourceTypeLocal {
		if err := c.mediaSourceService.ValidateLocalPath(path); err != nil {
			logger.Warnf("MediaSourceController[Update] 本地路径不存在或无法访问，但仍允许更新: %s, 错误: %v", path, err)
			// 不返回错误，允许更新媒体源
		}
		if watchPath != "" && watchPath != path {
			if err := c.mediaSourceService.ValidateLocalPath(watchPath); err != nil {
				ErrorResp(ctx, http.StatusBadRequest, "监控目录无效")
				return
			}
		}
	}

	source, err := c.mediaSourceService.Update(
		id,
		name,
		sourceType,
		path,
		watchPath,
		cloud115ID,
		priority,
		enabled,
		organizeTargetPath,
		mediaType,
		conflictPolicy,
		operationMode,
		autoOrganize,
		watchEnabled,
		watchInterval,
		embyLibraryID,
	)
	if err != nil {
		logger.Errorf("MediaSourceController[Update] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("MediaSourceController[Update] 更新媒体源成功: %s (ID: %d)", source.Name, id)
	c.syncWatchState(source)
	c.populateWatchStatus(source)
	SuccessResp(ctx, gin.H{
		"message": "更新成功",
		"data":    source,
	})
}

// Delete 删除媒体源
// DELETE /api/media/sources/:id
func (c *MediaSourceController) Delete(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	if err := c.mediaSourceService.Delete(id); err != nil {
		logger.Errorf("MediaSourceController[Delete] 删除失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("MediaSourceController[Delete] 删除媒体源成功: ID %d", id)
	c.stopWatchState(id)
	SuccessResp(ctx, gin.H{
		"message": "删除成功",
	})
}

func (c *MediaSourceController) syncWatchState(source *domain.MediaSource) {
	if c.watchService == nil || source == nil {
		return
	}

	c.watchService.StopWatching(source.ID)
	if source.Enabled && source.WatchEnabled {
		if err := c.watchService.StartWatching(source.ID); err != nil {
			logger.Warnf("MediaSourceController[syncWatchState] 启动监控失败: source_id=%d, error=%v", source.ID, err)
		}
	}
}

func (c *MediaSourceController) stopWatchState(sourceID int) {
	if c.watchService == nil {
		return
	}
	c.watchService.StopWatching(sourceID)
}

func (c *MediaSourceController) populateWatchStatus(source *domain.MediaSource) {
	if source == nil || c.watchService == nil {
		return
	}
	source.WatchRunning = c.watchService.IsWatching(source.ID)
}
