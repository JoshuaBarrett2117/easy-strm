package controller

import (
	"fmt"
	"net/http"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

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
