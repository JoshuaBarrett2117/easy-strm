package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// RenameController 更名控制器
type RenameController struct {
	renameService *service.RenameService
}

// NewRenameController 创建更名控制器实例
// 参数:
//   - renameService: 更名服务
// 返回:
//   - *RenameController: 更名控制器实例
func NewRenameController(renameService *service.RenameService) *RenameController {
	return &RenameController{
		renameService: renameService,
	}
}

// Preview 更名预览
// POST /api/media/rename/preview
func (c *RenameController) Preview(ctx *gin.Context) {
	var req domain.RenamePreviewRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	result, err := c.renameService.PreviewRename(&req)
	if err != nil {
		logger.Errorf("RenameController[Preview] 预览失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("RenameController[Preview] 预览完成: %s -> %s", result.OriginalName, result.NewName)
	SuccessResp(ctx, result)
}

// Execute 执行更名
// POST /api/media/rename/execute
func (c *RenameController) Execute(ctx *gin.Context) {
	var req domain.RenameExecuteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	result, err := c.renameService.ExecuteRename(&req)
	if err != nil {
		logger.Errorf("RenameController[Execute] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("RenameController[Execute] 执行完成: %s -> %s", result.OriginalPath, result.NewPath)
	SuccessResp(ctx, result)
}

// GetPresets 获取更名预设列表
// GET /api/media/rename/presets
func (c *RenameController) GetPresets(ctx *gin.Context) {
	mediaType := ctx.Query("media_type")

	presets, err := c.renameService.GetPresets(mediaType)
	if err != nil {
		logger.Errorf("RenameController[GetPresets] 获取预设失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取预设失败")
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  presets,
		"total": len(presets),
	})
}

// BatchPreview 批量更名预览
// POST /api/media/rename/batch-preview
func (c *RenameController) BatchPreview(ctx *gin.Context) {
	var req struct {
		Items []domain.RenamePreviewRequest `json:"items" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	if len(req.Items) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "预览列表不能为空")
		return
	}

	results := make([]*domain.RenamePreviewResult, 0, len(req.Items))
	for _, item := range req.Items {
		result, err := c.renameService.PreviewRename(&item)
		if err != nil {
			logger.Warnf("RenameController[BatchPreview] 预览失败: %v", err)
			results = append(results, &domain.RenamePreviewResult{
				OriginalName: item.FileID,
				NewName:      "",
			})
		} else {
			results = append(results, result)
		}
	}

	logger.Infof("RenameController[BatchPreview] 批量预览完成: total=%d", len(results))
	SuccessResp(ctx, gin.H{
		"data":  results,
		"total": len(results),
	})
}

// BatchExecute 批量执行更名
// POST /api/media/rename/batch-execute
func (c *RenameController) BatchExecute(ctx *gin.Context) {
	var req struct {
		Items []domain.RenameExecuteRequest `json:"items" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	if len(req.Items) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "执行列表不能为空")
		return
	}

	results := make([]*domain.RenameExecuteResult, 0, len(req.Items))
	successCount := 0
	failedCount := 0

	for _, item := range req.Items {
		result, err := c.renameService.ExecuteRename(&item)
		if err != nil {
			logger.Warnf("RenameController[BatchExecute] 执行失败: %v", err)
			results = append(results, &domain.RenameExecuteResult{
				Success: false,
				Message: err.Error(),
			})
			failedCount++
		} else {
			results = append(results, result)
			successCount++
		}
	}

	logger.Infof("RenameController[BatchExecute] 批量执行完成: success=%d, failed=%d", successCount, failedCount)
	SuccessResp(ctx, gin.H{
		"data":    results,
		"total":   len(results),
		"success": successCount,
		"failed":  failedCount,
	})
}
