package controller

import (
	"fmt"
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

type Cloud115Controller struct {
	cloud115Service *service.Cloud115Service

	// --- 回调依赖：main 包全局函数通过依赖注入解耦 ---
	// NOTE: 以下函数属于 main 包，无法在 controller 层直接引用，
	// 因此通过回调模式注入，与 CronController/LogController 保持一致

	// getAllCloud115: 获取所有 115 账号（main.GetAllCloud115）
	getAllCloud115 func(sortField, sortOrder string) ([]*Cloud115AccountBrief, error)
	// getCloud115ByID: 根据 ID 获取 115 账号（main.GetCloud115ByID）
	getCloud115ByID func(id int) (*Cloud115AccountBrief, error)
	// createCloud115: 创建 115 账号（main.CreateCloud115）
	createCloud115 func(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*Cloud115AccountBrief, error)
	// updateCloud115: 更新 115 账号（main.UpdateCloud115）
	updateCloud115 func(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*Cloud115AccountBrief, error)
	// deleteCloud115: 删除 115 账号（main.DeleteCloud115）
	deleteCloud115 func(id int) error
	// getQRCode: 获取登录二维码（main.Client.GetQRCode）
	getQRCode func() (interface{}, error)
	// checkLoginStatus: 检查登录状态（main.Client.CheckLoginStatus）
	checkLoginStatus func(session interface{}) (interface{}, error)
	// qrcodeLogin: 二维码登录（main.Client.QRCodeLogin）
	qrcodeLogin func(session interface{}) (interface{}, error)
	// qrcodeLoginWithApp: 带 App 的二维码登录（main.Client.QRCodeLoginWithApp）
	qrcodeLoginWithApp func(session interface{}, app string) (interface{}, error)
	// getOpenAPIQRCode: 获取 Open API 二维码（main.Client.GetOpenAPIQRCode）
	getOpenAPIQRCode func() (interface{}, error)
	// checkOpenAPILoginStatus: 检查 Open API 登录状态（main.Client.CheckOpenAPILoginStatus）
	checkOpenAPILoginStatus func(state string) (interface{}, error)
	// confirmOpenAPILogin: 确认 Open API 登录（main.Client.ConfirmOpenAPILogin）
	confirmOpenAPILogin func(state string) (interface{}, error)
	// getFileList: 获取 115 文件列表（main.Client.GetFileList）
	getFileList func(cid, showDir, offset, limit int, cloud115ID int, cookie string) (interface{}, error)
	// getFileDirectLink: 获取 115 文件直链（main.Client.GetFileDirectLink）
	getFileDirectLink func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error)
	// exportDirectoryTree: 导出目录树（main.Client.ExportDirectoryTree115）
	exportDirectoryTree func(fileIds, target, cookie string) (interface{}, error)
	// getExportDirectoryTreeStatus: 获取目录树导出状态（main.Client.GetExportDirectoryTreeStatus）
	getExportDirectoryTreeStatus func(exportId, cookie string) (interface{}, error)
	// testAccountCookie: 测试账号 cookie 是否有效
	testAccountCookie func(cloud115ID int, cookie string) (interface{}, error)
}

func NewCloud115Controller(cloud115Service *service.Cloud115Service) *Cloud115Controller {
	return &Cloud115Controller{
		cloud115Service: cloud115Service,
	}
}

// --- 回调注入方法 ---

func (c *Cloud115Controller) SetGetAllCloud115(fn func(sortField, sortOrder string) ([]*Cloud115AccountBrief, error)) {
	c.getAllCloud115 = fn
}

func (c *Cloud115Controller) SetGetCloud115ByID(fn func(id int) (*Cloud115AccountBrief, error)) {
	c.getCloud115ByID = fn
}

func (c *Cloud115Controller) SetCreateCloud115(fn func(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*Cloud115AccountBrief, error)) {
	c.createCloud115 = fn
}

func (c *Cloud115Controller) SetUpdateCloud115(fn func(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*Cloud115AccountBrief, error)) {
	c.updateCloud115 = fn
}

func (c *Cloud115Controller) SetDeleteCloud115(fn func(id int) error) {
	c.deleteCloud115 = fn
}

func (c *Cloud115Controller) SetGetQRCode(fn func() (interface{}, error)) {
	c.getQRCode = fn
}

func (c *Cloud115Controller) SetCheckLoginStatus(fn func(session interface{}) (interface{}, error)) {
	c.checkLoginStatus = fn
}

func (c *Cloud115Controller) SetQRCodeLogin(fn func(session interface{}) (interface{}, error)) {
	c.qrcodeLogin = fn
}

func (c *Cloud115Controller) SetQRCodeLoginWithApp(fn func(session interface{}, app string) (interface{}, error)) {
	c.qrcodeLoginWithApp = fn
}

func (c *Cloud115Controller) SetGetOpenAPIQRCode(fn func() (interface{}, error)) {
	c.getOpenAPIQRCode = fn
}

