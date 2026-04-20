package controller

import (
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// EmbyController Emby 集成控制器
// 负责处理 Emby 相关的 API 请求
type EmbyController struct {
	embyService *service.EmbyService
}

// NewEmbyController 创建 Emby 控制器实例
func NewEmbyController(embyService *service.EmbyService) *EmbyController {
	return &EmbyController{
		embyService: embyService,
	}
}

// GetStatus 检查 Emby 连接状态
// Route: GET /emby/status
func (c *EmbyController) GetStatus(ctx *gin.Context) {
	logger.Debugf("EmbyController[GetStatus] 检查Emby连接状态 from %s", ctx.ClientIP())

	connected, info, err := c.embyService.CheckConnection()
	if err != nil {
		SuccessResp(ctx, gin.H{
			"connected": false,
			"enabled":   c.embyService.IsEnabled(),
			"error":     err.Error(),
		})
		return
	}

	SuccessResp(ctx, gin.H{
		"connected": connected,
		"enabled":   c.embyService.IsEnabled(),
		"info":      info,
	})
}

// GetLibraries 获取 Emby 媒体库列表
// Route: GET /emby/libraries
func (c *EmbyController) GetLibraries(ctx *gin.Context) {
	logger.Debugf("EmbyController[GetLibraries] 获取Emby媒体库列表 from %s", ctx.ClientIP())

	libraries, err := c.embyService.ListLibraries()
	if err != nil {
		logger.Errorf("EmbyController[GetLibraries] 获取媒体库列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  libraries,
		"total": len(libraries),
	})
}

// Refresh 刷新 Emby 媒体库
// Route: POST /emby/refresh
// 支持刷新指定库或全部库
func (c *EmbyController) Refresh(ctx *gin.Context) {
	var req struct {
		LibraryID string `json:"library_id"` // 可选，为空时刷新全部
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		// 允许空 body，默认刷新全部
		req.LibraryID = ""
	}

	logger.Infof("EmbyController[Refresh] 刷新Emby媒体库: library_id=%s from %s", req.LibraryID, ctx.ClientIP())

	if req.LibraryID != "" {
		// 刷新指定媒体库
		result, err := c.embyService.RefreshLibrary(req.LibraryID)
		if err != nil {
			logger.Errorf("EmbyController[Refresh] 刷新媒体库失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		SuccessResp(ctx, result)
		return
	}

	// 刷新全部媒体库
	results, err := c.embyService.RefreshAll()
	if err != nil {
		logger.Errorf("EmbyController[Refresh] 刷新全部媒体库失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  results,
		"total": len(results),
	})
}
