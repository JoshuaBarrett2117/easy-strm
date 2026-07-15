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
	taskService     *service.TaskService
	// embyRefreshCallback 整理完成后的Emby刷新回调（由main包注入）
	embyRefreshCallback func(sourceID int) *service.EmbyLibraryRefreshResult
}

func NewOrganizeController(organizeService *service.OrganizeService) *OrganizeController {
	return &OrganizeController{organizeService: organizeService}
}

// SetTaskService 注入任务服务，用于异步整理执行进度上报。
func (c *OrganizeController) SetTaskService(taskService *service.TaskService) {
	c.taskService = taskService
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

// StartCandidateTaskAsync 启动异步候选扫描任务
func (c *OrganizeController) StartCandidateTaskAsync(ctx *gin.Context) {
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

	taskID, err := c.organizeService.StartCandidateTask(req.SourceID, req.SourcePath, req.MediaType, req.FileIDs)
	if err != nil {
		logger.Errorf("OrganizeController[StartCandidateTaskAsync] 启动任务失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"task_id": taskID,
		"message": "候选扫描任务已启动",
	})
}

// GetCandidateTaskStatus 获取候选扫描任务状态
func (c *OrganizeController) GetCandidateTaskStatus(ctx *gin.Context) {
	taskID := ctx.Query("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "缺少task_id参数")
		return
	}

	task, err := c.organizeService.GetCandidateTaskStatus(taskID)
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		ErrorResp(ctx, http.StatusNotFound, "任务不存在")
		return
	}

	SuccessResp(ctx, task)
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