func (c *Cloud115Controller) SetCheckOpenAPILoginStatus(fn func(state string) (interface{}, error)) {
	c.checkOpenAPILoginStatus = fn
}

func (c *Cloud115Controller) SetConfirmOpenAPILogin(fn func(state string) (interface{}, error)) {
	c.confirmOpenAPILogin = fn
}

func (c *Cloud115Controller) SetGetFileList(fn func(cid, showDir, offset, limit int, cloud115ID int, cookie string) (interface{}, error)) {
	c.getFileList = fn
}

func (c *Cloud115Controller) SetGetFileDirectLink(fn func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error)) {
	c.getFileDirectLink = fn
}

func (c *Cloud115Controller) SetExportDirectoryTree(fn func(fileIds, target, cookie string) (interface{}, error)) {
	c.exportDirectoryTree = fn
}

func (c *Cloud115Controller) SetGetExportDirectoryTreeStatus(fn func(exportId, cookie string) (interface{}, error)) {
	c.getExportDirectoryTreeStatus = fn
}

func (c *Cloud115Controller) SetTestAccountCookie(fn func(cloud115ID int, cookie string) (interface{}, error)) {
	c.testAccountCookie = fn
}

// --- 辅助方法 ---

// formatCloud115 格式化 115 账号为 API 响应格式
// 保持与原 auth.go 内联处理器一致的响应字段
func formatCloud115(acc *Cloud115AccountBrief, full bool) map[string]interface{} {
	result := map[string]interface{}{
		"id":                  acc.ID,
		"name":                acc.Name,
		"cookie":              acc.Cookie,
		"refresh_token":       acc.RefreshToken,
		"access_token":        acc.AccessToken,
		"expires_in":          acc.ExpiresIn,
		"transfer_account_id": acc.TransferAccountID,
		"transfer_directory":  acc.TransferDirectory,
		"account_type":        acc.AccountType,
		"priority":            acc.Priority,
		"status":              acc.Status,
		"transfer_method":     acc.TransferMethod,
		"alist_url":           acc.AlistUrl,
		"alist_token":         acc.AlistToken,
		"create_time":         acc.CreateTime.Format("2006-01-02 15:04:05"),
		"update_time":         acc.UpdateTime.Format("2006-01-02 15:04:05"),
	}
	// full=false 时省略部分字段（列表视图不需要）
	_ = full // 目前列表和详情返回相同字段，保持与原 auth.go 一致
	return result
}

// resolveCloud115Account 解析请求中的 cloud115_id 参数并返回对应账号
// 如果未指定 cloud115_id，则返回第一个账号
// 这是一个通用辅助方法，被多个处理器复用
func (c *Cloud115Controller) resolveCloud115Account(ctx *gin.Context) (*Cloud115AccountBrief, error) {
	cloud115IdStr := ctx.Query("cloud115_id")

	if cloud115IdStr != "" {
		var cloud115Id int
		fmt.Sscanf(cloud115IdStr, "%d", &cloud115Id)
		return c.getCloud115ByID(cloud115Id)
	}

	// 未指定时使用第一个账号
	list, err := c.getAllCloud115("", "")
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no cloud115 accounts found")
	}
	return list[0], nil
}

// --- 路由处理器方法 ---

// GetList 获取所有115云账号
// Route: GET /cloud115
func (c *Cloud115Controller) GetList(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[GetList] 获取115账号列表")

	if c.getAllCloud115 == nil {
		logger.Error("Cloud115Controller[GetList] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	sortField := ctx.DefaultQuery("sort_field", "id")
	sortOrder := ctx.DefaultQuery("sort_order", "asc")

	list, err := c.getAllCloud115(sortField, sortOrder)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetList] 获取列表失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 格式化返回数据，保持与原 auth.go 内联处理器一致的响应格式
	formattedList := make([]map[string]interface{}, len(list))
	for i, acc := range list {
		formattedList[i] = formatCloud115(acc, false)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": formattedList,
	})
}

