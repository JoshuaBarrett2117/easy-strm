package controller

import (
	"fmt"
	"net/http"

	"easy-strm/internal/service"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

type Cloud115Controller struct {
	cloud115Service *service.Cloud115Service
}

func NewCloud115Controller(cloud115Service *service.Cloud115Service) *Cloud115Controller {
	return &Cloud115Controller{
		cloud115Service: cloud115Service,
	}
}

func (c *Cloud115Controller) GetList(ctx *gin.Context) {
	sortField := ctx.DefaultQuery("sort_field", "id")
	sortOrder := ctx.DefaultQuery("sort_order", "ASC")

	list, err := c.cloud115Service.GetAll(sortField, sortOrder)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetList] 获取列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取列表失败")
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  list,
		"total": len(list),
	})
}

func (c *Cloud115Controller) GetByID(ctx *gin.Context) {
	id := ctx.GetInt("id")

	cloud115, err := c.cloud115Service.GetByID(id)
	if err != nil || cloud115 == nil {
		logger.Errorf("Cloud115Controller[GetByID] 获取详情失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "账号不存在")
		return
	}

	SuccessResp(ctx, cloud115)
}

func (c *Cloud115Controller) Create(ctx *gin.Context) {
	var createData struct {
		Name              string `json:"name"`
		Cookie            string `json:"cookie"`
		RefreshToken      string `json:"refresh_token"`
		AccessToken       string `json:"access_token"`
		ExpiresIn         int    `json:"expires_in"`
		TransferAccountID int    `json:"transfer_account_id"`
		TransferDirectory string `json:"transfer_directory"`
	}

	if err := ctx.ShouldBindJSON(&createData); err != nil {
		logger.Warnf("Cloud115Controller[Create] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	cloud115, err := c.cloud115Service.Create(
		createData.Name,
		createData.Cookie,
		createData.RefreshToken,
		createData.AccessToken,
		createData.ExpiresIn,
		createData.TransferAccountID,
		createData.TransferDirectory,
	)
	if err != nil {
		logger.Errorf("Cloud115Controller[Create] 创建失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("Cloud115Controller[Create] 创建账号成功: %s", cloud115.Name)
	SuccessResp(ctx, gin.H{
		"message": "创建成功",
		"data":    cloud115,
	})
}

func (c *Cloud115Controller) Update(ctx *gin.Context) {
	id := ctx.GetInt("id")

	var updateData struct {
		Name              string `json:"name"`
		Cookie            string `json:"cookie"`
		RefreshToken      string `json:"refresh_token"`
		AccessToken       string `json:"access_token"`
		ExpiresIn         int    `json:"expires_in"`
		TransferAccountID int    `json:"transfer_account_id"`
		TransferDirectory string `json:"transfer_directory"`
	}

	if err := ctx.ShouldBindJSON(&updateData); err != nil {
		logger.Warnf("Cloud115Controller[Update] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	cloud115, err := c.cloud115Service.Update(
		id,
		updateData.Name,
		updateData.Cookie,
		updateData.RefreshToken,
		updateData.AccessToken,
		updateData.ExpiresIn,
		updateData.TransferAccountID,
		updateData.TransferDirectory,
	)
	if err != nil {
		logger.Errorf("Cloud115Controller[Update] 更新失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("Cloud115Controller[Update] 更新账号成功: %s (ID: %d)", cloud115.Name, id)
	SuccessResp(ctx, gin.H{
		"message": "更新成功",
		"data":    cloud115,
	})
}

func (c *Cloud115Controller) Delete(ctx *gin.Context) {
	id := ctx.GetInt("id")

	if err := c.cloud115Service.Delete(id); err != nil {
		logger.Errorf("Cloud115Controller[Delete] 删除失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("Cloud115Controller[Delete] 删除账号成功: ID %d", id)
	SuccessResp(ctx, gin.H{
		"message": "删除成功",
	})
}

// GetLoginChannels 获取支持的登录渠道列表
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
	SuccessResp(ctx, channels)
}

// GetQRCode 获取登录二维码
func (c *Cloud115Controller) GetQRCode(ctx *gin.Context) {
	client, ok := ctx.Get("115client")
	if !ok {
		ErrorResp(ctx, http.StatusInternalServerError, "115 client not found")
		return
	}

	qrCodeResp, err := client.(interface {
		GetQRCode() (interface{ UID() string }, error)
	}).GetQRCode()
	if err != nil {
		logger.Errorf("Cloud115Controller[GetQRCode] 获取二维码失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	session, _ := qrCodeResp.(interface{ UID() string })
	InfoResp(ctx, http.StatusOK, "success", gin.H{
		"uid":    session.UID(),
		"qrcode": qrCodeResp,
	})
}

// CheckLoginStatus 检查扫码登录状态
func (c *Cloud115Controller) CheckLoginStatus(ctx *gin.Context) {
	uid := ctx.Query("uid")
	timeStr := ctx.Query("time")
	sign := ctx.Query("sign")

	if uid == "" || timeStr == "" || sign == "" {
		ErrorResp(ctx, http.StatusBadRequest, "uid, time and sign are required")
		return
	}

	client, ok := ctx.Get("115client")
	if !ok {
		ErrorResp(ctx, http.StatusInternalServerError, "115 client not found")
		return
	}

	statusResp, err := client.(interface {
		CheckLoginStatus(uid string, time int64, sign string) (interface{ Status() int; Msg() string }, error)
	}).CheckLoginStatus(uid, 0, sign)
	if err != nil {
		logger.Errorf("Cloud115Controller[CheckLoginStatus] 检查状态失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"status": statusResp.Status(),
		"msg":    statusResp.Msg(),
	})
}

// ConfirmLogin 确认登录并保存凭据
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
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	client, ok := ctx.Get("115client")
	if !ok {
		ErrorResp(ctx, http.StatusInternalServerError, "115 client not found")
		return
	}

	cred, err := client.(interface {
		QRCodeLogin(uid string, time int64, sign string) (interface{ Cookie() string }, error)
		QRCodeLoginWithApp(uid string, time int64, sign string, app string) (interface{ Cookie() string }, error)
	}).QRCodeLogin(loginData.UID, loginData.Time, loginData.Sign)
	if err != nil {
		logger.Errorf("Cloud115Controller[ConfirmLogin] 扫码登录失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	cookie, _ := cred.(interface{ Cookie() string })
	name := loginData.Name
	if name == "" {
		name = "115账号"
	}

	var result interface{}
	if loginData.CloudID > 0 {
		_, err = c.cloud115Service.Update(loginData.CloudID, name, cookie.Cookie(), "", "", 0, 0, "")
		if err != nil {
			logger.Errorf("Cloud115Controller[ConfirmLogin] 更新账号失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, "更新账号失败")
			return
		}
		result = gin.H{"message": "账号更新成功", "cookie": cookie.Cookie()}
	} else {
		cloud115, err := c.cloud115Service.Create(name, cookie.Cookie(), "", "", 0, 0, "")
		if err != nil {
			logger.Errorf("Cloud115Controller[ConfirmLogin] 创建账号失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, "创建账号失败")
			return
		}
		result = gin.H{"message": "账号创建成功", "cookie": cookie.Cookie(), "id": cloud115.ID}
	}

	logger.Infof("Cloud115Controller[ConfirmLogin] 登录确认成功: %s", name)
	SuccessResp(ctx, result)
}

func InfoResp(ctx *gin.Context, httpStatus int, message string, data interface{}) {
	ctx.JSON(httpStatus, Response{
		State:   true,
		Code:    0,
		Message: message,
		Data:    data,
		Error:   "",
		Errno:   0,
	})
}
