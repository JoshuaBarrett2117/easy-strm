package service

import (
	"errors"
	"fmt"

	"easy-strm/internal/dao"
)

// ErrUnknownConfigSecret 表示请求不属于允许查看的配置字段。
var ErrUnknownConfigSecret = errors.New("不支持查看此配置字段")

// ConfigSecretService 按白名单读取已保存的配置明文，不改变配置和脱敏列表。
type ConfigSecretService struct {
	configs       *dao.SystemConfigDAO
	notifications *NotificationConfigService
	servers       *dao.EmbyServerDAO
	tmdbKey       func() string
	ai            *AIRecognitionService
}

// NewConfigSecretService 注入既有配置存储及 TMDB 运行时回退值。
func NewConfigSecretService(configs *dao.SystemConfigDAO, notifications *NotificationConfigService, servers *dao.EmbyServerDAO, tmdbKey func() string, ai *AIRecognitionService) *ConfigSecretService {
	return &ConfigSecretService{configs: configs, notifications: notifications, servers: servers, tmdbKey: tmdbKey, ai: ai}
}

// Read 返回指定配置的完整值；未配置返回空字符串，存储错误向上传递。
func (s *ConfigSecretService) Read(key string, serverID int) (string, error) {
	switch key {
	case "tmdb_api_key", "global_api_key", "emby_cover_ai_api_key":
		config, err := s.configs.GetByKey(key)
		if err != nil {
			return "", err
		}
		if config != nil && config.ConfigVal != "" {
			return config.ConfigVal, nil
		}
		if key == "tmdb_api_key" {
			return s.tmdbKey(), nil
		}
		return "", nil
	case "ai_recognition_api_key":
		config, err := s.ai.config()
		return config.APIKey, err
	case "telegram_bot_token":
		config, _, err := s.notifications.GetTelegramConfig()
		return config.BotToken, err
	case "wecom_secret", "wecom_callback_token", "wecom_encoding_aes_key":
		config, _, err := s.notifications.GetWeComConfig()
		if err != nil {
			return "", err
		}
		switch key {
		case "wecom_secret":
			return config.Secret, nil
		case "wecom_callback_token":
			return config.CallbackToken, nil
		default:
			return config.EncodingAESKey, nil
		}
	case "emby_server_api_key":
		if serverID <= 0 {
			return "", fmt.Errorf("Emby 实例 ID 无效")
		}
		server, err := s.servers.GetByID(serverID)
		if err != nil {
			return "", err
		}
		if server == nil {
			return "", fmt.Errorf("Emby 实例不存在")
		}
		return server.APIKey, nil
	default:
		return "", ErrUnknownConfigSecret
	}
}
