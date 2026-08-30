package controller

import (
	"net/http"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// EmbyMonitorController 提供 Emby 观影统计只读接口。
type embyMonitorService interface {
	GetOverview(int) (*domain.EmbyMonitorOverview, error)
	GetRankings(int, string, string, string) (*domain.EmbyMonitorRankingResponse, error)
	GetHeatmap(int, string, string) (*domain.EmbyMonitorHeatmapResponse, error)
	GetRecentItems(int, string, string, string) (*domain.EmbyMonitorRecentResponse, error)
	GetItemImage(int, string) ([]byte, string, error)
}

type EmbyMonitorController struct{ service embyMonitorService }

// NewEmbyMonitorController 创建 Emby 观影监控控制器。
func NewEmbyMonitorController(monitorService *service.EmbyMonitorService) *EmbyMonitorController {
	return &EmbyMonitorController{service: monitorService}
}

func (c *EmbyMonitorController) serverID(ctx *gin.Context) (int, bool) {
	id, err := strconv.Atoi(ctx.Param("server_id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的 Emby 实例 ID")
		return 0, false
	}
	return id, true
}

// GetOverview 获取监控概览。
func (c *EmbyMonitorController) GetOverview(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	data, err := c.service.GetOverview(id)
	if err != nil {
		ErrorResp(ctx, http.StatusBadGateway, err.Error())
		return
	}
	SuccessResp(ctx, data)
}

// GetRankings 获取用户、媒体或客户端排行。
func (c *EmbyMonitorController) GetRankings(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	dimension := strings.TrimSpace(ctx.Param("dimension"))
	period := strings.TrimSpace(ctx.DefaultQuery("period", "day"))
	mediaType := strings.TrimSpace(ctx.DefaultQuery("media_type", "movie"))
	data, err := c.service.GetRankings(id, dimension, period, mediaType)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, data)
}

// GetHeatmap 获取用户活跃热力图。
func (c *EmbyMonitorController) GetHeatmap(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	rangeName := strings.TrimSpace(ctx.DefaultQuery("range", "7d"))
	userID := strings.TrimSpace(ctx.Query("user_id"))
	data, err := c.service.GetHeatmap(id, rangeName, userID)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, data)
}

// GetRecentItems 获取最近入库媒体。
func (c *EmbyMonitorController) GetRecentItems(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	timeBasis := strings.TrimSpace(ctx.DefaultQuery("time_basis", "emby"))
	rangeName := strings.TrimSpace(ctx.DefaultQuery("range", "today"))
	mediaType := strings.TrimSpace(ctx.DefaultQuery("media_type", "movie"))
	data, err := c.service.GetRecentItems(id, timeBasis, rangeName, mediaType)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, data)
}

// GetItemImage 代理返回 Emby 媒体主图。
func (c *EmbyMonitorController) GetItemImage(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	data, contentType, err := c.service.GetItemImage(id, ctx.Param("item_id"))
	if err != nil {
		ErrorResp(ctx, http.StatusNotFound, err.Error())
		return
	}
	ctx.Data(http.StatusOK, contentType, data)
}
