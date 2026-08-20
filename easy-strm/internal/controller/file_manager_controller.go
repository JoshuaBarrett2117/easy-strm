package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/service"
)

type fileManagerUseCase interface {
	ListLocations() ([]domain.FileManagerLocation, error)
	Browse(location domain.FileManagerLocationRef, path string) (*domain.FileManagerBrowseResult, error)
	StartTransfer(req domain.FileManagerTransferRequest) (*domain.FileManagerTransferResponse, error)
	Delete(req domain.FileManagerDeleteRequest) error
}

// FileManagerController 提供统一文件管理HTTP接口。
type FileManagerController struct {
	service fileManagerUseCase
}

// NewFileManagerController 创建文件管理控制器。
func NewFileManagerController(fileManagerService *service.FileManagerService) *FileManagerController {
	return &FileManagerController{service: fileManagerService}
}

// ListLocations 返回可用的本地媒体源和115账号。
func (c *FileManagerController) ListLocations(ctx *gin.Context) {
	locations, err := c.service.ListLocations()
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": locations, "total": len(locations)})
}

// Browse 返回指定位置和目录中的文件。
func (c *FileManagerController) Browse(ctx *gin.Context) {
	locationID, err := strconv.Atoi(ctx.Query("location_id"))
	if err != nil || locationID <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "location_id无效")
		return
	}
	location := domain.FileManagerLocationRef{Type: ctx.Query("location_type"), ID: locationID}
	result, err := c.service.Browse(location, ctx.Query("path"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, result)
}

// Transfer 创建异步复制或剪切任务。
func (c *FileManagerController) Transfer(ctx *gin.Context) {
	var req domain.FileManagerTransferRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	result, err := c.service.StartTransfer(req)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	InfoResp(ctx, http.StatusAccepted, "文件传输任务已创建", result)
}

// Delete 删除本地或115文件和目录。
func (c *FileManagerController) Delete(ctx *gin.Context) {
	var req domain.FileManagerDeleteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if err := c.service.Delete(req); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"message": "删除成功", "total": len(req.Items)})
}
