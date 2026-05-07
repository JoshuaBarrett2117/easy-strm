package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

type MediaSyncController struct {
	mediaSyncService *service.MediaSyncService
	pipeline         *service.MediaLibraryPipelineService
}

func NewMediaSyncController(mediaSyncService *service.MediaSyncService, pipeline *service.MediaLibraryPipelineService) *MediaSyncController {
	return &MediaSyncController{mediaSyncService: mediaSyncService, pipeline: pipeline}
}

func (c *MediaSyncController) RunFullSync(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || sourceID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}
	result, err := c.mediaSyncService.RunFullSync(sourceID)
	if err != nil {
		logger.Errorf("MediaSyncController[RunFullSync] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

func (c *MediaSyncController) RunIncrementalSync(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || sourceID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}
	result, err := c.mediaSyncService.RunIncrementalSync(sourceID)
	if err != nil {
		logger.Errorf("MediaSyncController[RunIncrementalSync] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

func (c *MediaSyncController) GetIndex(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || sourceID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}
	status := ctx.Query("status")
	list, err := c.mediaSyncService.ListIndex(sourceID, status)
	if err != nil {
		logger.Errorf("MediaSyncController[GetIndex] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "查询同步索引失败")
		return
	}
	SuccessResp(ctx, gin.H{
		"data":  list,
		"total": len(list),
	})
}

func (c *MediaSyncController) RunPipeline(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || sourceID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}
	result, err := c.pipeline.ProcessSource(sourceID)
	if err != nil {
		logger.Errorf("MediaSyncController[RunPipeline] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}
