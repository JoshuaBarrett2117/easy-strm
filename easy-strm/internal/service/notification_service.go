package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/pkg/logger"
	telegram "github.com/go-telegram/bot"
	telegrammodels "github.com/go-telegram/bot/models"
)

// NotificationService 通知服务，负责通过各渠道发送通知
type NotificationService struct {
	notificationConfigDAO *dao.NotificationConfigDAO
	httpClient            *http.Client
}

// NewNotificationService 创建通知服务实例
// httpClient 使用代理感知客户端以支持 Telegram 等需代理访问的 API
func NewNotificationService(notificationConfigDAO *dao.NotificationConfigDAO, httpClient *http.Client) *NotificationService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &NotificationService{
		notificationConfigDAO: notificationConfigDAO,
		httpClient:            httpClient,
	}
}

// --- 对外接口 ---

// SendNotification 通过所有已启用的渠道发送通知
// 通知失败不影响主流程，仅记录错误日志
func (s *NotificationService) SendNotification(title, message string) error {
	return s.SendCard(NotificationCard{Title: title, Detail: message})
}

// SendCard 通过所有已启用渠道发送结构化通知卡片。
// Telegram 保留富文本和按钮，其他渠道降级为纯文本。
func (s *NotificationService) SendCard(card NotificationCard) error {
	configs, err := s.notificationConfigDAO.GetAllEnabled()
	if err != nil {
		return fmt.Errorf("查询启用配置失败: %v", err)
	}
	if len(configs) == 0 {
		logger.Debugf("[NotificationService] 无启用的通知渠道，跳过发送")
		return nil
	}

	var lastErr error
	for _, cfg := range configs {
		var sendErr error
		if cfg.Channel == "telegram" {
			sendErr = s.sendTelegramCard(cfg.Config, card)
		} else {
			sendErr = s.SendToChannel(cfg.Channel, cfg.Config, card.Title, card.PlainText())
		}
		if sendErr != nil {
			err = sendErr
			logger.Errorf("[NotificationService] 渠道 %s 发送失败: %v", cfg.Channel, err)
			lastErr = err
		} else {
			logger.Infof("[NotificationService] 渠道 %s 发送成功", cfg.Channel)
		}
	}
	return lastErr
}

// SendTelegramCard 仅向已启用的 Telegram 渠道发送卡片。
func (s *NotificationService) SendTelegramCard(card NotificationCard) error {
	config, err := s.notificationConfigDAO.GetByChannel("telegram")
	if err != nil {
		return fmt.Errorf("查询Telegram配置失败: %v", err)
	}
	if config == nil || !config.Enabled {
		return nil
	}
	return s.sendTelegramCard(config.Config, card)
}

// SendToChannel 通过指定渠道发送通知
func (s *NotificationService) SendToChannel(channel, configJSON, title, message string) error {
	switch channel {
	case "telegram":
		return s.sendTelegram(configJSON, title, message)
	case "serverchan":
		return s.sendServerChan(configJSON, title, message)
	case "email":
		return s.sendEmail(configJSON, title, message)
	default:
		return fmt.Errorf("不支持的通知渠道: %s", channel)
	}
}

// SendTaskNotification 发送任务完成通知（格式化后调用 SendNotification）
func (s *NotificationService) SendTaskNotification(taskType, taskName string, success bool, detail string) {
	status := "成功"
	if !success {
		status = "失败"
	}
	title := fmt.Sprintf("[Easy-STRM] %s任务%s", taskType, status)
	message := fmt.Sprintf("任务类型: %s\n任务名称: %s\n执行状态: %s", taskType, taskName, status)
	if detail != "" {
		message += fmt.Sprintf("\n详情: %s", detail)
	}

	if err := s.SendNotification(title, message); err != nil {
		logger.Warnf("[NotificationService] 发送任务通知失败: %v", err)
	}
}