// GetByID 根据ID获取115云账号
// Route: GET /cloud115/:id
func (c *Cloud115Controller) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("Cloud115Controller[GetByID] 获取账号详情, ID: %d", id)

	if c.getCloud115ByID == nil {
		logger.Error("Cloud115Controller[GetByID] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.getCloud115ByID(id)
	if err != nil || acc == nil {
		logger.Errorf("Cloud115Controller[GetByID] 获取详情失败: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Cloud115 account not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": formatCloud115(acc, true),
	})
}

// Create 创建115云账号
// Route: POST /cloud115
func (c *Cloud115Controller) Create(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[Create] 创建115账号")

	var createData struct {
		Name              string `json:"name" binding:"required"`
		Cookie            string `json:"cookie"`
		RefreshToken      string `json:"refresh_token"`
		AccessToken       string `json:"access_token"`
		ExpiresIn         int    `json:"expires_in"`
		TransferAccountID int    `json:"transfer_account_id"`
		TransferDirectory string `json:"transfer_directory"`
		AccountType       string `json:"account_type"`
		Priority          int    `json:"priority"`
		TransferMethod    string `json:"transfer_method"`
		AlistUrl          string `json:"alist_url"`
		AlistToken        string `json:"alist_token"`
	}

	if err := ctx.ShouldBindJSON(&createData); err != nil {
		logger.Warnf("Cloud115Controller[Create] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if createData.AccountType == "" {
		createData.AccountType = "resource"
	}
	if createData.Priority == 0 {
		createData.Priority = 5
	}
	if createData.TransferMethod == "" {
		createData.TransferMethod = "115driver"
	}

	if c.createCloud115 == nil {
		logger.Error("Cloud115Controller[Create] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.createCloud115(
		createData.Name, createData.Cookie, createData.RefreshToken,
		createData.AccessToken, createData.ExpiresIn, createData.TransferAccountID,
		createData.TransferDirectory, createData.AccountType, createData.Priority,
		createData.TransferMethod, createData.AlistUrl, createData.AlistToken,
	)
	if err != nil {
		logger.Errorf("Cloud115Controller[Create] 创建失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Infof("Cloud115Controller[Create] 创建账号成功: %s", acc.Name)
	ctx.JSON(http.StatusCreated, gin.H{
		"message": "Cloud115 account created successfully",
		"data":    formatCloud115(acc, true),
	})
}

// Update 更新115云账号
// Route: PUT /cloud115/:id
func (c *Cloud115Controller) Update(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("Cloud115Controller[Update] 更新115账号, ID: %d", id)

	var updateData struct {
		Name              string `json:"name" binding:"required"`
		Cookie            string `json:"cookie"`
		RefreshToken      string `json:"refresh_token"`
		AccessToken       string `json:"access_token"`
		ExpiresIn         int    `json:"expires_in"`
		TransferAccountID int    `json:"transfer_account_id"`
		TransferDirectory string `json:"transfer_directory"`
		AccountType       string `json:"account_type"`
		Priority          int    `json:"priority"`
		Status            string `json:"status"`
		TransferMethod    string `json:"transfer_method"`
		AlistUrl          string `json:"alist_url"`
		AlistToken        string `json:"alist_token"`
	}

	if err := ctx.ShouldBindJSON(&updateData); err != nil {
		logger.Warnf("Cloud115Controller[Update] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if updateData.AccountType == "" {
		updateData.AccountType = "resource"
	}
	if updateData.Priority == 0 {
		updateData.Priority = 5
	}
	if updateData.Status == "" {
		updateData.Status = "active"
	}
	if updateData.TransferMethod == "" {
		updateData.TransferMethod = "115driver"
	}

	if c.updateCloud115 == nil {
		logger.Error("Cloud115Controller[Update] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.updateCloud115(
		id, updateData.Name, updateData.Cookie, updateData.RefreshToken,
		updateData.AccessToken, updateData.ExpiresIn, updateData.TransferAccountID,
		updateData.TransferDirectory, updateData.AccountType, updateData.Priority,
		updateData.Status, updateData.TransferMethod, updateData.AlistUrl, updateData.AlistToken,
	)
	if err != nil {
		logger.Errorf("Cloud115Controller[Update] 更新失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Infof("Cloud115Controller[Update] 更新账号成功: %s (ID: %d)", acc.Name, id)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Cloud115 account updated successfully",
		"data":    formatCloud115(acc, false),
	})
}

// Delete 删除115云账号
// Route: DELETE /cloud115/:id
func (c *Cloud115Controller) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("Cloud115Controller[Delete] 删除115账号, ID: %d", id)

	if c.deleteCloud115 == nil {
		logger.Error("Cloud115Controller[Delete] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if err := c.deleteCloud115(id); err != nil {
		logger.Errorf("Cloud115Controller[Delete] 删除失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Infof("Cloud115Controller[Delete] 删除账号成功: ID %d", id)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Cloud115 account deleted successfully",
	})
}

// TestAccountCookie 测试115账号cookie是否有效
// Route: GET /auth/cloud115/:id
func (c *Cloud115Controller) TestAccountCookie(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[TestAccountCookie] 测试115账号")

	idStr := ctx.Param("id")
	if idStr == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	var id int
	fmt.Sscanf(idStr, "%d", &id)

	if c.getCloud115ByID == nil || c.testAccountCookie == nil {
		logger.Error("Cloud115Controller[TestAccountCookie] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	acc, err := c.getCloud115ByID(id)
	if err != nil {
		logger.Errorf("Cloud115Controller[TestAccountCookie] 获取账号失败 ID %d: %v", id, err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Cloud115 account not found"})
		return
	}

	result, err := c.testAccountCookie(acc.ID, acc.Cookie)
	if err != nil {
		ctx.JSON(http.StatusOK, gin.H{
			"state":   false,
			"data":    gin.H{"account_id": acc.ID, "account_name": acc.Name},
			"message": "账号测试失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}
