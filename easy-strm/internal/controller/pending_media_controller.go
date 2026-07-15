package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

type PendingMediaController struct {
	pendingMediaService *service.PendingMediaService
}

func NewPendingMediaController(pendingMediaService *service.PendingMediaService) *PendingMediaController {
	return &PendingMediaController{pendingMediaService: pendingMediaService}
}

func (c *PendingMediaController) List(ctx *gin.Context) {
	sourceID, _ := strconv.Atoi(ctx.Query("source_id"))
	list, err := c.pendingMediaService.List(ctx.Query("status"), ctx.Query("media_type"), sourceID)
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
	item, err := c.pendingMediaService.Create(&req)
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
	item, err := c.pendingMediaService.Identify(id, req.TmdbID, req.Year, req.Season, req.Episode, req.Title, req.MediaType)
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
	result, err := c.pendingMediaService.Run(id)
	if err != nil {
		logger.Errorf("PendingMediaController[Run] 入库失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{
		"message": "待处理项已重新入库",
		"data":    result.Item,
		"task":    result.Task,
	})
}

func (c *PendingMediaController) Ignore(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的待处理项ID")
		return
	}
	item, err := c.pendingMediaService.Ignore(id)
	if err != nil {
		logger.Errorf("PendingMediaController[Ignore] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, item)
}
