package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// OfflineDownloadService 云下载服务接口（按controller调用场景隔离，便于测试注入桩实现）
type OfflineDownloadService interface {
	// Submit 提交批量云下载
	Submit(ctx context.Context, req domain.OfflineDownloadSubmitRequest) (*domain.OfflineDownloadSubmitResponse, error)
	// List 分页查询云下载记录
	List(ctx context.Context, cloud115ID int, status string, page, pageSize int) (*domain.OfflineDownloadTaskListResponse, error)
	// DeleteRecord 删除云下载记录（默认同时移除115侧离线任务）
	DeleteRecord(ctx context.Context, id int64, deleteFiles bool) error
}

// OfflineDownloadController 115云下载（离线下载）控制器
type OfflineDownloadController struct {
	service OfflineDownloadService
}

// NewOfflineDownloadController 创建云下载控制器实例
func NewOfflineDownloadController(service OfflineDownloadService) *OfflineDownloadController {
	return &OfflineDownloadController{service: service}
}

// Submit 提交批量云下载任务
// POST /v1/resource/115-offline/submit
// 请求体: { cloud115_id, directory?, urls: [...] }
func (c *OfflineDownloadController) Submit(ctx *gin.Context) {
	var req domain.OfflineDownloadSubmitRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("OfflineDownloadController[Submit] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	if req.Cloud115ID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "请选择执行云下载的115账号")
		return
	}
	if len(req.Urls) == 0 {
		ErrorResp(ctx, http.StatusBadRequest, "请提供至少一个下载链接")
		return
	}

	result, err := c.service.Submit(ctx.Request.Context(), req)
	if err != nil {
		logger.Errorf("OfflineDownloadController[Submit] 提交云下载失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// List 分页查询云下载记录
// GET /v1/resource/115-offline/tasks?cloud115_id=&status=&page=1&page_size=20
func (c *OfflineDownloadController) List(ctx *gin.Context) {
	cloud115ID, _ := strconv.Atoi(ctx.Query("cloud115_id"))
	status := ctx.Query("status")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	result, err := c.service.List(ctx.Request.Context(), cloud115ID, status, page, pageSize)
	if err != nil {
		logger.Errorf("OfflineDownloadController[List] 查询云下载记录失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, result)
}

// Delete 删除云下载记录
// DELETE /v1/resource/115-offline/tasks/:id?delete_files=false
// delete_files 为 true 时同时删除已下载完成的云端文件
func (c *OfflineDownloadController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的记录ID")
		return
	}
	deleteFiles, _ := strconv.ParseBool(ctx.DefaultQuery("delete_files", "false"))

	if err := c.service.DeleteRecord(ctx.Request.Context(), id, deleteFiles); err != nil {
		logger.Errorf("OfflineDownloadController[Delete] 删除云下载记录失败: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{"deleted": id})
}
