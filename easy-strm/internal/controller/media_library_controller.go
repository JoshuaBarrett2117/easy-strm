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

type MediaLibraryController struct {
	indexDAO *dao.MediaSyncIndexDAO
	pipeline *service.MediaLibraryPipelineService
}

func NewMediaLibraryController(indexDAO *dao.MediaSyncIndexDAO, pipeline *service.MediaLibraryPipelineService) *MediaLibraryController {
	return &MediaLibraryController{indexDAO: indexDAO, pipeline: pipeline}
}

func (c *MediaLibraryController) ListItems(ctx *gin.Context) {
	sourceID, _ := strconv.Atoi(ctx.Query("source_id"))
	if sourceID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "请先选择媒体源")
		return
	}
	status := ctx.Query("status")
	indexes, err := c.indexDAO.ListBySource(sourceID, status)
	if err != nil {
		logger.Errorf("MediaLibraryController[ListItems] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "查询媒体库失败")
		return
	}
	items := make([]domain.MediaLibraryItem, 0, len(indexes))
	for _, index := range indexes {
		item := domain.MediaLibraryItem{
			MediaSyncIndex: *index,
			HasStrm:        index.StrmPath != "",
			HasMetadata:    index.MetadataPath != "",
			HealthStatus:   resolveHealthStatus(index),
			LatestTaskID:   index.LastTaskID,
		}
		items = append(items, item)
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
	index, err := c.indexDAO.GetByID(id)
	if err != nil {
		logger.Errorf("MediaLibraryController[GetItem] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "查询媒体库条目失败")
		return
	}
	if index == nil {
		ErrorResp(ctx, http.StatusNotFound, "媒体库条目不存在")
		return
	}
	item := domain.MediaLibraryItem{
		MediaSyncIndex: *index,
		HasStrm:        index.StrmPath != "",
		HasMetadata:    index.MetadataPath != "",
		HealthStatus:   resolveHealthStatus(index),
		LatestTaskID:   index.LastTaskID,
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

func resolveHealthStatus(index *domain.MediaSyncIndex) string {
	if index.SyncStatus == domain.SyncStatusMissing || index.SyncStatus == domain.SyncStatusDeleted {
		return "missing"
	}
	if index.IdentityStatus == domain.IdentityStatusFailed {
		return "identify_failed"
	}
	if index.StrmPath == "" && index.SourceType == domain.SourceTypeCloud115 {
		return "strm_missing"
	}
	return "ok"
}