// SendAccountStatusNotification 发送账号状态变更通知
func (s *NotificationService) SendAccountStatusNotification(accountName, oldStatus, newStatus string) {
	title := "[Easy-STRM] 账号状态变更"
	message := fmt.Sprintf("账号: %s\n原状态: %s\n新状态: %s", accountName, oldStatus, newStatus)

	if err := s.SendNotification(title, message); err != nil {
		logger.Warnf("[NotificationService] 发送账号状态通知失败: %v", err)
	}
}

// --- Telegram ---

func (s *NotificationService) sendTelegram(configJSON, title, message string) error {
	return s.sendTelegramCard(configJSON, NotificationCard{Title: title, Detail: message})
}

func (s *NotificationService) sendTelegramCard(configJSON string, card NotificationCard) error {
	cfg, err := ParseTelegramConfig(configJSON)
	if err != nil {
		return fmt.Errorf("Telegram配置解析失败: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("Telegram配置不完整: %v", err)
	}

	client, err := telegram.New(
		cfg.BotToken,
		telegram.WithHTTPClient(10*time.Second, s.httpClient),
		telegram.WithSkipGetMe(),
	)
	if err != nil {
		return fmt.Errorf("Telegram客户端创建失败: %v", err)
	}
	params := &telegram.SendMessageParams{
		ChatID:    cfg.ChatID,
		Text:      card.TelegramText(),
		ParseMode: telegrammodels.ParseModeHTML,
	}
	if markup := card.TelegramMarkup(); markup != nil {
		params.ReplyMarkup = markup
	}
	_, err = client.SendMessage(context.Background(), params)
	if err != nil {
		return fmt.Errorf("Telegram发送失败: %v", err)
	}
	return nil
}

// --- Server酱 ---

type serverChanConfig struct {
	SendKey string `json:"send_key"`
}

func (s *NotificationService) sendServerChan(configJSON, title, message string) error {
	var cfg serverChanConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("Server酱配置解析失败: %v", err)
	}
	if cfg.SendKey == "" {
		return fmt.Errorf("Server酱配置不完整: send_key为空")
	}

	apiURL := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", cfg.SendKey)
	body, _ := json.Marshal(map[string]string{
		"title": title,
		"desp":  message,
	})

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("Server酱创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Server酱请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Server酱返回非200状态码: %d", resp.StatusCode)
	}

	return nil
}

// --- Email (SMTP) ---

type emailConfig struct {
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	UseTLS   bool   `json:"use_tls"`
}

func (s *NotificationService) sendEmail(configJSON, title, message string) error {
	var cfg emailConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("Email配置解析失败: %v", err)
	}
	if cfg.SMTPHost == "" || cfg.Username == "" || cfg.Password == "" || cfg.From == "" || cfg.To == "" {
		return fmt.Errorf("Email配置不完整")
	}
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)

	fromAddr := mail.Address{Name: "Easy-STRM", Address: cfg.From}
	header := map[string]string{
		"From":         fromAddr.String(),
		"To":           cfg.To,
		"Subject":      title,
		"MIME-Version": "1.0",
		"Content-Type": "text/plain; charset=UTF-8",
		"Date":         time.Now().Format(time.RFC1123Z),
	}

	var msg strings.Builder
	for k, v := range header {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(message)

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)

	if cfg.UseTLS {
		return s.sendMailWithTLS(addr, auth, cfg.From, []string{cfg.To}, []byte(msg.String()))
	}
	return smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg.String()))
}

// sendMailWithTLS 通过TLS直连SMTP发送邮件（适用于465端口等隐式TLS场景）
func (s *NotificationService) sendMailWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("Email地址解析失败: %v", err)
	}

	tlsConfig := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("Email TLS连接失败: %v", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("Email SMTP客户端创建失败: %v", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("Email SMTP认证失败: %v", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("Email 设置发件人失败: %v", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("Email 设置收件人失败: %v", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("Email 发送数据失败: %v", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("Email 写入内容失败: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("Email 关闭写入失败: %v", err)
	}
	return client.Quit()
}
