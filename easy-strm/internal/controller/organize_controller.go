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
	// embyRefreshCallback 整理完成后的Emby刷新回调（由main包注入）
	embyRefreshCallback func(sourceID int) *service.EmbyLibraryRefreshResult
}

func NewOrganizeController(organizeService *service.OrganizeService) *OrganizeController {
	return &OrganizeController{organizeService: organizeService}
}

// SetEmbyRefreshCallback 注入Emby刷新回调
func (c *OrganizeController) SetEmbyRefreshCallback(fn func(sourceID int) *service.EmbyLibraryRefreshResult) {
	c.embyRefreshCallback = fn
}

// PreviewOrganize 预览整理结果
// ListOrganizeCandidates 列出整理候选文件
func (c *OrganizeController) ListOrganizeCandidates(ctx *gin.Context) {
	var req struct {
		SourceID   int      `json:"source_id" binding:"required"`
		SourcePath string   `json:"source_path"`
		MediaType  string   `json:"media_type"`
		FileIDs    []string `json:"file_ids"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.MediaType == "" {
		req.MediaType = "all"
	}

	candidates, err := c.organizeService.ListOrganizeCandidates(req.SourceID, req.SourcePath, req.MediaType, req.FileIDs)
	if err != nil {
		logger.Errorf("OrganizeController[ListOrganizeCandidates] 获取候选文件失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  candidates,
		"total": len(candidates),
	})
}

func (c *OrganizeController) PreviewOrganize(ctx *gin.Context) {
	var req struct {
		SourceID    int                             `json:"source_id" binding:"required"`
		SourcePath  string                          `json:"source_path"`
		TargetPath  string                          `json:"target_path"` // 可以不传，如果启用了 UseCategory
		MediaType   string                          `json:"media_type"`
		Template    string                          `json:"template"`
		FileIDs     []string                        `json:"file_ids"`
		UseCategory bool                            `json:"use_category"`
		ManualItems []domain.OrganizeManualOverride `json:"manual_items"`
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

	previews, err := c.organizeService.PreviewOrganize(req.SourceID, req.SourcePath, req.TargetPath, req.MediaType, req.Template, req.FileIDs, req.UseCategory, req.ManualItems)
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

// StartPreviewTaskAsync 启动异步预览任务
func (c *OrganizeController) StartPreviewTaskAsync(ctx *gin.Context) {
	var req struct {
		SourceID    int                             `json:"source_id" binding:"required"`
		SourcePath  string                          `json:"source_path"`
		TargetPath  string                          `json:"target_path"`
		MediaType   string                          `json:"media_type"`
		Template    string                          `json:"template"`
		FileIDs     []string                        `json:"file_ids"`
		UseCategory bool                            `json:"use_category"`
		ManualItems []domain.OrganizeManualOverride `json:"manual_items"`
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

	taskID, err := c.organizeService.StartPreviewTask(req.SourceID, req.SourcePath, req.TargetPath, req.MediaType, req.Template, req.FileIDs, req.UseCategory, req.ManualItems)
	if err != nil {
		logger.Errorf("OrganizeController[StartPreviewTaskAsync] 启动任务失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"task_id": taskID,
		"message": "任务已启动",
	})
}

// GetPreviewTaskStatus 获取预览任务状态
func (c *OrganizeController) GetPreviewTaskStatus(ctx *gin.Context) {
	taskID := ctx.Query("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "缺少task_id参数")
		return
	}

	task, err := c.organizeService.GetPreviewTaskStatus(taskID)
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		ErrorResp(ctx, http.StatusNotFound, "任务不存在")
		return
	}

	// 直接返回 task 对象，SuccessResp 会将其放入 Data 字段中
	// 这样前端 statusResponse.data.data 对应的就是 task 结构，而不是 {"data": task}
	SuccessResp(ctx, task)
}

// CheckRestorableTask 检查可恢复任务
func (c *OrganizeController) CheckRestorableTask(ctx *gin.Context) {
	var req struct {
		SourceID   int      `json:"source_id" binding:"required"`
		SourcePath string   `json:"source_path"`
		FileIDs    []string `json:"file_ids"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	task, err := c.organizeService.CheckRestorableTask(req.SourceID, req.SourcePath, req.FileIDs)
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if task == nil {
		SuccessResp(ctx, gin.H{
			"has_task": false,
		})
		return
	}

	SuccessResp(ctx, gin.H{
		"has_task": task != nil,
		"task":     task,
	})
}

// ExecuteOrganize 执行整理
func (c *OrganizeController) ExecuteOrganize(ctx *gin.Context) {
	var req struct {
		SourceID       int                             `json:"source_id" binding:"required"`
		SourcePath     string                          `json:"source_path"`
		TargetPath     string                          `json:"target_path"` // 可以不传，如果启用了 UseCategory
		MediaType      string                          `json:"media_type"`
		Template       string                          `json:"template"`
		ConflictPolicy string                          `json:"conflict_policy"`
		OperationMode  string                          `json:"operation_mode"`
		MoveFiles      *bool                           `json:"move_files"`
		FileIDs        []string                        `json:"file_ids"`
		UseCategory    bool                            `json:"use_category"`
		ManualItems    []domain.OrganizeManualOverride `json:"manual_items"`
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

	operationMode := req.OperationMode
	if operationMode == "" {
		operationMode = "move"
		if req.MoveFiles != nil && !*req.MoveFiles {
			operationMode = "copy"
		}
	}

	results, err := c.organizeService.OrganizeDirectory(req.SourceID, req.SourcePath, req.TargetPath, req.MediaType, req.Template, req.ConflictPolicy, operationMode, req.FileIDs, req.UseCategory, req.ManualItems)
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

	// 整理成功后触发 Emby 媒体库刷新（仅在有成功文件时）
	var embyRefreshResult *service.EmbyLibraryRefreshResult
	if successCount > 0 && c.embyRefreshCallback != nil {
		embyRefreshResult = c.embyRefreshCallback(req.SourceID)
		if embyRefreshResult != nil {
			if embyRefreshResult.Success {
				logger.Infof("OrganizeController[ExecuteOrganize] Emby刷新成功: %s", embyRefreshResult.Message)
			} else {
				logger.Warnf("OrganizeController[ExecuteOrganize] Emby刷新失败: %s", embyRefreshResult.Message)
			}
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
		"emby_refresh": embyRefreshResult,
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
