package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"easy-strm/internal/pkg/logger"
	"github.com/go-redis/redis/v8"
	"github.com/sbzhu/weworkapi_golang/wxbizmsgcrypt"
)

const weComIncomingEventTTL = 24 * time.Hour

type weComCallbackConfigReader interface {
	GetWeComConfig() (WeComConfig, WeComConfigView, error)
}

type weComReplySender interface {
	SendWeComCardToUser(config WeComConfig, userID string, card NotificationCard) error
}

type weComDashboardReader interface {
	GetDashboardStats() (*DashboardStats, error)
}

type weComTaskReader interface {
	GetUnified() ([]map[string]interface{}, error)
}

type weComResourceExecutor interface {
	Execute(ctx context.Context, text string) (card NotificationCard, handled bool, err error)
}

// WeComIncomingMessage 描述解密后的企业微信应用消息或事件。
type WeComIncomingMessage struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgID        int64  `xml:"MsgId"`
	AgentID      int64  `xml:"AgentID"`
	Event        string `xml:"Event"`
	EventKey     string `xml:"EventKey"`
}

// WeComCallbackService 负责企业微信回调校验、消息解密、去重和应用回复。
type WeComCallbackService struct {
	configs      weComCallbackConfigReader
	replies      weComReplySender
	dashboard    weComDashboardReader
	tasks        weComTaskReader
	resources    weComResourceExecutor
	redis        *redis.Client
	replyTimeout time.Duration
}

// NewWeComCallbackService 创建企业微信 API 接收消息服务。
func NewWeComCallbackService(
	configs weComCallbackConfigReader,
	replies weComReplySender,
	dashboard weComDashboardReader,
	tasks weComTaskReader,
	redisClient *redis.Client,
) *WeComCallbackService {
	return &WeComCallbackService{
		configs: configs, replies: replies, dashboard: dashboard, tasks: tasks,
		redis: redisClient, replyTimeout: 2 * time.Minute,
	}
}

// SetResourceService 注入与 Telegram 共用的 115 分享转存和云下载执行器。
func (s *WeComCallbackService) SetResourceService(resources weComResourceExecutor) {
	s.resources = resources
}

// VerifyURL 校验企业微信后台保存回调地址时的请求，并返回解密后的 echostr。
func (s *WeComCallbackService) VerifyURL(signature, timestamp, nonce, echo string) (string, error) {
	config, err := s.getEnabledConfig()
	if err != nil {
		return "", err
	}
	plain, cryptErr := newWeComMessageCrypt(config).VerifyURL(signature, timestamp, nonce, echo)
	if cryptErr != nil {
		return "", fmt.Errorf("企业微信回调 URL 校验失败(%d): %s", cryptErr.ErrCode, cryptErr.ErrMsg)
	}
	return string(plain), nil
}

// Receive 解密并接收企业微信推送；业务回复异步发送，接口可立即返回 success。
func (s *WeComCallbackService) Receive(signature, timestamp, nonce string, body []byte) error {
	config, err := s.getEnabledConfig()
	if err != nil {
		return err
	}
	plain, cryptErr := newWeComMessageCrypt(config).DecryptMsg(signature, timestamp, nonce, body)
	if cryptErr != nil {
		return fmt.Errorf("企业微信消息解密失败(%d): %s", cryptErr.ErrCode, cryptErr.ErrMsg)
	}
	var message WeComIncomingMessage
	if err := xml.Unmarshal(plain, &message); err != nil {
		return fmt.Errorf("企业微信消息解析失败: %v", err)
	}
	if strings.TrimSpace(message.FromUserName) == "" {
		return fmt.Errorf("企业微信消息缺少发送成员")
	}
	if message.AgentID > 0 && config.AgentID > 0 && message.AgentID != config.AgentID {
		return fmt.Errorf("企业微信消息 AgentID 不匹配")
	}
	if !s.claimMessage(message, plain) {
		return nil
	}
	if strings.EqualFold(message.MsgType, "text") {
		go s.replyToText(config, message.FromUserName, message.Content)
	}
	return nil
}

func (s *WeComCallbackService) getEnabledConfig() (WeComConfig, error) {
	config, _, err := s.configs.GetWeComConfig()
	if err != nil {
		return WeComConfig{}, err
	}
	if !config.ReceiveEnabled {
		return WeComConfig{}, fmt.Errorf("企业微信 API 接收消息未启用")
	}
	if err := config.ValidateCallback(); err != nil {
		return WeComConfig{}, err
	}
	return config, nil
}

func newWeComMessageCrypt(config WeComConfig) *wxbizmsgcrypt.WXBizMsgCrypt {
	return wxbizmsgcrypt.NewWXBizMsgCrypt(
		strings.TrimSpace(config.CallbackToken),
		strings.TrimSpace(config.EncodingAESKey),
		strings.TrimSpace(config.CorpID),
		wxbizmsgcrypt.XmlType,
	)
}

