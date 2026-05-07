package controller

import (
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// CacheAdminController 缓存管理控制器。
type CacheAdminController struct {
	cacheAdminService *service.CacheAdminService
}

// NewCacheAdminController 创建缓存管理控制器实例。
func NewCacheAdminController(cacheAdminService *service.CacheAdminService) *CacheAdminController {
	return &CacheAdminController{
		cacheAdminService: cacheAdminService,
	}
}

// GetOverview 获取缓存总览。
func (c *CacheAdminController) GetOverview(ctx *gin.Context) {
	logger.Debug("CacheAdminController[GetOverview] 获取缓存总览")

	overview, err := c.cacheAdminService.GetOverview()
	if err != nil {
		logger.Errorf("CacheAdminController[GetOverview] 获取失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"data": overview})
}

// Clear 清理指定缓存。
func (c *CacheAdminController) Clear(ctx *gin.Context) {
	logger.Debug("CacheAdminController[Clear] 清理缓存")

	var req struct {
		Scope string `json:"scope"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	deleted, err := c.cacheAdminService.ClearGroup(req.Scope)
	if err != nil {
		logger.Errorf("CacheAdminController[Clear] 清理失败: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "缓存清理完成",
		"data": gin.H{
			"scope":         req.Scope,
			"deleted_count": deleted,
		},
	})
}
