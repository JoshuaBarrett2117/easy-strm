package controller

import (
	"errors"
	"strconv"

	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// ConfigSecretController 提供登录后按需查看配置的入口。
type ConfigSecretController struct{ service *service.ConfigSecretService }

// NewConfigSecretController 创建配置明文控制器。
func NewConfigSecretController(s *service.ConfigSecretService) *ConfigSecretController {
	return &ConfigSecretController{service: s}
}

// Get 只允许普通设置字段；Emby 字段必须经过管理员路由。
func (c *ConfigSecretController) Get(ctx *gin.Context) {
	key := ctx.Param("key")
	switch key {
	case "ai_recognition_api_key", "tmdb_api_key", "global_api_key", "telegram_bot_token", "wecom_secret", "wecom_callback_token", "wecom_encoding_aes_key":
		c.respond(ctx, key, 0)
	default:
		ErrorResp(ctx, 400, "不支持查看此配置字段")
	}
}

// GetEmby 在既有管理员权限下读取 Emby 实例或 AI 配置。
func (c *ConfigSecretController) GetEmby(ctx *gin.Context) {
	key := ctx.Param("key")
	id := 0
	if key == "emby_server_api_key" {
		var err error
		id, err = strconv.Atoi(ctx.Query("server_id"))
		if err != nil || id <= 0 {
			ErrorResp(ctx, 400, "Emby 实例 ID 无效")
			return
		}
	} else if key != "emby_cover_ai_api_key" {
		ErrorResp(ctx, 400, "不支持查看此配置字段")
		return
	}
	c.respond(ctx, key, id)
}

func (c *ConfigSecretController) respond(ctx *gin.Context, key string, id int) {
	ctx.Header("Cache-Control", "no-store")
	value, err := c.service.Read(key, id)
	if err != nil {
		if errors.Is(err, service.ErrUnknownConfigSecret) {
			ErrorResp(ctx, 400, err.Error())
			return
		}
		ErrorResp(ctx, 500, "读取已保存配置失败，请稍后重试")
		return
	}
	SuccessResp(ctx, gin.H{"value": value})
}
