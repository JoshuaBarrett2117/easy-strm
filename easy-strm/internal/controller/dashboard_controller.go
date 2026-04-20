package controller

import (
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
