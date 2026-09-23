package controller

import (
	"fmt"
	"net/http"

	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

// GetFileList 获取115云文件列表
// Route: GET /115/files
func (c *Cloud115Controller) GetFileList(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[GetFileList] 获取文件列表")

	cid := 0
	showDir := 1
	offset := 0
	limit := 100

	if ctx.Query("cid") != "" {
		fmt.Sscanf(ctx.Query("cid"), "%d", &cid)
	}
	if ctx.Query("show_dir") != "" {
		fmt.Sscanf(ctx.Query("show_dir"), "%d", &showDir)
	}
	if ctx.Query("offset") != "" {
		fmt.Sscanf(ctx.Query("offset"), "%d", &offset)
	}
	if ctx.Query("limit") != "" {
		fmt.Sscanf(ctx.Query("limit"), "%d", &limit)
	}

	if c.getCloud115ByID == nil || c.getAllCloud115 == nil || c.getFileList == nil {
		logger.Error("Cloud115Controller[GetFileList] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.resolveCloud115Account(ctx)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetFileList] 获取账号失败: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	fileList, err := c.getFileList(cid, showDir, offset, limit, acc.ID, acc.Cookie)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetFileList] 获取文件列表失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, fileList)
}

// GetDirectLink 获取115云文件直链
// Route: GET /115/direct-link
func (c *Cloud115Controller) GetDirectLink(ctx *gin.Context) {
	defer beginDirectLinkRequest(ctx, "cloud115_api")()
	logger.Debug("Cloud115Controller[GetDirectLink] 获取文件直链")

	fid := ctx.Query("fid")
	if fid == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "fid is required"})
		return
	}

	if c.getCloud115ByID == nil || c.getAllCloud115 == nil || c.getFileDirectLink == nil {
		logger.Error("Cloud115Controller[GetDirectLink] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.resolveCloud115Account(ctx)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetDirectLink] 获取账号失败: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 115 CDN 签名依赖 UA，生成直链时必须使用实际客户端 UA。
	clientUA := ctx.GetHeader("User-Agent")

	logger.Infof("[DirectLink] event=resolve request_id=%s source=cloud115_api cloud115_id=%d pickcode=%q effective_ua=%q", logger.RequestID(ctx.Request.Context()), acc.ID, fid, clientUA)
	result, err := c.getFileDirectLink(0, fid, acc.ID, acc.Cookie, clientUA)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetDirectLink] 获取直链失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// ExportDirectoryTree 导出115目录树
// Route: POST /115/export-dir
func (c *Cloud115Controller) ExportDirectoryTree(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[ExportDirectoryTree] 导出目录树")

	var exportData struct {
		CID        string `json:"cid" binding:"required"`
		Cloud115Id string `json:"cloud115_id"`
	}

	if err := ctx.ShouldBindJSON(&exportData); err != nil {
		logger.Warnf("Cloud115Controller[ExportDirectoryTree] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if c.getCloud115ByID == nil || c.getAllCloud115 == nil || c.exportDirectoryTree == nil {
		logger.Error("Cloud115Controller[ExportDirectoryTree] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.resolveCloud115AccountFromJSON(exportData.Cloud115Id)
	if err != nil {
		logger.Errorf("Cloud115Controller[ExportDirectoryTree] 获取账号失败: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	target := fmt.Sprintf("U_1_%s", exportData.CID)
	result, err := c.exportDirectoryTree(exportData.CID, target, acc.Cookie)
	if err != nil {
		logger.Errorf("Cloud115Controller[ExportDirectoryTree] 导出目录树失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// GetExportDirectoryTreeStatus 查询115目录树生成状态
// Route: GET /115/export-dir/status
func (c *Cloud115Controller) GetExportDirectoryTreeStatus(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[GetExportDirectoryTreeStatus] 查询导出状态")

	exportId := ctx.Query("export_id")
	if exportId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "export_id is required"})
		return
	}

	if c.getCloud115ByID == nil || c.getAllCloud115 == nil || c.getExportDirectoryTreeStatus == nil {
		logger.Error("Cloud115Controller[GetExportDirectoryTreeStatus] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.resolveCloud115Account(ctx)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetExportDirectoryTreeStatus] 获取账号失败: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	result, err := c.getExportDirectoryTreeStatus(exportId, acc.Cookie)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetExportDirectoryTreeStatus] 查询状态失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// resolveCloud115AccountFromJSON 从 JSON body 中的 cloud115_id 字符串解析账号。
func (c *Cloud115Controller) resolveCloud115AccountFromJSON(cloud115IdStr string) (*Cloud115AccountBrief, error) {
	if cloud115IdStr != "" {
		var cloud115Id int
		fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
		return c.getCloud115ByID(cloud115Id)
	}

	list, err := c.getAllCloud115("", "")
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no cloud115 accounts found")
	}
	return list[0], nil
}