func (s *WeComCallbackService) claimMessage(message WeComIncomingMessage, plain []byte) bool {
	if s.redis == nil {
		return true
	}
	identity := strconv.FormatInt(message.MsgID, 10)
	if message.MsgID == 0 {
		digest := sha256.Sum256(plain)
		identity = hex.EncodeToString(digest[:])
	}
	key := "easy_strm:notification:wecom:incoming:" + identity
	claimed, err := s.redis.SetNX(context.Background(), key, "1", weComIncomingEventTTL).Result()
	if err != nil {
		logger.Warnf("[WeComCallbackService] 消息去重失败，继续处理: %v", err)
		return true
	}
	return claimed
}

func (s *WeComCallbackService) replyToText(config WeComConfig, userID, content string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.replyTimeout)
	defer cancel()
	card, err := s.buildReply(ctx, content)
	if err != nil {
		card = NotificationCard{Title: "操作失败", Status: "❌", Detail: err.Error()}
	}
	if err := s.replies.SendWeComCardToUser(config, userID, card); err != nil {
		logger.Warnf("[WeComCallbackService] 回复成员 %s 失败: %v", userID, err)
	}
}

func (s *WeComCallbackService) buildReply(ctx context.Context, content string) (NotificationCard, error) {
	command := strings.ToLower(strings.TrimSpace(content))
	command = strings.TrimPrefix(command, "/")
	switch command {
	case "status", "状态", "系统状态":
		return s.buildStatusCard()
	case "tasks", "任务", "最近任务":
		return s.buildTasksCard()
	case "help", "帮助", "start":
		return buildWeComHelpCard(), nil
	}
	if s.resources != nil {
		card, handled, err := s.resources.Execute(ctx, content)
		if handled {
			return card, err
		}
	}
	return buildWeComHelpCard(), nil
}

func (s *WeComCallbackService) buildStatusCard() (NotificationCard, error) {
	if s.dashboard == nil {
		return NotificationCard{}, fmt.Errorf("系统状态服务未初始化")
	}
	stats, err := s.dashboard.GetDashboardStats()
	if err != nil {
		return NotificationCard{}, fmt.Errorf("读取系统状态失败: %v", err)
	}
	return NotificationCard{
		Title:  "系统概览",
		Status: "📊",
		Fields: [][2]string{
			{"115 账号", fmt.Sprintf("%d/%d 活跃", stats.Accounts.Active, stats.Accounts.Total)},
			{"媒体源", fmt.Sprintf("%d/%d 启用", stats.MediaSources.Enabled, stats.MediaSources.Total)},
			{"任务", fmt.Sprintf("运行 %d · 今日成功 %d · 失败 %d", stats.Tasks.Running, stats.Tasks.CompletedToday, stats.Tasks.FailedToday)},
			{"STRM 文件", strconv.Itoa(stats.StrmFiles.Total)},
		},
	}, nil
}

func (s *WeComCallbackService) buildTasksCard() (NotificationCard, error) {
	if s.tasks == nil {
		return NotificationCard{}, fmt.Errorf("任务服务未初始化")
	}
	tasks, err := s.tasks.GetUnified()
	if err != nil {
		return NotificationCard{}, fmt.Errorf("读取任务失败: %v", err)
	}
	if len(tasks) == 0 {
		return NotificationCard{Title: "最近任务", Status: "📭", Detail: "暂无任务记录"}, nil
	}
	limit := len(tasks)
	if limit > 5 {
		limit = 5
	}
	fields := make([][2]string, 0, limit)
	for _, task := range tasks[:limit] {
		name := strings.TrimSpace(fmt.Sprint(task["task_name"]))
		if name == "" || name == "<nil>" {
			name = notificationTaskTypeName(fmt.Sprint(task["task_type"]))
		}
		fields = append(fields, [2]string{name, notificationTaskStatusName(fmt.Sprint(task["status"]))})
	}
	return NotificationCard{Title: "最近任务", Status: "📋", Fields: fields}, nil
}

func buildWeComHelpCard() NotificationCard {
	return NotificationCard{
		Title:  "Easy-STRM 企业微信助手",
		Status: "🤖",
		Detail: "发送“状态”查看系统概览，发送“任务”查看最近任务，发送“帮助”查看本说明。\n\n也可直接发送 115 分享链接或 magnet、ed2k、HTTP、HTTPS、FTP 链接创建转存或云下载任务；链接后可追加账号名称和目标目录。",
	}
}

func notificationTaskStatusName(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "等待中"
	case "running", "processing":
		return "运行中"
	case "completed", "success":
		return "成功"
	case "failed", "partial_failed", "partial_success", "unknown":
		return "失败"
	case "cancelled", "canceled":
		return "已取消"
	default:
		return status
	}
}
