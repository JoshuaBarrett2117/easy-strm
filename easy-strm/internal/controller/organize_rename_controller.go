package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// BatchIdentify 批量识别文件
func (c *OrganizeController) BatchIdentify(ctx *gin.Context) {
	var req struct {
		SourceID int      `json:"source_id" binding:"required"`
		FileIDs  []string `json:"file_ids" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if len(req.FileIDs) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "文件列表不能为空")
		return
	}

	results, err := c.organizeService.BatchIdentify(req.SourceID, req.FileIDs)
	if err != nil {
		logger.Errorf("OrganizeController[BatchIdentify] 批量识别失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{"data": results, "total": len(results)})
}

// BatchIdentifyDirectory 递归扫描目录后批量识别视频文件
func (c *OrganizeController) BatchIdentifyDirectory(ctx *gin.Context) {
	var req struct {
		SourceID   int      `json:"source_id" binding:"required"`
		SourcePath string   `json:"source_path"`
		FileIDs    []string `json:"file_ids"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	results, err := c.organizeService.BatchIdentifyDirectory(req.SourceID, req.SourcePath, req.FileIDs)
	if err != nil {
		logger.Errorf("OrganizeController[BatchIdentifyDirectory] 批量识别失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	successCount := 0
	failedCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	SuccessResp(ctx, gin.H{
		"data":    results,
		"total":   len(results),
		"success": successCount,
		"failed":  failedCount,
	})
}

// BatchRenamePreview 批量更名预览
func (c *OrganizeController) BatchRenamePreview(ctx *gin.Context) {
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

	results, err := c.organizeService.BatchRenamePreview(req.Items)
	if err != nil {
		logger.Errorf("OrganizeController[BatchRenamePreview] 批量预览失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{"data": results, "total": len(results)})
}

// BatchRenameExecute 批量执行更名
func (c *OrganizeController) BatchRenameExecute(ctx *gin.Context) {
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

	results, err := c.organizeService.BatchRenameExecute(req.Items)
	if err != nil {
		logger.Errorf("OrganizeController[BatchRenameExecute] 批量执行失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	successCount := 0
	failedCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	SuccessResp(ctx, gin.H{
		"data":    results,
		"total":   len(results),
		"success": successCount,
		"failed":  failedCount,
	})
}

func (c *OrganizeController) GetRenamePresets(ctx *gin.Context) {
	mediaType := ctx.Query("media_type")
	presets, err := c.organizeService.GetRenamePresets(mediaType)
	if err != nil {
		logger.Errorf("OrganizeController[GetRenamePresets] 获取预设失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取预设失败")
		return
	}

	SuccessResp(ctx, gin.H{"data": presets, "total": len(presets)})
}

func (c *OrganizeController) IdentifyFile(ctx *gin.Context) {
	var req struct {
		SourceID int    `json:"source_id" binding:"required"`
		FileID   string `json:"file_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	results, err := c.organizeService.BatchIdentify(req.SourceID, []string{req.FileID})
	if err != nil {
		logger.Errorf("OrganizeController[IdentifyFile] 识别失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if len(results) == 0 {
		ErrorResp(ctx, http.StatusNotFound, "识别失败")
		return
	}

	SuccessResp(ctx, results[0])
}

func (c *OrganizeController) PreviewRename(ctx *gin.Context) {
	var req domain.RenamePreviewRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	results, err := c.organizeService.BatchRenamePreview([]domain.RenamePreviewRequest{req})
	if err != nil {
		logger.Errorf("OrganizeController[PreviewRename] 预览失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if len(results) == 0 {
		ErrorResp(ctx, http.StatusNotFound, "预览失败")
		return
	}

	SuccessResp(ctx, results[0])
}

func (c *OrganizeController) ExecuteRename(ctx *gin.Context) {
	var req domain.RenameExecuteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	results, err := c.organizeService.BatchRenameExecute([]domain.RenameExecuteRequest{req})
	if err != nil {
		logger.Errorf("OrganizeController[ExecuteRename] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if len(results) == 0 {
		ErrorResp(ctx, http.StatusNotFound, "执行失败")
		return
	}

	SuccessResp(ctx, results[0])
}

func (c *OrganizeController) SearchTmdb(ctx *gin.Context) {
	query := ctx.Query("query")
	if query == "" {
		ErrorResp(ctx, http.StatusBadRequest, "搜索关键词不能为空")
		return
	}

	ErrorResp(ctx, http.StatusNotImplemented, "请使用 /api/media/tmdb/search 接口")
}
