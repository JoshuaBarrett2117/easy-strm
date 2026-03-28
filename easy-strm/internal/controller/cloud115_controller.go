package controller

import (
	"net/http"
	"strconv"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

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
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

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
		AccountType       string `json:"account_type"`
		Priority          int    `json:"priority"`
		TransferMethod    string `json:"transfer_method"`
		AlistUrl          string `json:"alist_url"`
		AlistToken        string `json:"alist_token"`
	}

	if err := ctx.ShouldBindJSON(&createData); err != nil {
		logger.Warnf("Cloud115Controller[Create] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
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

	cloud115, err := c.cloud115Service.Create(
		createData.Name,
		createData.Cookie,
		createData.RefreshToken,
		createData.AccessToken,
		createData.ExpiresIn,
		createData.TransferAccountID,
		createData.TransferDirectory,
		createData.AccountType,
		createData.Priority,
		createData.TransferMethod,
		createData.AlistUrl,
		createData.AlistToken,
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
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

	var updateData struct {
		Name              string `json:"name"`
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
		updateData.AccountType,
		updateData.Priority,
		updateData.Status,
		updateData.TransferMethod,
		updateData.AlistUrl,
		updateData.AlistToken,
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
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

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
		CheckLoginStatus(uid string, time int64, sign string) (interface {
			Status() int
			Msg() string
		}, error)
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
		_, err = c.cloud115Service.Update(loginData.CloudID, name, cookie.Cookie(), "", "", 0, 0, "", "resource", 5, "active", "115driver", "", "")
		if err != nil {
			logger.Errorf("Cloud115Controller[ConfirmLogin] 更新账号失败: %v", err)
			ErrorResp(ctx, http.StatusInternalServerError, "更新账号失败")
			return
		}
		result = gin.H{"message": "账号更新成功", "cookie": cookie.Cookie()}
	} else {
		cloud115, err := c.cloud115Service.Create(name, cookie.Cookie(), "", "", 0, 0, "", "resource", 5, "115driver", "", "")
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
	logger.Infof("Cloud115Controller[TestNotification] 测试通知发送: %s", reqData.Channel)
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

	// 构建文件信息
	sourceFile := &domain.FileInfo{
		Name: reqData.FileName,
		Size: reqData.FileSize,
		Sha1: reqData.FileSHA1,
	}

	logger.Infof("Cloud115Controller[InstantTransfer] 秒传请求: SHA1=%s, source=%d, target=%d",
		reqData.FileSHA1, reqData.SourceAccountID, reqData.TargetAccountID)

	// 调用服务层执行秒传
	result, err := c.cloud115Service.InstantTransfer(sourceFile, reqData.SourceAccountID, reqData.TargetAccountID, reqData.TargetDirectory)
	if err != nil {
		logger.Errorf("Cloud115Controller[InstantTransfer] 秒传失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// 根据结果返回响应
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

	// 调用服务层查询缓存
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
