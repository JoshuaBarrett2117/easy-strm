package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// TelegramConfigView 是不会泄露 Bot Token 的前端配置视图。
type TelegramConfigView struct {
	Enabled             bool   `json:"enabled"`
	ChatID              string `json:"chat_id"`
	HasBotToken         bool   `json:"has_bot_token"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// TelegramConfigUpdate 描述 Telegram 专用配置更新请求。
type TelegramConfigUpdate struct {
	Enabled             bool   `json:"enabled"`
	BotToken            string `json:"bot_token"`
	ChatID              string `json:"chat_id"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// NotificationConfigService 负责通知渠道配置读取、校验和持久化。
type NotificationConfigService struct {
	dao *dao.NotificationConfigDAO
}

// NewNotificationConfigService 创建通知配置服务。
func NewNotificationConfigService(configDAO *dao.NotificationConfigDAO) *NotificationConfigService {
	return &NotificationConfigService{dao: configDAO}
}

// GetAll 获取全部通知渠道配置，供兼容接口使用。
func (s *NotificationConfigService) GetAll() ([]*domain.NotificationConfig, error) {
	return s.dao.GetAll()
}

// GetByChannel 按渠道读取原始配置。
func (s *NotificationConfigService) GetByChannel(channel string) (*domain.NotificationConfig, error) {
	return s.dao.GetByChannel(channel)
}

// Upsert 更新兼容接口的渠道配置。
func (s *NotificationConfigService) Upsert(channel, configJSON string, enabled bool) (*domain.NotificationConfig, error) {
	return s.dao.Upsert(channel, configJSON, enabled)
}

// Delete 删除渠道配置。
func (s *NotificationConfigService) Delete(channel string) error {
	return s.dao.Delete(channel)
}

// GetTelegramConfig 读取 Telegram 配置和脱敏视图。
func (s *NotificationConfigService) GetTelegramConfig() (TelegramConfig, TelegramConfigView, error) {
	record, err := s.dao.GetByChannel("telegram")
	if err != nil {
		return TelegramConfig{}, TelegramConfigView{}, err
	}
	configJSON := "{}"
	enabled := false
	if record != nil {
		configJSON = record.Config
		enabled = record.Enabled
	}
	config, err := ParseTelegramConfig(configJSON)
	if err != nil {
		return TelegramConfig{}, TelegramConfigView{}, fmt.Errorf("Telegram配置解析失败: %v", err)
	}
	return config, buildTelegramConfigView(config, enabled), nil
}

// UpdateTelegramConfig 保存 Telegram 配置；BotToken 为空时保留旧值。
func (s *NotificationConfigService) UpdateTelegramConfig(update TelegramConfigUpdate) (TelegramConfigView, error) {
	current, _, err := s.GetTelegramConfig()
	if err != nil {
		return TelegramConfigView{}, err
	}
	token := strings.TrimSpace(update.BotToken)
	if token == "" {
		token = current.BotToken
	}
	config := TelegramConfig{
		BotToken:            token,
		ChatID:              strings.TrimSpace(update.ChatID),
		NotifyTaskCompleted: update.NotifyTaskCompleted,
		NotifyTaskFailed:    update.NotifyTaskFailed,
		NotifyTaskCancelled: update.NotifyTaskCancelled,
		NotifyAccountStatus: update.NotifyAccountStatus,
	}
	if update.Enabled {
		if err := config.Validate(); err != nil {
			return TelegramConfigView{}, err
		}
	} else if config.ChatID != "" {
		if _, err := ParseTelegramChatID(config.ChatID); err != nil {
			return TelegramConfigView{}, err
		}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return TelegramConfigView{}, fmt.Errorf("序列化Telegram配置失败: %v", err)
	}
	if _, err := s.dao.Upsert("telegram", string(encoded), update.Enabled); err != nil {
		return TelegramConfigView{}, err
	}
	return buildTelegramConfigView(config, update.Enabled), nil
}

// ParseTelegramChatID 将配置中的 Chat ID 转为 Telegram API 使用的整数。
func ParseTelegramChatID(value string) (int64, error) {
	chatID, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("chat_id 必须是整数")
	}
	return chatID, nil
}

func buildTelegramConfigView(config TelegramConfig, enabled bool) TelegramConfigView {
	return TelegramConfigView{
		Enabled:             enabled,
		ChatID:              config.ChatID,
		HasBotToken:         strings.TrimSpace(config.BotToken) != "",
		NotifyTaskCompleted: config.NotifyTaskCompleted,
		NotifyTaskFailed:    config.NotifyTaskFailed,
		NotifyTaskCancelled: config.NotifyTaskCancelled,
		NotifyAccountStatus: config.NotifyAccountStatus,
	}
}
