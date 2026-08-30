package controller

import (
	"net/http"

	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

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
	// 保持与原 auth.go 内联处理器一致的响应格式。
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
		cred, err = c.qrcodeLoginWithApp(session, loginData.App, loginData.Name, loginData.CloudID)
	} else {
		cred, err = c.qrcodeLogin(session, loginData.Name, loginData.CloudID)
	}
	if err != nil {
		logger.Errorf("Cloud115Controller[ConfirmLogin] 扫码登录失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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
