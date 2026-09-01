package service

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	telegrammodels "github.com/go-telegram/bot/models"
)

const telegramMessageLimit = 4096

// TelegramConfig 定义 Telegram 通知和机器人配置。
type TelegramConfig struct {
	BotToken            string `json:"bot_token"`
	ChatID              string `json:"chat_id"`
	NotifyTaskStarted   bool   `json:"notify_task_started"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// ParseTelegramConfig 解析配置，并只为历史配置中缺失的事件字段应用默认值。
func ParseTelegramConfig(configJSON string) (TelegramConfig, error) {
	var config TelegramConfig
	if strings.TrimSpace(configJSON) == "" {
		configJSON = "{}"
	}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return config, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &fields); err != nil {
		return config, err
	}
	if _, exists := fields["notify_task_completed"]; !exists {
		config.NotifyTaskCompleted = true
	}
	if _, exists := fields["notify_task_started"]; !exists {
		config.NotifyTaskStarted = true
	}
	if _, exists := fields["notify_task_failed"]; !exists {
		config.NotifyTaskFailed = true
	}
	if _, exists := fields["notify_task_cancelled"]; !exists {
		config.NotifyTaskCancelled = true
	}
	if _, exists := fields["notify_account_status"]; !exists {
		config.NotifyAccountStatus = true
	}
	return config, nil
}

// Validate 校验机器人启动和消息发送所需的关键配置。
func (c TelegramConfig) Validate() error {
	if strings.TrimSpace(c.BotToken) == "" {
		return fmt.Errorf("bot_token 不能为空")
	}
	if strings.TrimSpace(c.ChatID) == "" {
		return fmt.Errorf("chat_id 不能为空")
	}
	if _, err := strconv.ParseInt(c.ChatID, 10, 64); err != nil {
		return fmt.Errorf("chat_id 必须是整数")
	}
	return nil
}

// NotificationAction 描述 Telegram 卡片中的内联操作。
type NotificationAction struct {
	Text string
	Data string
}

// NotificationCard 描述跨渠道可复用的结构化通知。
type NotificationCard struct {
	Title   string
	Status  string
	Fields  [][2]string
	Detail  string
	Actions [][]NotificationAction
}

// PlainText 生成 Server酱和邮件使用的纯文本内容。
func (c NotificationCard) PlainText() string {
	var builder strings.Builder
	builder.WriteString(c.Title)
	for _, field := range c.Fields {
		builder.WriteString("\n")
		builder.WriteString(field[0])
		builder.WriteString(": ")
		builder.WriteString(field[1])
	}
	if c.Detail != "" {
		builder.WriteString("\n详情: ")
		builder.WriteString(c.Detail)
	}
	return builder.String()
}

// TelegramText 生成 HTML 富文本，并确保不超过 Telegram 文本限制。
func (c NotificationCard) TelegramText() string {
	var builder strings.Builder
	if c.Status != "" {
		builder.WriteString(htmlEscapeTruncate(c.Status, 16))
		builder.WriteString(" ")
	}
	builder.WriteString("<b>")
	builder.WriteString(htmlEscapeTruncate(c.Title, 256))
	builder.WriteString("</b>")
	fields := c.Fields
	if len(fields) > 7 {
		fields = fields[:7]
	}
	for _, field := range fields {
		builder.WriteString("\n<b>")
		builder.WriteString(htmlEscapeTruncate(field[0], 64))
		builder.WriteString("：</b>")
		builder.WriteString(htmlEscapeTruncate(field[1], 256))
	}
	if c.Detail != "" {
		builder.WriteString("\n\n<blockquote>")
		builder.WriteString(htmlEscapeTruncate(c.Detail, 900))
		builder.WriteString("</blockquote>")
	}
	return builder.String()
}

func htmlEscapeTruncate(value string, limit int) string {
	escaped := html.EscapeString(value)
	if len([]rune(escaped)) <= limit {
		return escaped
	}
	characters := []rune(value)
	low, high := 0, len(characters)
	for low < high {
		mid := (low + high + 1) / 2
		candidate := html.EscapeString(string(characters[:mid]) + "…")
		if len([]rune(candidate)) <= limit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return html.EscapeString(string(characters[:low]) + "…")
}

// TelegramMarkup 将通用动作转换为 Telegram Inline Keyboard。
func (c NotificationCard) TelegramMarkup() *telegrammodels.InlineKeyboardMarkup {
	if len(c.Actions) == 0 {
		return nil
	}
	rows := make([][]telegrammodels.InlineKeyboardButton, 0, len(c.Actions))
	for _, row := range c.Actions {
		buttons := make([]telegrammodels.InlineKeyboardButton, 0, len(row))
		for _, action := range row {
			if action.Text == "" || action.Data == "" || len(action.Data) > 64 {
				continue
			}
			buttons = append(buttons, telegrammodels.InlineKeyboardButton{Text: action.Text, CallbackData: action.Data})
		}
		if len(buttons) > 0 {
			rows = append(rows, buttons)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &telegrammodels.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func truncateRunes(value string, limit int) string {
	characters := []rune(value)
	if len(characters) <= limit {
		return value
	}
	if limit <= 1 {
		return string(characters[:limit])
	}
	return string(characters[:limit-1]) + "…"
}

var notificationTaskTypeNames = map[string]string{
	"strm_generate":           "STRM 文件生成",
	"incremental_sync":        "增量同步",
	"sync_full":               "全量同步",
	"sync_transfer":           "秒传同步",
	"organize":                "媒体整理",
	"watch_auto_organize":     "自动整理",
	"scrape":                  "NFO 刮削",
	"emby_refresh":            "Emby 库刷新",
	"offline_download":        "115 云下载",
	"file_transfer":           "文件传输",
	"share_transfer":          "115 分享转存",
	"share_transfer_organize": "转存后整理",
	"share_transfer_scrape":   "转存后刮削",
}

func notificationTaskTypeName(taskType string) string {
	if name := notificationTaskTypeNames[taskType]; name != "" {
		return name
	}
	return taskType
}
