package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/go-redis/redis/v8"
)

const (
	notificationBaselineKey      = "easy_strm:notification:telegram:baseline:v1"
	notificationTaskEventPrefix  = "easy_strm:notification:telegram:task:"
	notificationAccountStatusKey = "easy_strm:notification:telegram:accounts"
	notificationEventTTL         = 24 * time.Hour
)

// NotificationEventMonitor 统一观察 Redis 任务终态和 115 账号状态变化。
type NotificationEventMonitor struct {
	configService telegramConfigReader
	notifications notificationCardSender
	taskDAO       notificationTaskStore
	accountDAO    notificationAccountStore
	redis         *redis.Client
	interval      time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
}

type telegramConfigReader interface {
	GetTelegramConfig() (TelegramConfig, TelegramConfigView, error)
}

type notificationCardSender interface {
	SendTelegramCard(card NotificationCard) error
}

type notificationTaskStore interface {
	GetUnified() ([]map[string]interface{}, error)
}

type notificationAccountStore interface {
	GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error)
}

// NewNotificationEventMonitor 创建通知事件监控器。
func NewNotificationEventMonitor(
	configService telegramConfigReader,
	notifications notificationCardSender,
	taskDAO notificationTaskStore,
	accountDAO notificationAccountStore,
	redisClient *redis.Client,
) *NotificationEventMonitor {
	return &NotificationEventMonitor{
		configService: configService,
		notifications: notifications,
		taskDAO:       taskDAO,
		accountDAO:    accountDAO,
		redis:         redisClient,
		interval:      5 * time.Second,
	}
}

// Start 启动事件轮询；重复调用不会创建多个协程。
func (m *NotificationEventMonitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil || m.redis == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.run(ctx)
}

// Stop 停止事件轮询。
func (m *NotificationEventMonitor) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

func (m *NotificationEventMonitor) run(ctx context.Context) {
	if err := m.runOnce(ctx); err != nil {
		logger.Warnf("[NotificationEventMonitor] 首次检查失败: %v", err)
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.runOnce(ctx); err != nil {
				logger.Warnf("[NotificationEventMonitor] 检查失败: %v", err)
			}
		}
	}
}

func (m *NotificationEventMonitor) runOnce(ctx context.Context) error {
	config, view, err := m.configService.GetTelegramConfig()
	if err != nil || !view.Enabled {
		return err
	}
	tasks, err := m.taskDAO.GetUnified()
	if err != nil {
		return err
	}
	accounts, err := m.accountDAO.GetAll("", "")
	if err != nil {
		return err
	}
	initialized, err := m.redis.Exists(ctx, notificationBaselineKey).Result()
	if err != nil {
		return err
	}
	if initialized == 0 {
		return m.initializeBaseline(ctx, tasks, accounts)
	}
	if err := m.processTasks(ctx, config, tasks); err != nil {
		logger.Warnf("[NotificationEventMonitor] 处理任务通知失败: %v", err)
	}
	return m.processAccounts(ctx, config, accounts)
}

func (m *NotificationEventMonitor) initializeBaseline(ctx context.Context, tasks []map[string]interface{}, accounts []*domain.Cloud115) error {
	pipe := m.redis.TxPipeline()
	for _, task := range tasks {
		status := normalizeTerminalStatus(fmt.Sprint(task["status"]))
		if status == "" {
			continue
		}
		pipe.Set(ctx, taskEventKey(fmt.Sprint(task["task_id"]), status), "1", notificationEventTTL)
	}
	for _, account := range accounts {
		pipe.HSet(ctx, notificationAccountStatusKey, strconv.Itoa(account.ID), account.Status)
	}
	pipe.Expire(ctx, notificationAccountStatusKey, notificationEventTTL)
	pipe.Set(ctx, notificationBaselineKey, time.Now().Format(time.RFC3339), notificationEventTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (m *NotificationEventMonitor) processTasks(ctx context.Context, config TelegramConfig, tasks []map[string]interface{}) error {
	if err := m.redis.Expire(ctx, notificationBaselineKey, notificationEventTTL).Err(); err != nil {
		return err
	}
	for _, task := range tasks {
		status := normalizeTerminalStatus(fmt.Sprint(task["status"]))
		if status == "" {
			continue
		}
		key := taskEventKey(fmt.Sprint(task["task_id"]), status)
		delivered, err := m.redis.Exists(ctx, key).Result()
		if err != nil {
			return err
		}
		if delivered > 0 {
			continue
		}
		if !taskEventEnabled(config, status) {
			_ = m.redis.Set(ctx, key, "disabled", notificationEventTTL).Err()
			continue
		}
		card := buildTaskCard(task, true)
		card.Title = "任务通知 · " + card.Title
		card.Actions = append(card.Actions, []NotificationAction{{Text: "最近任务", Data: "tasks"}})
		if err := m.notifications.SendTelegramCard(card); err != nil {
			return err
		}
		if err := m.redis.Set(ctx, key, "1", notificationEventTTL).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (m *NotificationEventMonitor) processAccounts(ctx context.Context, config TelegramConfig, accounts []*domain.Cloud115) error {
	for _, account := range accounts {
		field := strconv.Itoa(account.ID)
		oldStatus, err := m.redis.HGet(ctx, notificationAccountStatusKey, field).Result()
		if err != nil && err != redis.Nil {
			return err
		}
		if err == redis.Nil {
			oldStatus = account.Status
		}
		if oldStatus != account.Status && config.NotifyAccountStatus {
			icon := "⚠️"
			title := "115 账号状态异常"
			if account.Status == domain.AccountStatusActive {
				icon = "✅"
				title = "115 账号状态已恢复"
			}
			card := NotificationCard{
				Title:   title,
				Status:  icon,
				Fields:  [][2]string{{"账号", account.Name}, {"原状态", accountStatusName(oldStatus)}, {"新状态", accountStatusName(account.Status)}, {"时间", time.Now().Format("2006-01-02 15:04:05")}},
				Actions: [][]NotificationAction{{{Text: "系统状态", Data: "status"}}},
			}
			if err := m.notifications.SendTelegramCard(card); err != nil {
				return err
			}
		}
		if err := m.redis.HSet(ctx, notificationAccountStatusKey, field, account.Status).Err(); err != nil {
			return err
		}
	}
	return m.redis.Expire(ctx, notificationAccountStatusKey, notificationEventTTL).Err()
}

func normalizeTerminalStatus(status string) string {
	switch strings.ToLower(status) {
	case "completed":
		return "completed"
	case "failed", "partial_failed":
		return "failed"
	case "cancelled":
		return "cancelled"
	default:
		return ""
	}
}

func taskEventEnabled(config TelegramConfig, status string) bool {
	switch status {
	case "completed":
		return config.NotifyTaskCompleted
	case "failed":
		return config.NotifyTaskFailed
	case "cancelled":
		return config.NotifyTaskCancelled
	default:
		return false
	}
}

func taskEventKey(taskID, status string) string {
	return notificationTaskEventPrefix + taskID + ":" + status
}

func accountStatusName(status string) string {
	switch status {
	case domain.AccountStatusActive:
		return "正常"
	case domain.AccountStatusCooling:
		return "冷却中"
	case domain.AccountStatusDisabled:
		return "已停用"
	default:
		return status
	}
}
