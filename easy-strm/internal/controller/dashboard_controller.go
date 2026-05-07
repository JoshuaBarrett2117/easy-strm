package controller

import (
	"fmt"
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// DashboardController Dashboard 数据概览控制器
type DashboardController struct {
	dashboardService *service.DashboardService
}

// NewDashboardController 创建 Dashboard 控制器实例
func NewDashboardController(dashboardService *service.DashboardService) *DashboardController {
	return &DashboardController{
		dashboardService: dashboardService,
	}
}

// GetStats 获取 Dashboard 统计数据
// Route: GET /api/dashboard/stats
// 聚合返回账号、媒体源、STRM文件、任务、存储等维度的统计数据
func (c *DashboardController) GetStats(ctx *gin.Context) {
	logger.Debugf("DashboardController[GetStats] 获取Dashboard统计数据 from %s", ctx.ClientIP())

	stats, err := c.dashboardService.GetDashboardStats()
	if err != nil {
		logger.Errorf("DashboardController[GetStats] 获取统计数据失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取统计数据失败")
		return
	}

	SuccessResp(ctx, stats)
}

// GetOverview 获取 Dashboard 首页总览
// Route: GET /api/dashboard/overview
func (c *DashboardController) GetOverview(ctx *gin.Context) {
	logger.Debugf("DashboardController[GetOverview] 获取Dashboard总览 from %s", ctx.ClientIP())

	overview, err := c.dashboardService.GetDashboardOverview()
	if err != nil {
		logger.Errorf("DashboardController[GetOverview] 获取总览失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取总览数据失败")
		return
	}

	SuccessResp(ctx, overview)
}

// GetResourceMonitor 获取 Dashboard 资源监控
// Route: GET /api/dashboard/resource-monitor
func (c *DashboardController) GetResourceMonitor(ctx *gin.Context) {
	logger.Debugf("DashboardController[GetResourceMonitor] 获取Dashboard资源监控 from %s", ctx.ClientIP())

	monitor, err := c.dashboardService.GetDashboardResourceMonitor()
	if err != nil {
		logger.Errorf("DashboardController[GetResourceMonitor] 获取资源监控失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取资源监控失败")
		return
	}

	SuccessResp(ctx, monitor)
}

// GetTrend 获取 Dashboard 趋势数据
// Route: GET /api/dashboard/trends/:kind?days=7
func (c *DashboardController) GetTrend(ctx *gin.Context) {
	kind := ctx.Param("kind")
	days := 7
	if value := ctx.Query("days"); value != "" {
		if _, err := fmt.Sscanf(value, "%d", &days); err != nil {
			days = 7
		}
	}

	logger.Debugf("DashboardController[GetTrend] 获取Dashboard趋势 kind=%s days=%d from %s", kind, days, ctx.ClientIP())

	trend, err := c.dashboardService.GetDashboardTrend(kind, days)
	if err != nil {
		logger.Errorf("DashboardController[GetTrend] 获取趋势失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取趋势数据失败")
		return
	}

	SuccessResp(ctx, trend)
}
