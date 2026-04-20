package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

type Cloud115Controller struct {
	cloud115Service     *service.Cloud115Service
	notificationService *service.NotificationService

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

func NewCloud115Controller(cloud115Service *service.Cloud115Service, notificationService *service.NotificationService) *Cloud115Controller {
	return &Cloud115Controller{
		cloud115Service:     cloud115Service,
		notificationService: notificationService,
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

// GetLoginChannels 获取支持的登录渠道列表
// Route: GET /115/login/channels
func (c *Cloud115Controller) GetLoginChannels(ctx *gin.Context) {
	channels := []gin.H{
		{"value": "wechatmini", "label": "微信小程序", "description": "使用微信小程序扫码登录"},
		{"value": "web", "label": "网页版", "description": "使用115网页版扫码登录"},
		{"value": "android", "label": "安卓APP", "description": "使用115安卓APP扫码登录"},
		{"value": "ios", "label": "iOS APP", "description": "使用115 iOS APP扫码登录"},
		{"value": "tv", "label": "电视版", "description": "使用115电视版扫码登录"},
		{"value": "alipaymini", "label": "支付宝小程序", "description": "使用支付宝小程序扫码登录"},
		{"value": "qandroid", "label": "安卓Q版", "description": "使用115安卓Q版扫码登录"},
	}
	// 保持与原 auth.go 内联处理器一致的响应格式
	ctx.JSON(http.StatusOK, gin.H{
		"state":   true,
		"code":    0,
		"message": "success",
		"data":    channels,
	})
}

// GetQRCode 获取登录二维码
// Route: GET /115/qrcode
func (c *Cloud115Controller) GetQRCode(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[GetQRCode] 获取二维码")

	if c.getQRCode == nil {
		logger.Error("Cloud115Controller[GetQRCode] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	qrCodeResp, err := c.getQRCode()
	if err != nil {
		logger.Errorf("Cloud115Controller[GetQRCode] 获取二维码失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, qrCodeResp)
}

// CheckLoginStatus 检查扫码登录状态
// Route: GET /115/login/status
func (c *Cloud115Controller) CheckLoginStatus(ctx *gin.Context) {
	uid := ctx.Query("uid")
	timeStr := ctx.Query("time")
	sign := ctx.Query("sign")

	if uid == "" || timeStr == "" || sign == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "uid, time and sign are required"})
		return
	}

	if c.checkLoginStatus == nil {
		logger.Error("Cloud115Controller[CheckLoginStatus] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	session := map[string]string{
		"uid":  uid,
		"time": timeStr,
		"sign": sign,
	}

	statusResp, err := c.checkLoginStatus(session)
	if err != nil {
		logger.Errorf("Cloud115Controller[CheckLoginStatus] 检查状态失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, statusResp)
}

// ConfirmLogin 确认登录并保存凭据
// Route: POST /115/login/confirm
func (c *Cloud115Controller) ConfirmLogin(ctx *gin.Context) {
	var loginData struct {
		UID     string `json:"uid" binding:"required"`
		Time    int64  `json:"time" binding:"required"`
		Sign    string `json:"sign" binding:"required"`
		Name    string `json:"name"`
		CloudID int    `json:"cloud_id"`
		App     string `json:"app"`
	}

	if err := ctx.ShouldBindJSON(&loginData); err != nil {
		logger.Warnf("Cloud115Controller[ConfirmLogin] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if c.qrcodeLogin == nil || c.qrcodeLoginWithApp == nil {
		logger.Error("Cloud115Controller[ConfirmLogin] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	session := map[string]interface{}{
		"uid":  loginData.UID,
		"time": loginData.Time,
		"sign": loginData.Sign,
	}

	var cred interface{}
	var err error

	if loginData.App != "" {
		cred, err = c.qrcodeLoginWithApp(session, loginData.App)
	} else {
		cred, err = c.qrcodeLogin(session)
	}
	if err != nil {
		logger.Errorf("Cloud115Controller[ConfirmLogin] 扫码登录失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// cred 包含 cookie 信息，由回调注入的函数返回
	// 回调函数负责类型转换，这里直接返回结果
	ctx.JSON(http.StatusOK, cred)
}

// GetOpenAPIQRCode 获取Open API登录二维码
// Route: GET /115/open/qrcode
func (c *Cloud115Controller) GetOpenAPIQRCode(ctx *gin.Context) {
	logger.Debug("Cloud115Controller[GetOpenAPIQRCode] 获取Open API二维码")

	if c.getOpenAPIQRCode == nil {
		logger.Error("Cloud115Controller[GetOpenAPIQRCode] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	session, err := c.getOpenAPIQRCode()
	if err != nil {
		logger.Errorf("Cloud115Controller[GetOpenAPIQRCode] 获取Open API二维码失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, session)
}

// CheckOpenAPILoginStatus 检查Open API登录状态
// Route: GET /115/open/login/status
func (c *Cloud115Controller) CheckOpenAPILoginStatus(ctx *gin.Context) {
	state := ctx.Query("state")
	if state == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "state is required"})
		return
	}

	if c.checkOpenAPILoginStatus == nil {
		logger.Error("Cloud115Controller[CheckOpenAPILoginStatus] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	status, err := c.checkOpenAPILoginStatus(state)
	if err != nil {
		logger.Errorf("Cloud115Controller[CheckOpenAPILoginStatus] 检查状态失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, status)
}

// ConfirmOpenAPILogin 确认Open API登录并保存Token
// Route: POST /115/open/login/confirm
func (c *Cloud115Controller) ConfirmOpenAPILogin(ctx *gin.Context) {
	var loginData struct {
		State   string `json:"state" binding:"required"`
		Name    string `json:"name"`
		CloudID int    `json:"cloud_id"`
	}

	if err := ctx.ShouldBindJSON(&loginData); err != nil {
		logger.Warnf("Cloud115Controller[ConfirmOpenAPILogin] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if c.confirmOpenAPILogin == nil {
		logger.Error("Cloud115Controller[ConfirmOpenAPILogin] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	result, err := c.confirmOpenAPILogin(loginData.State)
	if err != nil {
		logger.Errorf("Cloud115Controller[ConfirmOpenAPILogin] 确认登录失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

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

	// 解析 cloud115_id 参数
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

	// 获取客户端 UA（115 CDN 签名必须与访问时的 UA 匹配）
	clientUA := ctx.GetHeader("User-Agent")

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

	// 解析 cloud115_id
	var acc *Cloud115AccountBrief
	var err error

	if exportData.Cloud115Id != "" {
		var cloud115Id int
		fmt.Sscanf(exportData.Cloud115Id, "%d", &cloud115Id)
		acc, err = c.getCloud115ByID(cloud115Id)
	} else {
		acc, err = c.resolveCloud115AccountFromJSON(exportData.Cloud115Id)
	}

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

// resolveCloud115AccountFromJSON 从 JSON body 中的 cloud115_id 字符串解析账号
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

// --- 通知配置相关处理器（已有，保持不变） ---

func (c *Cloud115Controller) GetNotificationConfig(ctx *gin.Context) {
	list, err := c.cloud115Service.GetAllNotificationConfig()
	if err != nil {
		logger.Errorf("Cloud115Controller[GetNotificationConfig] 获取通知配置失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取通知配置失败")
		return
	}
	SuccessResp(ctx, gin.H{"data": list})
}

func (c *Cloud115Controller) UpdateNotificationConfig(ctx *gin.Context) {
	var reqData struct {
		Channel string `json:"channel" binding:"required"`
		Config  string `json:"config"`
		Enabled bool   `json:"enabled"`
	}
	if err := ctx.ShouldBindJSON(&reqData); err != nil {
		logger.Warnf("Cloud115Controller[UpdateNotificationConfig] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	validChannels := map[string]bool{"telegram": true, "serverchan": true, "email": true}
	if !validChannels[reqData.Channel] {
		ErrorResp(ctx, http.StatusBadRequest, "无效的通知渠道")
		return
	}
	config, err := c.cloud115Service.UpsertNotificationConfig(reqData.Channel, reqData.Config, reqData.Enabled)
	if err != nil {
		logger.Errorf("Cloud115Controller[UpdateNotificationConfig] 更新通知配置失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "更新通知配置失败")
		return
	}
	logger.Infof("Cloud115Controller[UpdateNotificationConfig] 更新通知配置成功: %s", reqData.Channel)
	SuccessResp(ctx, gin.H{"message": "通知配置更新成功", "data": config})
}

func (c *Cloud115Controller) DeleteNotificationConfig(ctx *gin.Context) {
	channel := ctx.Param("channel")
	if err := c.cloud115Service.DeleteNotificationConfig(channel); err != nil {
		logger.Errorf("Cloud115Controller[DeleteNotificationConfig] 删除通知配置失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "删除通知配置失败")
		return
	}
	logger.Infof("Cloud115Controller[DeleteNotificationConfig] 删除通知配置成功: %s", channel)
	SuccessResp(ctx, gin.H{"message": "通知配置删除成功"})
}

func (c *Cloud115Controller) TestNotification(ctx *gin.Context) {
	var reqData struct {
		Channel string `json:"channel" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&reqData); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	config, err := c.cloud115Service.GetNotificationConfigByChannel(reqData.Channel)
	if err != nil || config == nil {
		ErrorResp(ctx, http.StatusNotFound, "通知配置不存在")
		return
	}

	testTitle := "[Easy-STRM] 测试通知"
	testMessage := fmt.Sprintf("这是一条测试通知，渠道: %s\n发送时间: %s", reqData.Channel, time.Now().Format("2006-01-02 15:04:05"))

	if err := c.notificationService.SendToChannel(reqData.Channel, config.Config, testTitle, testMessage); err != nil {
		logger.Errorf("Cloud115Controller[TestNotification] 测试通知发送失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, fmt.Sprintf("测试通知发送失败: %v", err))
		return
	}

	logger.Infof("Cloud115Controller[TestNotification] 测试通知发送成功: %s", reqData.Channel)
	SuccessResp(ctx, gin.H{"message": "测试通知发送成功", "channel": reqData.Channel})
}

func (c *Cloud115Controller) InstantTransfer(ctx *gin.Context) {
	var reqData struct {
		SourceAccountID int    `json:"source_account_id" binding:"required"`
		TargetAccountID int    `json:"target_account_id" binding:"required"`
		FileSHA1        string `json:"file_sha1" binding:"required"`
		FileName        string `json:"file_name"`
		FileSize        int64  `json:"file_size"`
		TargetDirectory string `json:"target_directory"`
	}
	if err := ctx.ShouldBindJSON(&reqData); err != nil {
		logger.Warnf("Cloud115Controller[InstantTransfer] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	sourceFile := &domain.FileInfo{
		Name: reqData.FileName,
		Size: reqData.FileSize,
		Sha1: reqData.FileSHA1,
	}

	logger.Infof("Cloud115Controller[InstantTransfer] 秒传请求: SHA1=%s, source=%d, target=%d",
		reqData.FileSHA1, reqData.SourceAccountID, reqData.TargetAccountID)

	result, err := c.cloud115Service.InstantTransfer(sourceFile, reqData.SourceAccountID, reqData.TargetAccountID, reqData.TargetDirectory)
	if err != nil {
		logger.Errorf("Cloud115Controller[InstantTransfer] 秒传失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	status := "pending"
	if result.Skip {
		status = "skipped"
	} else if result.Success {
		status = "completed"
	}

	SuccessResp(ctx, gin.H{
		"message":    result.Message,
		"file_sha1":  result.SHA1,
		"source_id":  reqData.SourceAccountID,
		"target_id":  reqData.TargetAccountID,
		"status":     status,
		"need_retry": result.NeedRetry,
	})
}

func (c *Cloud115Controller) GetTransferCache(ctx *gin.Context) {
	sha1 := ctx.Param("sha1")
	logger.Debugf("Cloud115Controller[GetTransferCache] SHA1缓存查询: %s", sha1)

	cached, cachedCID, err := c.cloud115Service.GetTransferCache(sha1)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetTransferCache] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"sha1":    sha1,
		"cached":  cached,
		"cid":     cachedCID,
		"message": "SHA1缓存查询成功",
	})
}

// 确保 strconv 被使用（某些旧代码可能引用）
var _ = strconv.Itoa
