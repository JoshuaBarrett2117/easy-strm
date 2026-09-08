package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// ResourceController 资源聚合控制器
// 负责处理115分享链接的解析、文件浏览和批量转存请求
type ResourceController struct {
	shareTransferService *service.ShareTransferService
}

// NewResourceController 创建资源聚合控制器实例
// 参数:
//   - shareTransferService: 分享转存服务
//
// 返回:
//   - *ResourceController: 控制器实例
func NewResourceController(shareTransferService *service.ShareTransferService) *ResourceController {
	return &ResourceController{
		shareTransferService: shareTransferService,
	}
}

// Parse 解析115分享链接
// POST /v1/resource/115-share/parse
// 请求体: { url, password? }
// 成功响应: { share_code, folder_name, files, total_files, total_size }
func (c *ResourceController) Parse(ctx *gin.Context) {
	var req domain.ParseShareRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("ResourceController[Parse] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体，请提供115分享链接")
		return
	}

	if req.URL == "" {
		ErrorResp(ctx, http.StatusBadRequest, "请提供115分享链接")
		return
	}

	result, err := c.shareTransferService.ParseShareLink(ctx.Request.Context(), req.URL, req.Password)
	if err != nil {
		logger.Errorf("ResourceController[Parse] 解析失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// GetFiles 获取分享文件列表（支持分页、筛选、搜索）
// GET /v1/resource/115-share/files?share_code=xxx&password=xxx&dir_id=xxx&page=1&page_size=50&type=video&keyword=xxx
func (c *ResourceController) GetFiles(ctx *gin.Context) {
	shareCode := ctx.Query("share_code")
	password := ctx.Query("password")
	dirID := ctx.DefaultQuery("dir_id", "0")

	if shareCode == "" {
		ErrorResp(ctx, http.StatusBadRequest, "请提供分享码")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))
	typeFilter := ctx.Query("type")
	keyword := ctx.Query("keyword")

	result, err := c.shareTransferService.GetShareFiles(
		ctx.Request.Context(), shareCode, password, dirID, page, pageSize, typeFilter, keyword,
	)
	if err != nil {
		logger.Errorf("ResourceController[GetFiles] 获取文件列表失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// SubmitTransfer 提交转存任务
// POST /v1/resource/115-share/transfer
// 请求体: { share_code, password, target_cloud115_id, target_directory, files, conflict_strategy,
//
//	auto_organize, auto_scrape, organize_source_id, organize_target_path }
//
// 说明：auto_organize/auto_scrape 由 ShouldBindJSON 自动接收；业务逻辑在 service 层处理（仅多写元数据字段，
// 默认（false）时终态后不触发额外整理/刮削，行为与原版一致）。
func (c *ResourceController) SubmitTransfer(ctx *gin.Context) {
	var req domain.TransferRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("ResourceController[SubmitTransfer] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	if req.ShareCode == "" {
		ErrorResp(ctx, http.StatusBadRequest, "请提供分享码")
		return
	}
	if req.TargetCloud115Id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "请选择目标115账号")
		return
	}
	if len(req.Files) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "请选择需要转存的文件")
		return
	}

	// 设置默认冲突策略
	if req.ConflictStrategy == "" {
		req.ConflictStrategy = "skip"
	}

	result, err := c.shareTransferService.SubmitTransfer(ctx.Request.Context(), req)
	if err != nil {
		logger.Errorf("ResourceController[SubmitTransfer] 提交转存失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// GetProgress 查询转存任务进度
// GET /v1/resource/115-share/transfer/:taskId
func (c *ResourceController) GetProgress(ctx *gin.Context) {
	taskId := ctx.Param("taskId")
	if taskId == "" {
		ErrorResp(ctx, http.StatusBadRequest, "请提供任务ID")
		return
	}

	result, err := c.shareTransferService.GetProgress(ctx.Request.Context(), taskId)
	if err != nil {
		logger.Errorf("ResourceController[GetProgress] 查询进度失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// CancelTransfer 取消转存任务
// POST /v1/resource/115-share/transfer/:taskId/cancel
func (c *ResourceController) CancelTransfer(ctx *gin.Context) {
	taskId := ctx.Param("taskId")
	if taskId == "" {
		ErrorResp(ctx, http.StatusBadRequest, "请提供任务ID")
		return
	}

	result, err := c.shareTransferService.CancelTransfer(ctx.Request.Context(), taskId)
	if err != nil {
		logger.Errorf("ResourceController[CancelTransfer] 取消失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// RetryTransfer 重试转存失败文件
// POST /v1/resource/115-share/transfer/:taskId/retry
func (c *ResourceController) RetryTransfer(ctx *gin.Context) {
	taskId := ctx.Param("taskId")
	if taskId == "" {
		ErrorResp(ctx, http.StatusBadRequest, "请提供任务ID")
		return
	}

	result, err := c.shareTransferService.RetryTransfer(ctx.Request.Context(), taskId)
	if err != nil {
		logger.Errorf("ResourceController[RetryTransfer] 重试失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, result)
}
