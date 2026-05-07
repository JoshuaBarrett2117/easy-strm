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

type PendingMediaController struct {
	pendingDAO *dao.PendingMediaDAO
	pipeline   *service.MediaLibraryPipelineService
}

func NewPendingMediaController(pendingDAO *dao.PendingMediaDAO, pipeline *service.MediaLibraryPipelineService) *PendingMediaController {
	return &PendingMediaController{pendingDAO: pendingDAO, pipeline: pipeline}
}

func (c *PendingMediaController) List(ctx *gin.Context) {
	sourceID, _ := strconv.Atoi(ctx.Query("source_id"))
	list, err := c.pendingDAO.List(ctx.Query("status"), ctx.Query("media_type"), sourceID)
	if err != nil {
		logger.Errorf("PendingMediaController[List] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "查询待处理清单失败")
		return
	}
	SuccessResp(ctx, gin.H{
		"data":  list,
		"total": len(list),
	})
}

func (c *PendingMediaController) Create(ctx *gin.Context) {
	var req domain.PendingMediaItem
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	item, err := c.pendingDAO.Create(&req)
	if err != nil {
		logger.Errorf("PendingMediaController[Create] 创建失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, item)
}

func (c *PendingMediaController) Identify(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的待处理项ID")
		return
	}
	var req struct {
		TmdbID    int    `json:"tmdb_id"`
		Title     string `json:"title"`
		Year      int    `json:"year"`
		MediaType string `json:"media_type"`
		Season    int    `json:"season"`
		Episode   int    `json:"episode"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	item, err := c.pendingDAO.UpdateIdentify(id, req.TmdbID, req.Year, req.Season, req.Episode, req.Title, req.MediaType)
	if err != nil {
		logger.Errorf("PendingMediaController[Identify] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, item)
}

func (c *PendingMediaController) Run(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的待处理项ID")
		return
	}
	item, err := c.pendingDAO.UpdateStatus(id, "running", "", "")
	if err != nil {
		logger.Errorf("PendingMediaController[Run] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	result, err := c.pipeline.ProcessPendingItem(id)
	if err != nil {
		logger.Errorf("PendingMediaController[Run] 入库失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if refreshed, refreshErr := c.pendingDAO.GetByID(id); refreshErr == nil && refreshed != nil {
		item = refreshed
	}
	SuccessResp(ctx, gin.H{
		"message": "待处理项已重新入库",
		"data":    item,
		"task":    result,
	})
}

func (c *PendingMediaController) Ignore(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的待处理项ID")
		return
	}
	item, err := c.pendingDAO.UpdateStatus(id, "ignored", "用户忽略", "")
	if err != nil {
		logger.Errorf("PendingMediaController[Ignore] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, item)
}
