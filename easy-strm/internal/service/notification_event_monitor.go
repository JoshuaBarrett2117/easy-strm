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
	notificationNamespace     = "easy_strm:notification:"
	notificationEventTTL      = 24 * time.Hour
	notificationEventClaimTTL = time.Minute
)

// NotificationEventMonitor 统一观察 Redis 任务触发、终态和 115 账号状态变化。
type NotificationEventMonitor struct {
	configService notificationEventConfigReader
	notifications notificationCardSender
	taskDAO       notificationTaskStore
	accountDAO    notificationAccountStore
	redis         *redis.Client
	interval      time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
}

type notificationEventConfigReader interface {
	GetNotificationEventChannels() ([]NotificationEventChannel, error)
}

type notificationCardSender interface {
	SendCardToChannel(channel, configJSON string, card NotificationCard) error
}

type notificationTaskStore interface {
	GetUnified() ([]map[string]interface{}, error)
}

type notificationAccountStore interface {
	GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error)
}

// NewNotificationEventMonitor 创建通知事件监控器。
func NewNotificationEventMonitor(
	configService notificationEventConfigReader,
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
	channels, err := m.configService.GetNotificationEventChannels()
	if err != nil || len(channels) == 0 {
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
	for _, channel := range channels {
		initialized, checkErr := m.redis.Exists(ctx, notificationBaselineKey(channel.Channel)).Result()
		if checkErr != nil {
			return checkErr
		}
		if initialized == 0 {
			if initErr := m.initializeBaseline(ctx, channel.Channel, tasks, accounts); initErr != nil {
				return initErr
			}
			continue
		}
		if processErr := m.processTasks(ctx, channel, tasks); processErr != nil {
			logger.Warnf("[NotificationEventMonitor] 渠道 %s 处理任务通知失败: %v", channel.Channel, processErr)
		}
		if processErr := m.processAccounts(ctx, channel, accounts); processErr != nil {
			logger.Warnf("[NotificationEventMonitor] 渠道 %s 处理账号通知失败: %v", channel.Channel, processErr)
		}
	}
	return nil
}

func (m *NotificationEventMonitor) initializeBaseline(ctx context.Context, channel string, tasks []map[string]interface{}, accounts []*domain.Cloud115) error {
	pipe := m.redis.TxPipeline()
	for _, task := range tasks {
		taskID := strings.TrimSpace(fmt.Sprint(task["task_id"]))
		if taskID == "" {
			continue
		}
		pipe.Set(ctx, taskEventKeyForChannel(channel, taskID, "started"), "1", notificationEventTTL)
		status := normalizeTerminalStatus(fmt.Sprint(task["status"]))
		if status == "" {
			continue
		}
		pipe.Set(ctx, taskEventKeyForChannel(channel, taskID, status), "1", notificationEventTTL)
	}
	for _, account := range accounts {
		pipe.HSet(ctx, notificationAccountStatusKey(channel), strconv.Itoa(account.ID), account.Status)
	}
	pipe.Expire(ctx, notificationAccountStatusKey(channel), notificationEventTTL)
	pipe.Set(ctx, notificationBaselineKey(channel), time.Now().Format(time.RFC3339), notificationEventTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (m *NotificationEventMonitor) processTasks(ctx context.Context, channel NotificationEventChannel, tasks []map[string]interface{}) error {
	if err := m.redis.Expire(ctx, notificationBaselineKey(channel.Channel), notificationEventTTL).Err(); err != nil {
		return err
	}
	for _, task := range tasks {
		taskID := strings.TrimSpace(fmt.Sprint(task["task_id"]))
		if taskID == "" {
			continue
		}
		if err := m.processTaskEvent(ctx, channel, task, "started"); err != nil {
			return err
		}
		if status := normalizeTerminalStatus(fmt.Sprint(task["status"])); status != "" {
			if err := m.processTaskEvent(ctx, channel, task, status); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *NotificationEventMonitor) processTaskEvent(ctx context.Context, channel NotificationEventChannel, task map[string]interface{}, event string) error {
	taskID := strings.TrimSpace(fmt.Sprint(task["task_id"]))
	key := taskEventKeyForChannel(channel.Channel, taskID, event)
	claimed, err := m.redis.SetNX(ctx, key, "processing", notificationEventClaimTTL).Result()
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	if !taskEventEnabled(channel, event) {
		if err := m.redis.Set(ctx, key, "disabled", notificationEventTTL).Err(); err != nil {
			return err
		}
		return nil
	}
	card := buildTaskEventCard(task, event)
	card.Actions = append(card.Actions, []NotificationAction{{Text: "最近任务", Data: "tasks"}})
	if err := m.notifications.SendCardToChannel(channel.Channel, channel.ConfigJSON, card); err != nil {
		// 发送失败时释放抢占，允许下一轮重新尝试该事件。
		_ = m.redis.Del(ctx, key).Err()
		return err
	}
	if err := m.redis.Set(ctx, key, "1", notificationEventTTL).Err(); err != nil {
		return err
	}
	return nil
}

func buildTaskEventCard(task map[string]interface{}, event string) NotificationCard {
	if event == "started" {
		taskID := strings.TrimSpace(fmt.Sprint(task["task_id"]))
		taskName := strings.TrimSpace(fmt.Sprint(task["task_name"]))
		if taskName == "" || taskName == "<nil>" {
			taskName = notificationTaskTypeName(fmt.Sprint(task["task_type"]))
		}
		fields := [][2]string{
			{"类型", notificationTaskTypeName(fmt.Sprint(task["task_type"]))},
			{"状态", "已触发"},
			{"任务 ID", taskID},
		}
		if createdAt := strings.TrimSpace(fmt.Sprint(task["create_time"])); createdAt != "" && createdAt != "<nil>" {
			fields = append(fields, [2]string{"触发时间", createdAt})
		}
		actions := [][]NotificationAction{{{Text: "详情", Data: "task:detail:" + taskID}}}
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(task["status"])))
		if (status == "pending" || status == "running" || status == "processing") && len("task:cancel:"+taskID) <= 64 {
			actions = append(actions, []NotificationAction{{Text: "取消任务", Data: "task:cancel:" + taskID}})
		}
		return NotificationCard{Title: "任务已触发 · " + taskName, Status: "▶️", Fields: fields, Actions: actions}
	}
	card := buildTaskCard(task, true)
	titlePrefix := map[string]string{"completed": "任务已完成 · ", "failed": "任务已失败 · ", "cancelled": "任务已取消 · "}[event]
	card.Title = titlePrefix + card.Title
	return card
}

func (m *NotificationEventMonitor) processAccounts(ctx context.Context, channel NotificationEventChannel, accounts []*domain.Cloud115) error {
	accountStatusKey := notificationAccountStatusKey(channel.Channel)
	for _, account := range accounts {
		field := strconv.Itoa(account.ID)
		oldStatus, err := m.redis.HGet(ctx, accountStatusKey, field).Result()
		if err != nil && err != redis.Nil {
			return err
		}
		if err == redis.Nil {
			oldStatus = account.Status
		}
		if oldStatus != account.Status && channel.NotifyAccountStatus {
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
			if err := m.notifications.SendCardToChannel(channel.Channel, channel.ConfigJSON, card); err != nil {
				return err
			}
		}
		if err := m.redis.HSet(ctx, accountStatusKey, field, account.Status).Err(); err != nil {
			return err
		}
	}
	return m.redis.Expire(ctx, accountStatusKey, notificationEventTTL).Err()
}

func normalizeTerminalStatus(status string) string {
	switch strings.ToLower(status) {
	case "completed", "success":
		return "completed"
	case "failed", "partial_failed", "partial_success", "unknown":
		return "failed"
	case "cancelled":
		return "cancelled"
	default:
		return ""
	}
}

func taskEventEnabled(config NotificationEventChannel, status string) bool {
	switch status {
	case "started":
		return config.NotifyTaskStarted
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

func notificationBaselineKey(channel string) string {
	return notificationNamespace + channel + ":baseline:v2"
}

func notificationAccountStatusKey(channel string) string {
	return notificationNamespace + channel + ":accounts"
}

func taskEventKeyForChannel(channel, taskID, status string) string {
	return notificationNamespace + channel + ":task:" + taskID + ":" + status
}

func taskEventKey(taskID, status string) string {
	return taskEventKeyForChannel("telegram", taskID, status)
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
