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
	NotifyTaskStarted   bool   `json:"notify_task_started"`
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
	NotifyTaskStarted   bool   `json:"notify_task_started"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// WeComConfigView 是不会泄露应用 Secret 的企业微信配置视图。
type WeComConfigView struct {
	Enabled             bool   `json:"enabled"`
	CorpID              string `json:"corp_id"`
	AgentID             string `json:"agent_id"`
	HasSecret           bool   `json:"has_secret"`
	APIBaseURL          string `json:"api_base_url"`
	DetailURL           string `json:"detail_url"`
	ReceiveEnabled      bool   `json:"receive_enabled"`
	HasCallbackToken    bool   `json:"has_callback_token"`
	HasEncodingAESKey   bool   `json:"has_encoding_aes_key"`
	ToUser              string `json:"to_user"`
	ToParty             string `json:"to_party"`
	ToTag               string `json:"to_tag"`
	NotifyTaskStarted   bool   `json:"notify_task_started"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// WeComConfigUpdate 描述企业微信应用配置更新请求。
type WeComConfigUpdate struct {
	Enabled             bool   `json:"enabled"`
	CorpID              string `json:"corp_id"`
	AgentID             string `json:"agent_id"`
	Secret              string `json:"secret"`
	APIBaseURL          string `json:"api_base_url"`
	DetailURL           string `json:"detail_url"`
	ReceiveEnabled      bool   `json:"receive_enabled"`
	CallbackToken       string `json:"callback_token"`
	EncodingAESKey      string `json:"encoding_aes_key"`
	ToUser              string `json:"to_user"`
	ToParty             string `json:"to_party"`
	ToTag               string `json:"to_tag"`
	NotifyTaskStarted   bool   `json:"notify_task_started"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// NotificationConfigService 负责通知渠道配置读取、校验和持久化。
type NotificationConfigService struct {
	dao *dao.NotificationConfigDAO
}

// NotificationEventChannel 描述一个启用渠道的事件订阅和原始发送配置。
type NotificationEventChannel struct {
	Channel             string
	ConfigJSON          string
	NotifyTaskStarted   bool
	NotifyTaskCompleted bool
	NotifyTaskFailed    bool
	NotifyTaskCancelled bool
	NotifyAccountStatus bool
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
		NotifyTaskStarted:   update.NotifyTaskStarted,
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
		NotifyTaskStarted:   config.NotifyTaskStarted,
		NotifyTaskCompleted: config.NotifyTaskCompleted,
		NotifyTaskFailed:    config.NotifyTaskFailed,
		NotifyTaskCancelled: config.NotifyTaskCancelled,
		NotifyAccountStatus: config.NotifyAccountStatus,
	}
}

// GetWeComConfig 读取企业微信应用配置和脱敏视图。
func (s *NotificationConfigService) GetWeComConfig() (WeComConfig, WeComConfigView, error) {
	record, err := s.dao.GetByChannel("wecom")
	if err != nil {
		return WeComConfig{}, WeComConfigView{}, err
	}
	configJSON := "{}"
	enabled := false
	if record != nil {
		configJSON = record.Config
		enabled = record.Enabled
	}
	config, err := ParseWeComConfig(configJSON)
	if err != nil {
		return WeComConfig{}, WeComConfigView{}, fmt.Errorf("企业微信配置解析失败: %v", err)
	}
	return config, buildWeComConfigView(config, enabled), nil
}

// UpdateWeComConfig 保存企业微信应用配置；Secret 为空时保留旧值。
func (s *NotificationConfigService) UpdateWeComConfig(update WeComConfigUpdate) (WeComConfigView, error) {
	current, _, err := s.GetWeComConfig()
	if err != nil {
		return WeComConfigView{}, err
	}
	secret := strings.TrimSpace(update.Secret)
	if secret == "" {
		secret = current.Secret
	}
	callbackToken := strings.TrimSpace(update.CallbackToken)
	if callbackToken == "" {
		callbackToken = current.CallbackToken
	}
	encodingAESKey := strings.TrimSpace(update.EncodingAESKey)
	if encodingAESKey == "" {
		encodingAESKey = current.EncodingAESKey
	}
	agentID := int64(0)
	if strings.TrimSpace(update.AgentID) != "" {
		agentID, err = parseWeComAgentID(update.AgentID)
		if err != nil {
			return WeComConfigView{}, err
		}
	}
	config := WeComConfig{
		CorpID:              strings.TrimSpace(update.CorpID),
		AgentID:             agentID,
		Secret:              secret,
		APIBaseURL:          strings.TrimRight(strings.TrimSpace(update.APIBaseURL), "/"),
		DetailURL:           strings.TrimSpace(update.DetailURL),
		ReceiveEnabled:      update.ReceiveEnabled,
		CallbackToken:       callbackToken,
		EncodingAESKey:      encodingAESKey,
		ToUser:              strings.TrimSpace(update.ToUser),
		ToParty:             strings.TrimSpace(update.ToParty),
		ToTag:               strings.TrimSpace(update.ToTag),
		NotifyTaskStarted:   update.NotifyTaskStarted,
		NotifyTaskCompleted: update.NotifyTaskCompleted,
		NotifyTaskFailed:    update.NotifyTaskFailed,
		NotifyTaskCancelled: update.NotifyTaskCancelled,
		NotifyAccountStatus: update.NotifyAccountStatus,
	}
	if update.Enabled {
		if err := config.Validate(); err != nil {
			return WeComConfigView{}, err
		}
	}
	if update.ReceiveEnabled {
		if err := config.ValidateCallback(); err != nil {
			return WeComConfigView{}, err
		}
		if strings.TrimSpace(config.Secret) == "" || config.AgentID <= 0 {
			return WeComConfigView{}, fmt.Errorf("启用消息接收时必须配置有效的 Agent ID 和应用 Secret，以便回复成员")
		}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return WeComConfigView{}, fmt.Errorf("序列化企业微信配置失败: %v", err)
	}
	if _, err := s.dao.Upsert("wecom", string(encoded), update.Enabled); err != nil {
		return WeComConfigView{}, err
	}
	return buildWeComConfigView(config, update.Enabled), nil
}

func buildWeComConfigView(config WeComConfig, enabled bool) WeComConfigView {
	agentID := ""
	if config.AgentID > 0 {
		agentID = strconv.FormatInt(config.AgentID, 10)
	}
	return WeComConfigView{
		Enabled:             enabled,
		CorpID:              config.CorpID,
		AgentID:             agentID,
		HasSecret:           strings.TrimSpace(config.Secret) != "",
		APIBaseURL:          config.APIBaseURL,
		DetailURL:           config.DetailURL,
		ReceiveEnabled:      config.ReceiveEnabled,
		HasCallbackToken:    strings.TrimSpace(config.CallbackToken) != "",
		HasEncodingAESKey:   strings.TrimSpace(config.EncodingAESKey) != "",
		ToUser:              config.ToUser,
		ToParty:             config.ToParty,
		ToTag:               config.ToTag,
		NotifyTaskStarted:   config.NotifyTaskStarted,
		NotifyTaskCompleted: config.NotifyTaskCompleted,
		NotifyTaskFailed:    config.NotifyTaskFailed,
		NotifyTaskCancelled: config.NotifyTaskCancelled,
		NotifyAccountStatus: config.NotifyAccountStatus,
	}
}

// GetNotificationEventChannels 返回启用且支持事件通知的渠道。
func (s *NotificationConfigService) GetNotificationEventChannels() ([]NotificationEventChannel, error) {
	channels := make([]NotificationEventChannel, 0, 4)
	telegramConfig, telegramView, err := s.GetTelegramConfig()
	if err != nil {
		return nil, err
	}
	if telegramView.Enabled {
		encoded, _ := json.Marshal(telegramConfig)
		channels = append(channels, NotificationEventChannel{
			Channel: "telegram", ConfigJSON: string(encoded),
			NotifyTaskStarted:   telegramConfig.NotifyTaskStarted,
			NotifyTaskCompleted: telegramConfig.NotifyTaskCompleted, NotifyTaskFailed: telegramConfig.NotifyTaskFailed,
			NotifyTaskCancelled: telegramConfig.NotifyTaskCancelled, NotifyAccountStatus: telegramConfig.NotifyAccountStatus,
		})
	}
	weComConfig, weComView, err := s.GetWeComConfig()
	if err != nil {
		return nil, err
	}
	if weComView.Enabled {
		encoded, _ := json.Marshal(weComConfig)
		channels = append(channels, NotificationEventChannel{
			Channel: "wecom", ConfigJSON: string(encoded),
			NotifyTaskStarted:   weComConfig.NotifyTaskStarted,
			NotifyTaskCompleted: weComConfig.NotifyTaskCompleted, NotifyTaskFailed: weComConfig.NotifyTaskFailed,
			NotifyTaskCancelled: weComConfig.NotifyTaskCancelled, NotifyAccountStatus: weComConfig.NotifyAccountStatus,
		})
	}
	legacyConfigs, err := s.dao.GetAllEnabled()
	if err != nil {
		return nil, err
	}
	for _, config := range legacyConfigs {
		if config == nil || config.Channel == "telegram" || config.Channel == "wecom" {
			continue
		}
		if config.Channel != "serverchan" && config.Channel != "email" {
			continue
		}
		channels = append(channels, NotificationEventChannel{
			Channel: config.Channel, ConfigJSON: config.Config,
			NotifyTaskStarted: true, NotifyTaskCompleted: true,
			NotifyTaskFailed: true, NotifyTaskCancelled: true,
		})
	}
	return channels, nil
}
