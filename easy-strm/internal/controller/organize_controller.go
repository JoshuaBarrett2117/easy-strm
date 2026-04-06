package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// OrganizeController 自动整理控制器
type OrganizeController struct {
	organizeService *service.OrganizeService
}

func NewOrganizeController(organizeService *service.OrganizeService) *OrganizeController {
	return &OrganizeController{organizeService: organizeService}
}

// PreviewOrganize 预览整理结果
func (c *OrganizeController) PreviewOrganize(ctx *gin.Context) {
	var req struct {
		SourceID    int      `json:"source_id" binding:"required"`
		SourcePath  string   `json:"source_path"`
		TargetPath  string   `json:"target_path"` // 可以不传，如果启用了 UseCategory
		MediaType   string   `json:"media_type"`
		Template    string   `json:"template"`
		FileIDs     []string `json:"file_ids"`
		UseCategory bool     `json:"use_category"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.MediaType == "" {
		req.MediaType = "all"
	}

	if !req.UseCategory && req.TargetPath == "" {
		ErrorResp(ctx, http.StatusBadRequest, "使用非分类整理时必须提供目标目录")
		return
	}

	previews, err := c.organizeService.PreviewOrganize(req.SourceID, req.SourcePath, req.TargetPath, req.MediaType, req.Template, req.FileIDs, req.UseCategory)
	if err != nil {
		logger.Errorf("OrganizeController[PreviewOrganize] 预览失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	conflictCount := 0
	failedCount := 0
	for _, preview := range previews {
		if preview.Conflict {
			conflictCount++
		}
		if preview.IdentifyError != "" {
			failedCount++
		}
	}

	SuccessResp(ctx, gin.H{
		"data":  previews,
		"total": len(previews),
		"summary": gin.H{
			"total":       len(previews),
			"conflicts":   conflictCount,
			"failed":      failedCount,
			"processable": len(previews) - failedCount,
		},
	})
}

// ExecuteOrganize 执行整理
func (c *OrganizeController) ExecuteOrganize(ctx *gin.Context) {
	var req struct {
		SourceID       int      `json:"source_id" binding:"required"`
		SourcePath     string   `json:"source_path"`
		TargetPath     string   `json:"target_path"` // 可以不传，如果启用了 UseCategory
		MediaType      string   `json:"media_type"`
		Template       string   `json:"template"`
		ConflictPolicy string   `json:"conflict_policy"`
		MoveFiles      *bool    `json:"move_files"`
		FileIDs        []string `json:"file_ids"`
		UseCategory    bool     `json:"use_category"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.MediaType == "" {
		req.MediaType = "all"
	}
	if req.ConflictPolicy == "" {
		req.ConflictPolicy = "skip"
	}

	if !req.UseCategory && req.TargetPath == "" {
		ErrorResp(ctx, http.StatusBadRequest, "使用非分类整理时必须提供目标目录")
		return
	}

	moveFiles := true
	if req.MoveFiles != nil {
		moveFiles = *req.MoveFiles
	}

	results, err := c.organizeService.OrganizeDirectory(req.SourceID, req.SourcePath, req.TargetPath, req.MediaType, req.Template, req.ConflictPolicy, moveFiles, req.FileIDs, req.UseCategory)
	if err != nil {
		logger.Errorf("OrganizeController[ExecuteOrganize] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	successCount := 0
	skippedCount := 0
	failedCount := 0
	for _, result := range results {
		if result.Skipped {
			skippedCount++
		} else if result.Success {
			successCount++
		} else {
			failedCount++
		}
	}

	SuccessResp(ctx, gin.H{
		"data":    results,
		"total":   len(results),
		"success": successCount,
		"skipped": skippedCount,
		"failed":  failedCount,
		"summary": gin.H{
			"total":   len(results),
			"success": successCount,
			"skipped": skippedCount,
			"failed":  failedCount,
		},
	})
}

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

func lastIndexOf(s string, substr string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func (c *OrganizeController) SearchTmdb(ctx *gin.Context) {
	query := ctx.Query("query")
	if query == "" {
		ErrorResp(ctx, http.StatusBadRequest, "搜索关键词不能为空")
		return
	}

	ErrorResp(ctx, http.StatusNotImplemented, "请使用 /api/media/tmdb/search 接口")
}
