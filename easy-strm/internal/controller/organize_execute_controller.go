package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// ExecuteOrganize 执行整理
func (c *OrganizeController) ExecuteOrganize(ctx *gin.Context) {
	var req organizeExecuteRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if err := normalizeOrganizeExecuteRequest(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	results, err := c.organizeService.OrganizeDirectory(req.SourceID, req.SourcePath, req.TargetPath, req.MediaType, req.Template, req.ConflictPolicy, req.OperationMode, req.FileIDs, req.UseCategory, req.ManualItems, req.RenameItems)
	if err != nil {
		logger.Errorf("OrganizeController[ExecuteOrganize] 执行失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	successCount, skippedCount, failedCount := summarizeOrganizeResults(results)

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

type organizeExecuteRequest struct {
	SourceID       int                             `json:"source_id" binding:"required"`
	SourcePath     string                          `json:"source_path"`
	TargetPath     string                          `json:"target_path"`
	MediaType      string                          `json:"media_type"`
	Template       string                          `json:"template"`
	ConflictPolicy string                          `json:"conflict_policy"`
	OperationMode  string                          `json:"operation_mode"`
	MoveFiles      *bool                           `json:"move_files"`
	FileIDs        []string                        `json:"file_ids"`
	UseCategory    bool                            `json:"use_category"`
	ManualItems    []domain.OrganizeManualOverride `json:"manual_items"`
	RenameItems    []domain.OrganizeRenameOverride `json:"rename_items"`
}

func normalizeOrganizeExecuteRequest(req *organizeExecuteRequest) error {
	if req.MediaType == "" {
		req.MediaType = "all"
	}
	if req.ConflictPolicy == "" {
		req.ConflictPolicy = "skip"
	}
	if !req.UseCategory && req.TargetPath == "" {
		return fmt.Errorf("使用非分类整理时必须提供目标目录")
	}
	if req.OperationMode == "" {
		req.OperationMode = "move"
		if req.MoveFiles != nil && !*req.MoveFiles {
			req.OperationMode = "copy"
		}
	}
	return nil
}

// ExecuteOrganizeAsync 异步执行整理，进度写入任务中心。
func (c *OrganizeController) ExecuteOrganizeAsync(ctx *gin.Context) {
	if c.taskService == nil {
		ErrorResp(ctx, http.StatusInternalServerError, "任务服务未初始化")
		return
	}

	var req organizeExecuteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	if err := normalizeOrganizeExecuteRequest(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	taskID := fmt.Sprintf("organize_%d_%d", req.SourceID, time.Now().UnixNano())
	taskName := fmt.Sprintf("手动整理-%d", req.SourceID)
	if err := c.taskService.Create(taskID, "organize", taskName); err != nil {
		logger.Errorf("OrganizeController[ExecuteOrganizeAsync] 创建任务失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	c.updateOrganizeTaskMetadata(taskID, req, nil, 0, 0, 0)
	_ = c.taskService.UpdateProgress(taskID, len(req.FileIDs), 0, 0, 0)

	go c.runOrganizeExecuteTask(taskID, req)

	SuccessResp(ctx, gin.H{
		"task_id": taskID,
		"message": "整理任务已启动，可在任务中心查看进度",
	})
}

func (c *OrganizeController) runOrganizeExecuteTask(taskID string, req organizeExecuteRequest) {
	failedItems := make([]map[string]interface{}, 0)
	progressTotal := len(req.FileIDs)
	if progressTotal == 0 {
		progressTotal = 1
	}

	defer func() {
		if panicErr := recover(); panicErr != nil {
			errMsg := fmt.Sprintf("整理任务异常中断: %v", panicErr)
			_ = c.taskService.SetError(taskID, errMsg)
		}
	}()

	_ = c.taskService.UpdateStatus(taskID, "running")
	_ = c.taskService.UpdateProgress(taskID, progressTotal, 0, 0, 0)

	results, err := c.organizeService.OrganizeDirectoryWithCallbacks(
		req.SourceID,
		req.SourcePath,
		req.TargetPath,
		req.MediaType,
		req.Template,
		req.ConflictPolicy,
		req.OperationMode,
		req.FileIDs,
		req.UseCategory,
		req.ManualItems,
		req.RenameItems,
		func(progress service.OrganizeExecutionProgress) {
			if progress.Total > 0 {
				progressTotal = progress.Total
			}
			if progress.Result != nil && (!progress.Result.Success || progress.Result.Skipped) {
				failedItems = append(failedItems, map[string]interface{}{
					"file_id":   progress.Result.FileID,
					"file_name": progress.Result.FileName,
					"category":  classifyOrganizeTaskFailure(progress.Result),
					"reason":    progress.Result.Message,
				})
			}
			_ = c.taskService.UpdateProgress(taskID, progressTotal, progress.Processed, progress.Success, progress.Failed+progress.Skipped)
		},
		func() bool {
			return c.taskService.IsCancelled(taskID)
		},
	)

	if err != nil && c.taskService.IsCancelled(taskID) {
		c.updateOrganizeTaskMetadata(taskID, req, failedItems, 0, 0, len(failedItems))
		_ = c.taskService.UpdateStatus(taskID, "cancelled")
		return
	}
	if err != nil {
		c.updateOrganizeTaskMetadata(taskID, req, failedItems, 0, 0, len(failedItems))
		_ = c.taskService.SetError(taskID, err.Error())
		return
	}

	successCount, skippedCount, failedCount := summarizeOrganizeResults(results)
	c.updateOrganizeTaskMetadata(taskID, req, failedItems, successCount, skippedCount, failedCount)
	_ = c.taskService.UpdateProgress(taskID, len(results), len(results), successCount, failedCount+skippedCount)
	if failedCount > 0 || skippedCount > 0 {
		_ = c.taskService.SetError(taskID, fmt.Sprintf("整理完成但有 %d 项失败/跳过", failedCount+skippedCount))
		return
	}
	_ = c.taskService.UpdateStatus(taskID, "completed")

	if successCount > 0 && c.embyRefreshCallback != nil {
		refreshResult := c.embyRefreshCallback(req.SourceID)
		if refreshResult != nil && !refreshResult.Success {
			logger.Warnf("OrganizeController[runOrganizeExecuteTask] Emby刷新失败: %s", refreshResult.Message)
		}
	}
}

func summarizeOrganizeResults(results []service.OrganizeResult) (successCount, skippedCount, failedCount int) {
	for _, result := range results {
		if result.Skipped {
			skippedCount++
		} else if result.Success {
			successCount++
		} else {
			failedCount++
		}
	}
	return successCount, skippedCount, failedCount
}

func classifyOrganizeTaskFailure(result *service.OrganizeResult) string {
	if result == nil {
		return "other"
	}
	if result.Skipped {
		return "conflict_skipped"
	}
	message := result.Message
	if message == "" {
		return "other"
	}
	lowerMessage := strings.ToLower(message)
	if containsAny(message, []string{"识别", "identify", "TMDB", "tmdb"}) {
		return "identify_failed"
	}
	if containsAny(lowerMessage, []string{"冲突", "conflict", "已存在", "exists"}) {
		return "conflict_skipped"
	}
	return "organize_failed"
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func (c *OrganizeController) updateOrganizeTaskMetadata(taskID string, req organizeExecuteRequest, failedItems []map[string]interface{}, successCount, skippedCount, failedCount int) {
	if c.taskService == nil || taskID == "" {
		return
	}
	metadata := map[string]interface{}{
		"source_id":            req.SourceID,
		"source_path":          req.SourcePath,
		"organize_target_path": req.TargetPath,
		"media_type":           req.MediaType,
		"conflict_policy":      req.ConflictPolicy,
		"operation_mode":       req.OperationMode,
		"trigger_mode":         "manual",
		"detected_files":       len(req.FileIDs),
		"success_files":        successCount,
		"failed_files":         failedCount,
		"skipped_files":        skippedCount,
		"result_summary":       fmt.Sprintf("成功 %d / 跳过 %d / 失败 %d", successCount, skippedCount, failedCount),
	}
	if len(failedItems) > 0 {
		metadata["failed_items"] = failedItems
		metadata["failed_item_count"] = len(failedItems)
	}
	if err := c.taskService.UpdateMetadata(taskID, metadata); err != nil {
		logger.Warnf("OrganizeController[updateOrganizeTaskMetadata] 更新任务元数据失败: %v", err)
	}
}
