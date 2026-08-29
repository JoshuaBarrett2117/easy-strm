package service

import (
	"context"
	"testing"

	"easy-strm/internal/domain"
	"github.com/go-redis/redis/v8"
)

type fakeTelegramConfigReader struct {
	config TelegramConfig
	view   TelegramConfigView
}

func (f *fakeTelegramConfigReader) GetTelegramConfig() (TelegramConfig, TelegramConfigView, error) {
	return f.config, f.view, nil
}

type fakeNotificationSender struct{ cards []NotificationCard }

func (f *fakeNotificationSender) SendTelegramCard(card NotificationCard) error {
	f.cards = append(f.cards, card)
	return nil
}

type fakeNotificationTaskStore struct{ tasks []map[string]interface{} }

func (f *fakeNotificationTaskStore) GetUnified() ([]map[string]interface{}, error) {
	return f.tasks, nil
}

type fakeNotificationAccountStore struct{ accounts []*domain.Cloud115 }

func (f *fakeNotificationAccountStore) GetAll(string, string) ([]*domain.Cloud115, error) {
	return f.accounts, nil
}

func TestNotificationEventMonitorBaselineAndDeduplication(t *testing.T) {
	client := newNotificationTestRedis(t)
	config := TelegramConfig{NotifyTaskCompleted: true, NotifyTaskFailed: true, NotifyTaskCancelled: true, NotifyAccountStatus: true}
	reader := &fakeTelegramConfigReader{config: config, view: TelegramConfigView{Enabled: true}}
	sender := &fakeNotificationSender{}
	tasks := &fakeNotificationTaskStore{tasks: []map[string]interface{}{{"task_id": "old", "task_type": "organize", "task_name": "旧任务", "status": "completed"}}}
	accounts := &fakeNotificationAccountStore{accounts: []*domain.Cloud115{{ID: 1, Name: "主号", Status: domain.AccountStatusActive}}}
	monitor := NewNotificationEventMonitor(reader, sender, tasks, accounts, client)

	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.cards) != 0 {
		t.Fatal("首次启用不应补发历史通知")
	}

	tasks.tasks = append(tasks.tasks, map[string]interface{}{
		"task_id": "new", "task_type": "offline_download", "task_name": "新任务", "status": "failed",
		"progress": 100, "success_files": 0, "failed_files": 1, "error_message": "下载失败",
	})
	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.cards) != 1 {
		t.Fatalf("新终态应发送一次，实际 %d", len(sender.cards))
	}
	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.cards) != 1 {
		t.Fatalf("终态不应重复发送，实际 %d", len(sender.cards))
	}

	accounts.accounts[0].Status = domain.AccountStatusCooling
	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.cards) != 2 || sender.cards[1].Title != "115 账号状态异常" {
		t.Fatalf("账号状态变化通知异常: %#v", sender.cards)
	}
}

func newNotificationTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("本地 Redis 不可用: %v", err)
	}
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	return client
}
