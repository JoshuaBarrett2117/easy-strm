package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type fakeTelegramConfigReader struct {
	config TelegramConfig
	view   TelegramConfigView
}

func (f *fakeTelegramConfigReader) GetTelegramConfig() (TelegramConfig, TelegramConfigView, error) {
	return f.config, f.view, nil
}

type fakeNotificationSender struct {
	mu      sync.Mutex
	cards   []NotificationCard
	delay   time.Duration
	sendErr error
}

func (f *fakeNotificationSender) SendTelegramCard(card NotificationCard) error {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.sendErr != nil {
		return f.sendErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cards = append(f.cards, card)
	return nil
}

func (f *fakeNotificationSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.cards)
}

type fakeNotificationTaskStore struct{ tasks []map[string]interface{} }

func (f *fakeNotificationTaskStore) GetUnified() ([]map[string]interface{}, error) {
	return f.tasks, nil
}

func TestNotificationEventMonitorConcurrentTaskDeduplication(t *testing.T) {
	client := newNotificationTestRedis(t)
	config := TelegramConfig{NotifyTaskCompleted: true}
	sender := &fakeNotificationSender{delay: 50 * time.Millisecond}
	monitor := NewNotificationEventMonitor(nil, sender, nil, nil, client)
	tasks := []map[string]interface{}{{
		"task_id": "same-task", "task_type": "strm", "task_name": "并发任务", "status": "completed",
	}}

	const workers = 8
	start := make(chan struct{})
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errorsCh <- monitor.processTasks(context.Background(), config, tasks)
		}()
	}
	close(start)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := sender.count(); got != 1 {
		t.Fatalf("并发检查同一终态只能发送一次，实际 %d", got)
	}
}

func TestNotificationEventMonitorReleasesClaimAfterSendFailure(t *testing.T) {
	client := newNotificationTestRedis(t)
	sender := &fakeNotificationSender{sendErr: errors.New("发送失败")}
	monitor := NewNotificationEventMonitor(nil, sender, nil, nil, client)
	tasks := []map[string]interface{}{{"task_id": "retry-task", "status": "completed"}}

	err := monitor.processTasks(context.Background(), TelegramConfig{NotifyTaskCompleted: true}, tasks)
	if err == nil {
		t.Fatal("发送失败应返回错误")
	}
	exists, redisErr := client.Exists(context.Background(), taskEventKey("retry-task", "completed")).Result()
	if redisErr != nil {
		t.Fatal(redisErr)
	}
	if exists != 0 {
		t.Fatal("发送失败后应释放事件抢占")
	}
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
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}
