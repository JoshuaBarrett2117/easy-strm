package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

// FileOperationController 文件操作控制器
// 负责处理文件的移动、复制、删除、重命名等操作请求
type FileOperationController struct {
	fileOperationService *service.FileOperationService
	mediaSourceService   *service.MediaSourceService
}

// NewFileOperationController 创建文件操作控制器实例
// 参数:
//   - fileOperationService: 文件操作服务
//   - mediaSourceService: 媒体源服务
// 返回:
//   - *FileOperationController: 文件操作控制器实例
func NewFileOperationController(
	fileOperationService *service.FileOperationService,
	mediaSourceService *service.MediaSourceService,
) *FileOperationController {
	return &FileOperationController{
		fileOperationService: fileOperationService,
		mediaSourceService:   mediaSourceService,
	}
}

// MoveFile 移动文件
// POST /api/media/files/move
// 请求体:
//   - source_id: 源媒体源ID
//   - file_id: 文件ID
//   - target_path: 目标路径
func (c *FileOperationController) MoveFile(ctx *gin.Context) {
	var req struct {
		SourceID   int    `json:"source_id" binding:"required"`
		FileID     string `json:"file_id" binding:"required"`
		TargetPath string `json:"target_path" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("FileOperationController[MoveFile] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 验证源媒体源是否存在
	source, err := c.mediaSourceService.GetByID(req.SourceID)
	if err != nil || source == nil {
		logger.Errorf("FileOperationController[MoveFile] 获取源媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "源媒体源不存在")
		return
	}

	// 执行移动操作
	result, err := c.fileOperationService.MoveFile(req.SourceID, req.FileID, req.TargetPath)
	if err != nil {
		logger.Errorf("FileOperationController[MoveFile] 移动文件失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("FileOperationController[MoveFile] 移动文件成功: %s -> %s", req.FileID, req.TargetPath)
	SuccessResp(ctx, gin.H{
		"message": result.Message,
		"data":    result,
	})
}

// CopyFile 复制文件
// POST /api/media/files/copy
// 请求体:
//   - source_id: 源媒体源ID
//   - target_id: 目标媒体源ID
//   - file_id: 文件ID
//   - target_path: 目标路径
//   - delete_source: 是否删除源文件（可选，默认false）
func (c *FileOperationController) CopyFile(ctx *gin.Context) {
	var req struct {
		SourceID     int    `json:"source_id" binding:"required"`
		TargetID     int    `json:"target_id" binding:"required"`
		FileID       string `json:"file_id" binding:"required"`
		TargetPath   string `json:"target_path" binding:"required"`
		DeleteSource bool   `json:"delete_source"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("FileOperationController[CopyFile] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 验证源媒体源是否存在
	source, err := c.mediaSourceService.GetByID(req.SourceID)
	if err != nil || source == nil {
		logger.Errorf("FileOperationController[CopyFile] 获取源媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "源媒体源不存在")
		return
	}

	// 验证目标媒体源是否存在
	target, err := c.mediaSourceService.GetByID(req.TargetID)
	if err != nil || target == nil {
		logger.Errorf("FileOperationController[CopyFile] 获取目标媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "目标媒体源不存在")
		return
	}

	// 执行复制操作
	result, err := c.fileOperationService.CopyFile(req.SourceID, req.TargetID, req.FileID, req.TargetPath, req.DeleteSource)
	if err != nil {
		logger.Errorf("FileOperationController[CopyFile] 复制文件失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("FileOperationController[CopyFile] 复制文件成功: %s -> %s", req.FileID, req.TargetPath)
	SuccessResp(ctx, gin.H{
		"message": result.Message,
		"data":    result,
	})
}

// DeleteFile 删除文件
// POST /api/media/files/delete
// 请求体:
//   - source_id: 媒体源ID
//   - file_ids: 文件ID列表（支持批量删除）
func (c *FileOperationController) DeleteFile(ctx *gin.Context) {
	var req struct {
		SourceID int      `json:"source_id" binding:"required"`
		FileIDs  []string `json:"file_ids" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("FileOperationController[DeleteFile] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 验证源媒体源是否存在
	source, err := c.mediaSourceService.GetByID(req.SourceID)
	if err != nil || source == nil {
		logger.Errorf("FileOperationController[DeleteFile] 获取源媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "源媒体源不存在")
		return
	}

	// 执行批量删除操作
	result, err := c.fileOperationService.DeleteFile(req.SourceID, req.FileIDs)
	if err != nil {
		logger.Errorf("FileOperationController[DeleteFile] 删除文件失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("FileOperationController[DeleteFile] 删除文件成功: %d 个文件", len(req.FileIDs))
	SuccessResp(ctx, gin.H{
		"message": result.Message,
		"data":    result,
	})
}

// RenameFile 重命名文件
// POST /api/media/files/rename
// 请求体:
//   - source_id: 媒体源ID
//   - file_id: 文件ID
//   - new_name: 新文件名
func (c *FileOperationController) RenameFile(ctx *gin.Context) {
	var req struct {
		SourceID int    `json:"source_id" binding:"required"`
		FileID   string `json:"file_id" binding:"required"`
		NewName  string `json:"new_name" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("FileOperationController[RenameFile] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 验证源媒体源是否存在
	source, err := c.mediaSourceService.GetByID(req.SourceID)
	if err != nil || source == nil {
		logger.Errorf("FileOperationController[RenameFile] 获取源媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "源媒体源不存在")
		return
	}

	// 执行重命名操作
	result, err := c.fileOperationService.RenameFile(req.SourceID, req.FileID, req.NewName)
	if err != nil {
		logger.Errorf("FileOperationController[RenameFile] 重命名文件失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("FileOperationController[RenameFile] 重命名文件成功: %s -> %s", req.FileID, req.NewName)
	SuccessResp(ctx, gin.H{
		"message": result.Message,
		"data":    result,
	})
}

// BatchOperation 批量文件操作
// POST /api/media/files/batch
// 请求体:
//   - operation: 操作类型 (move/copy/delete/rename)
//   - source_id: 源媒体源ID
//   - target_id: 目标媒体源ID (复制操作需要)
//   - items: 操作项列表
func (c *FileOperationController) BatchOperation(ctx *gin.Context) {
	var req struct {
		Operation string                      `json:"operation" binding:"required"`
		SourceID  int                         `json:"source_id" binding:"required"`
		TargetID  int                         `json:"target_id"`
		Items     []domain.BatchOperationItem `json:"items" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("FileOperationController[BatchOperation] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	// 验证操作类型
	validOperations := map[string]bool{
		"move":   true,
		"copy":   true,
		"delete": true,
		"rename": true,
	}
	if !validOperations[req.Operation] {
		ErrorResp(ctx, http.StatusBadRequest, "不支持的操作类型: "+req.Operation)
		return
	}

	// 验证源媒体源是否存在
	source, err := c.mediaSourceService.GetByID(req.SourceID)
	if err != nil || source == nil {
		logger.Errorf("FileOperationController[BatchOperation] 获取源媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "源媒体源不存在")
		return
	}

	// 复制操作需要验证目标媒体源
	if req.Operation == "copy" && req.TargetID > 0 {
		target, err := c.mediaSourceService.GetByID(req.TargetID)
		if err != nil || target == nil {
			logger.Errorf("FileOperationController[BatchOperation] 获取目标媒体源失败: %v", err)
			ErrorResp(ctx, http.StatusNotFound, "目标媒体源不存在")
			return
		}
	}

	// 执行批量操作
	result, err := c.fileOperationService.BatchOperation(req.Operation, req.SourceID, req.TargetID, req.Items)
	if err != nil {
		logger.Errorf("FileOperationController[BatchOperation] 批量操作失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("FileOperationController[BatchOperation] 批量操作完成: 类型=%s, 成功=%d, 失败=%d",
		req.Operation, result.Success, result.Failed)
	SuccessResp(ctx, gin.H{
		"message": "批量操作完成",
		"data":    result,
	})
}

// GetFilePreview 获取文件预览信息
// GET /api/media/files/preview
// 查询参数:
//   - source_id: 媒体源ID
//   - file_id: 文件ID
func (c *FileOperationController) GetFilePreview(ctx *gin.Context) {
	sourceID, err := strconv.Atoi(ctx.Query("source_id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的媒体源ID")
		return
	}

	fileID := ctx.Query("file_id")
	if fileID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "文件ID不能为空")
		return
	}

	// 获取媒体源信息
	source, err := c.mediaSourceService.GetByID(sourceID)
	if err != nil || source == nil {
		logger.Errorf("FileOperationController[GetFilePreview] 获取媒体源失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "媒体源不存在")
		return
	}

	// 构建预览信息
	preview := gin.H{
		"source_id":   sourceID,
		"source_name": source.Name,
		"source_type": source.SourceType,
		"file_id":     fileID,
	}

	// 根据媒体源类型获取额外信息
	if source.SourceType == domain.SourceTypeLocal {
		// 本地文件预览信息
		preview["path"] = source.Path
	} else if source.SourceType == domain.SourceTypeCloud115 {
		// 115云盘文件预览信息
		if source.Cloud115ID != nil {
			preview["cloud115_id"] = *source.Cloud115ID
		}
	}

	logger.Infof("FileOperationController[GetFilePreview] 获取文件预览信息: source_id=%d, file_id=%s", sourceID, fileID)
	SuccessResp(ctx, preview)
}
