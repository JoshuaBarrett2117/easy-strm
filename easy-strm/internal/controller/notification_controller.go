package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// NotificationController 负责通知渠道配置、测试和 Telegram 机器人状态接口。
type NotificationController struct {
	configs       *service.NotificationConfigService
	notifications *service.NotificationService
	telegramBot   *service.TelegramBotService
}

// NewNotificationController 创建通知控制器。
func NewNotificationController(
	configs *service.NotificationConfigService,
	notifications *service.NotificationService,
	telegramBot *service.TelegramBotService,
) *NotificationController {
	return &NotificationController{configs: configs, notifications: notifications, telegramBot: telegramBot}
}

// GetAll 获取全部通知渠道配置，兼容原有接口。
func (c *NotificationController) GetAll(ctx *gin.Context) {
	list, err := c.configs.GetAll()
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, "获取通知配置失败")
		return
	}
	for _, config := range list {
		if config.Channel == "telegram" {
			config.Config = maskTelegramToken(config.Config)
		}
	}
	SuccessResp(ctx, gin.H{"data": list})
}

// Update 更新指定通知渠道，兼容原有接口。
func (c *NotificationController) Update(ctx *gin.Context) {
	var request struct {
		Channel string `json:"channel" binding:"required"`
		Config  string `json:"config"`
		Enabled bool   `json:"enabled"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	validChannels := map[string]bool{"telegram": true, "serverchan": true, "email": true}
	if !validChannels[request.Channel] {
		ErrorResp(ctx, http.StatusBadRequest, "无效的通知渠道")
		return
	}
	config, err := c.configs.Upsert(request.Channel, request.Config, request.Enabled)
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, "更新通知配置失败")
		return
	}
	if request.Channel == "telegram" {
		config.Config = maskTelegramToken(config.Config)
		if err := c.telegramBot.Reload(); err != nil {
			logger.Warnf("NotificationController[Update] Telegram重载失败: %v", err)
		}
	}
	SuccessResp(ctx, gin.H{"message": "通知配置更新成功", "data": config})
}

func maskTelegramToken(configJSON string) string {
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "{}"
	}
	delete(config, "bot_token")
	encoded, err := json.Marshal(config)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

// Delete 删除通知渠道配置。
func (c *NotificationController) Delete(ctx *gin.Context) {
	channel := ctx.Param("channel")
	if err := c.configs.Delete(channel); err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if channel == "telegram" {
		_ = c.telegramBot.Reload()
	}
	SuccessResp(ctx, gin.H{"message": "通知配置删除成功"})
}

// Test 测试通用通知渠道。
func (c *NotificationController) Test(ctx *gin.Context) {
	var request struct {
		Channel string `json:"channel" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	config, err := c.configs.GetByChannel(request.Channel)
	if err != nil || config == nil {
		ErrorResp(ctx, http.StatusNotFound, "通知配置不存在")
		return
	}
	if err := c.notifications.SendToChannel(request.Channel, config.Config, "[Easy-STRM] 测试通知", fmt.Sprintf("渠道: %s\n发送时间: %s", request.Channel, time.Now().Format("2006-01-02 15:04:05"))); err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, fmt.Sprintf("测试通知发送失败: %v", err))
		return
	}
	SuccessResp(ctx, gin.H{"message": "测试通知发送成功", "channel": request.Channel})
}

// GetTelegramConfig 返回脱敏的 Telegram 配置。
func (c *NotificationController) GetTelegramConfig(ctx *gin.Context) {
	_, view, err := c.configs.GetTelegramConfig()
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	SuccessResp(ctx, view)
}

// UpdateTelegramConfig 更新 Telegram 配置并立即重载机器人。
func (c *NotificationController) UpdateTelegramConfig(ctx *gin.Context) {
	var request service.TelegramConfigUpdate
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}
	view, err := c.configs.UpdateTelegramConfig(request)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := c.telegramBot.Reload(); err != nil && request.Enabled {
		ErrorResp(ctx, http.StatusBadGateway, err.Error())
		return
	}
	SuccessResp(ctx, view)
}

// GetTelegramStatus 返回机器人运行状态。
func (c *NotificationController) GetTelegramStatus(ctx *gin.Context) {
	SuccessResp(ctx, c.telegramBot.Status())
}

// TestTelegram 发送 Telegram 富文本测试卡片。
func (c *NotificationController) TestTelegram(ctx *gin.Context) {
	username, err := c.telegramBot.TestConnection(ctx.Request.Context())
	if err != nil {
		ErrorResp(ctx, http.StatusBadGateway, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"message": "测试通知发送成功", "bot_username": username})
}
