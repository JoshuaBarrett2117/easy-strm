package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

type MediaLibraryController struct {
	mediaLibraryService *service.MediaLibraryService
	pipeline            *service.MediaLibraryPipelineService
}

func NewMediaLibraryController(mediaLibraryService *service.MediaLibraryService, pipeline *service.MediaLibraryPipelineService) *MediaLibraryController {
	return &MediaLibraryController{mediaLibraryService: mediaLibraryService, pipeline: pipeline}
}

func (c *MediaLibraryController) ListItems(ctx *gin.Context) {
	sourceID, _ := strconv.Atoi(ctx.Query("source_id"))
	if sourceID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "请先选择媒体源")
		return
	}
	status := ctx.Query("status")
	items, err := c.mediaLibraryService.ListItems(sourceID, status)
	if err != nil {
		logger.Errorf("MediaLibraryController[ListItems] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "查询媒体库失败")
		return
	}
	SuccessResp(ctx, gin.H{
		"data":  items,
		"total": len(items),
	})
}

func (c *MediaLibraryController) GetItem(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体库条目ID")
		return
	}
	item, err := c.mediaLibraryService.GetItem(id)
	if err != nil {
		logger.Errorf("MediaLibraryController[GetItem] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "查询媒体库条目失败")
		return
	}
	if item == nil {
		ErrorResp(ctx, http.StatusNotFound, "媒体库条目不存在")
		return
	}
	SuccessResp(ctx, item)
}

func (c *MediaLibraryController) RunPipeline(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体库条目ID")
		return
	}
	result, err := c.pipeline.ProcessItem(id)
	if err != nil {
		logger.Errorf("MediaLibraryController[RunPipeline] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

func (c *MediaLibraryController) GenerateStrm(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体库条目ID")
		return
	}
	result, err := c.pipeline.GenerateStrmForItemTask(id)
	if err != nil {
		logger.Errorf("MediaLibraryController[GenerateStrm] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

func (c *MediaLibraryController) RefreshServer(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体库条目ID")
		return
	}
	result, err := c.pipeline.RefreshMediaServerForItemTask(id)
	if err != nil {
		logger.Errorf("MediaLibraryController[RefreshServer] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}
