package service

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type fakeNotificationScopeConfigs struct {
	values   map[string]string
	readErr  error
	writeErr error
	writes   int
}

func (store *fakeNotificationScopeConfigs) GetHotConfigs(context.Context, ...string) (map[string]string, error) {
	return store.values, store.readErr
}

func (store *fakeNotificationScopeConfigs) BatchUpsertContext(ctx context.Context, values map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.writes++
	if store.writeErr != nil {
		return store.writeErr
	}
	store.values = values
	return nil
}

type fakeNotificationScopeCron struct {
	tasks []*domain.CronTask
	err   error
	reads atomic.Int32
}

func (store *fakeNotificationScopeCron) GetNotificationTasks(context.Context) ([]*domain.CronTask, error) {
	store.reads.Add(1)
	return store.tasks, store.err
}

func notificationScopeCronFixture() *fakeNotificationScopeCron {
	return &fakeNotificationScopeCron{tasks: []*domain.CronTask{
		{ID: 1, TaskKey: "key:a", TaskName: "任务甲", Handler: "shared", Status: "enabled"},
		{ID: 2, TaskKey: "key:b", TaskName: "任务乙", Handler: "shared", Status: "disabled"},
		{ID: 3, TaskName: "旧任务", Handler: "legacy", Status: "enabled"},
	}}
}

func newNotificationTestScope() *NotificationTaskScopeService {
	return NewNotificationTaskScopeService(&fakeNotificationScopeConfigs{}, &fakeNotificationScopeCron{})
}

func TestNotificationTaskScopeCompatibility(t *testing.T) {
	for _, test := range []struct {
		name    string
		values  map[string]string
		want    []string
		all     bool
		invalid bool
	}{
		{name: "missing", all: true, want: []string{"key:a", "key:b", "handler:legacy"}},
		{name: "blank", values: map[string]string{notifyTaskKeysKey: " \n "}, all: true, want: []string{"key:a", "key:b", "handler:legacy"}},
		{name: "legacy empty", values: map[string]string{notifyTaskKeysKey: "[]"}, all: true, want: []string{"key:a", "key:b", "handler:legacy"}},
		{name: "legacy restricted", values: map[string]string{notifyTaskKeysKey: `["key:a"]`}, want: []string{"key:a"}},
		{name: "explicit none", values: map[string]string{notifyTaskKeysKey: "[]", notifyTaskScopeConfiguredKey: "true"}, want: []string{}},
		{name: "stale remains restricted", values: map[string]string{notifyTaskKeysKey: `["deleted"]`}, want: []string{}},
		{name: "broken json", values: map[string]string{notifyTaskKeysKey: "["}, invalid: true},
		{name: "null", values: map[string]string{notifyTaskKeysKey: "null"}, invalid: true},
		{name: "wrong element", values: map[string]string{notifyTaskKeysKey: `[1]`}, invalid: true},
		{name: "null element", values: map[string]string{notifyTaskKeysKey: `[null]`}, invalid: true},
		{name: "empty key", values: map[string]string{notifyTaskKeysKey: `[""]`}, invalid: true},
		{name: "broken marker", values: map[string]string{notifyTaskScopeConfiguredKey: "false"}, invalid: true},
		{name: "lost configured array", values: map[string]string{notifyTaskScopeConfiguredKey: "true"}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope := NewNotificationTaskScopeService(&fakeNotificationScopeConfigs{values: test.values}, notificationScopeCronFixture())
			view, err := scope.Get(context.Background())
			if test.invalid {
				if err == nil {
					t.Fatal("损坏配置不能扩大通知范围")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(view.Selected, test.want) || len(view.Tasks) != 3 {
				t.Fatalf("view=%+v err=%v", view, err)
			}
			snapshot, err := scope.snapshot(context.Background())
			if err != nil || snapshot.all != test.all {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
		})
	}
}

func TestNotificationTaskScopePersistenceAndValidation(t *testing.T) {
	configs := &fakeNotificationScopeConfigs{}
	cron := notificationScopeCronFixture()
	scope := NewNotificationTaskScopeService(configs, cron)
	for _, selected := range [][]string{nil, {"1"}, {"handler:shared"}, {"deleted"}, {""}, {" key:a"}} {
		if _, err := scope.Update(context.Background(), selected); !errors.Is(err, ErrInvalidNotificationTaskSelection) {
			t.Fatalf("接受无效选择 %v: %v", selected, err)
		}
	}
	if configs.writes != 0 {
		t.Fatal("无效选择不应写入")
	}
	for _, test := range []struct {
		selected []string
		encoded  string
	}{
		{[]string{"key:a", "key:a", "handler:legacy", "key:b"}, `["key:a","handler:legacy","key:b"]`},
		{[]string{}, `[]`},
	} {
		before := configs.writes
		if _, err := scope.Update(context.Background(), test.selected); err != nil {
			t.Fatal(err)
		}
		if configs.writes != before+1 || !reflect.DeepEqual(configs.values, map[string]string{notifyTaskKeysKey: test.encoded, notifyTaskScopeConfiguredKey: "true"}) {
			t.Fatalf("必须原子保存数组和标记: %v", configs.values)
		}
	}
	cron.tasks = cron.tasks[1:]
	if _, err := scope.Update(context.Background(), []string{"key:a"}); !errors.Is(err, ErrInvalidNotificationTaskSelection) {
		t.Fatal("删除任务后的旧选择必须拒绝")
	}
	configs.writeErr = errors.New("write unavailable")
	if _, err := scope.Update(context.Background(), []string{}); err == nil {
		t.Fatal("必须返回写入错误")
	}
	configs.readErr = errors.New("read unavailable")
	if _, err := scope.Get(context.Background()); err == nil {
		t.Fatal("必须返回读取错误")
	}
	configs.readErr = nil
	cron.err = errors.New("cron unavailable")
	if _, err := scope.Get(context.Background()); err == nil {
		t.Fatal("必须返回任务表读取错误")
	}
	if _, err := scope.Update(context.Background(), []string{}); err == nil {
		t.Fatal("保存也必须校验当前任务表")
	}
}

func TestNotificationTaskScopeMonitorMatrix(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]string
		count  int
	}{
		{"missing", nil, 8},
		{"legacy empty", map[string]string{notifyTaskKeysKey: `[]`}, 8},
		{"selected", map[string]string{notifyTaskKeysKey: `["key:a"]`}, 4},
		{"explicit none", map[string]string{notifyTaskKeysKey: `[]`, notifyTaskScopeConfiguredKey: "true"}, 0},
		{"malformed", map[string]string{notifyTaskKeysKey: `broken`}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newNotificationTestRedis(t)
			tasks := dao.NewTaskRedisDAO(client)
			configs := &fakeNotificationScopeConfigs{values: test.values}
			cron := notificationScopeCronFixture()
			sender := &fakeNotificationSender{}
			channel := NotificationEventChannel{Channel: "telegram", NotifyTaskStarted: true, NotifyTaskCompleted: true, NotifyTaskFailed: true}
			monitor := NewNotificationEventMonitor(&fakeTelegramConfigReader{channels: []NotificationEventChannel{channel}}, sender, tasks, &fakeNotificationAccountStore{}, client, NewNotificationTaskScopeService(configs, cron))
			if err := monitor.runOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, execution := range []struct{ id, key, status string }{
				{"a-completed", "key:a", "completed"}, {"a-failed", "key:a", "failed"},
				{"b-completed", "key:b", "completed"}, {"b-failed", "key:b", "failed"},
			} {
				if err := tasks.Create(execution.id, "shared", execution.id); err != nil {
					t.Fatal(err)
				}
				if err := tasks.UpdateMetadata(execution.id, map[string]interface{}{"cron_task_key": execution.key, "cron_handler": "shared"}); err != nil {
					t.Fatal(err)
				}
				if err := tasks.UpdateStatus(execution.id, execution.status); err != nil {
					t.Fatal(err)
				}
			}
			for repeat := 0; repeat < 2; repeat++ {
				if err := monitor.runOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if sender.count() != test.count {
				t.Fatalf("cards=%d want=%d", sender.count(), test.count)
			}
			if cron.reads.Load() > 3 {
				t.Fatalf("不应逐任务查询: %d", cron.reads.Load())
			}
		})
	}
}

func TestNotificationTaskScopeMonitorIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata interface{}
		selected string
		allowed  bool
	}{
		{"stable key priority", map[string]interface{}{"cron_task_key": "key:a", "cron_task_id": 2, "cron_handler": "shared"}, "key:a", true},
		{"same handler separate", map[string]interface{}{"cron_task_key": "key:b", "cron_handler": "shared"}, "key:a", false},
		{"retained numeric ID", map[string]interface{}{"cron_task_id": float64(1), "cron_handler": "shared"}, "key:a", true},
		{"retained string ID", map[string]interface{}{"cron_task_id": "1"}, "key:a", true},
		{"retained other ID", map[string]interface{}{"cron_task_id": 2, "cron_handler": "shared"}, "key:a", false},
		{"handler cannot match keyed", map[string]interface{}{"cron_handler": "shared"}, "handler:shared", false},
		{"keyless fallback", map[string]interface{}{"cron_handler": "legacy"}, "handler:legacy", true},
		{"keyless via old ID", map[string]interface{}{"cron_task_id": 3}, "handler:legacy", true},
		{"deleted ID", map[string]interface{}{"cron_task_id": 100, "cron_handler": "legacy"}, "handler:legacy", false},
		{"deleted stable key", map[string]interface{}{"cron_task_key": "deleted", "cron_task_id": 1}, "key:a", false},
		{"invalid stable key", map[string]interface{}{"cron_task_key": 1, "cron_task_id": 1}, "key:a", false},
		{"invalid ID", map[string]interface{}{"cron_task_id": 1.5, "cron_handler": "legacy"}, "handler:legacy", false},
		{"nil ID", map[string]interface{}{"cron_task_id": nil, "cron_handler": "legacy"}, "handler:legacy", false},
		{"nil metadata", nil, "key:a", false},
		{"wrong metadata", "key:a", "key:a", false},
		{"manual task", map[string]interface{}{}, "key:a", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newNotificationTestRedis(t)
			sender := &fakeNotificationSender{}
			configs := &fakeNotificationScopeConfigs{values: map[string]string{notifyTaskKeysKey: `["` + test.selected + `"]`}}
			monitor := NewNotificationEventMonitor(nil, sender, nil, nil, client, NewNotificationTaskScopeService(configs, notificationScopeCronFixture()))
			task := map[string]interface{}{"task_id": "key:a", "task_type": "key:a", "status": "completed", "metadata": test.metadata}
			if err := monitor.processTasks(context.Background(), NotificationEventChannel{Channel: "telegram", NotifyTaskCompleted: true}, []map[string]interface{}{task}); err != nil {
				t.Fatal(err)
			}
			if (sender.count() == 1) != test.allowed {
				t.Fatalf("cards=%d allowed=%v", sender.count(), test.allowed)
			}
		})
	}
}

func TestNotificationTaskScopePendingRaceAndNoReplay(t *testing.T) {
	client := newNotificationTestRedis(t)
	sender := &fakeNotificationSender{}
	configs := &fakeNotificationScopeConfigs{values: map[string]string{notifyTaskKeysKey: `["key:a"]`}}
	scope := NewNotificationTaskScopeService(configs, notificationScopeCronFixture())
	monitor := NewNotificationEventMonitor(nil, sender, nil, nil, client, scope)
	channel := NotificationEventChannel{Channel: "telegram", NotifyTaskStarted: true, NotifyTaskCompleted: true}
	task := map[string]interface{}{"task_id": "race", "status": "pending"}
	process := func() {
		t.Helper()
		if err := monitor.processTasks(context.Background(), channel, []map[string]interface{}{task}); err != nil {
			t.Fatal(err)
		}
	}
	process()
	if sender.count() != 0 || client.Exists(context.Background(), taskEventKey("race", "started")).Val() != 0 {
		t.Fatal("未绑定的 pending 不能推送或永久静默")
	}
	task["metadata"] = map[string]interface{}{"cron_task_key": "key:a"}
	process()
	if sender.count() != 1 {
		t.Fatal("补齐元数据后必须推送选中的触发事件")
	}
	task["status"] = "completed"
	process()
	if sender.count() != 2 {
		t.Fatal("选中任务终态未推送")
	}
	task["task_id"] = "unselected"
	task["metadata"] = map[string]interface{}{"cron_task_key": "key:b"}
	process()
	if _, err := scope.Update(context.Background(), []string{"key:b"}); err != nil {
		t.Fatal(err)
	}
	process()
	if sender.count() != 2 {
		t.Fatal("范围变更不能重放已静默的事件")
	}
	channel.Channel = "wecom"
	process()
	process()
	if sender.count() != 4 {
		t.Fatal("渠道去重必须独立")
	}
	channel.NotifyTaskStarted = false
	task["task_id"] = "flags"
	process()
	channel.NotifyTaskStarted = true
	process()
	if sender.count() != 5 {
		t.Fatal("范围过滤不能改变事件开关及其去重")
	}
}

func TestNotificationTaskScopeAccountsSurviveScopeError(t *testing.T) {
	for _, failure := range []string{"none", "read", "malformed", "cron"} {
		t.Run(failure, func(t *testing.T) {
			client := newNotificationTestRedis(t)
			sender := &fakeNotificationSender{}
			configs := &fakeNotificationScopeConfigs{values: map[string]string{notifyTaskKeysKey: `[]`, notifyTaskScopeConfiguredKey: "true"}}
			cron := notificationScopeCronFixture()
			accounts := &fakeNotificationAccountStore{accounts: []*domain.Cloud115{{ID: 1, Name: "本地测试", Status: domain.AccountStatusActive}}}
			tasks := &fakeNotificationTaskStore{}
			channel := NotificationEventChannel{Channel: "telegram", NotifyTaskStarted: true, NotifyTaskFailed: true, NotifyAccountStatus: true}
			monitor := NewNotificationEventMonitor(&fakeTelegramConfigReader{channels: []NotificationEventChannel{channel}}, sender, tasks, accounts, client, NewNotificationTaskScopeService(configs, cron))
			if err := monitor.runOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "read":
				configs.readErr = errors.New("scope read failed")
			case "malformed":
				configs.values[notifyTaskKeysKey] = "{"
			case "cron":
				cron.err = errors.New("cron read failed")
			}
			tasks.tasks = []map[string]interface{}{{"task_id": "failed", "status": "failed", "metadata": map[string]interface{}{"cron_task_key": "key:a"}}}
			accounts.accounts[0].Status = domain.AccountStatusCooling
			for repeat := 0; repeat < 2; repeat++ {
				if err := monitor.runOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if sender.count() != 1 || sender.cards[0].Title != "115 账号状态异常" {
				t.Fatalf("任务静默不应影响账号: %v", sender.cards)
			}
		})
	}
}

func TestNotificationTaskScopeBaselinePendingMetadataRace(t *testing.T) {
	client := newNotificationTestRedis(t)
	sender := &fakeNotificationSender{}
	configs := &fakeNotificationScopeConfigs{values: map[string]string{notifyTaskKeysKey: `["key:a"]`}}
	task := map[string]interface{}{"task_id": "initial-pending", "status": "pending"}
	tasks := &fakeNotificationTaskStore{tasks: []map[string]interface{}{task, {"task_id": "historical", "status": "completed", "metadata": map[string]interface{}{"cron_task_key": "key:a"}}}}
	channel := NotificationEventChannel{Channel: "telegram", NotifyTaskStarted: true, NotifyTaskCompleted: true}
	monitor := NewNotificationEventMonitor(&fakeTelegramConfigReader{channels: []NotificationEventChannel{channel}}, sender, tasks, &fakeNotificationAccountStore{}, client, NewNotificationTaskScopeService(configs, notificationScopeCronFixture()))
	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	task["metadata"] = map[string]interface{}{"cron_task_key": "key:a"}
	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.count() != 1 {
		t.Fatalf("初始化竞态不应吞掉选中任务触发，也不能补发历史事件: %v", sender.cards)
	}
}

func TestNotificationTaskScopeAmbiguousLegacyHandler(t *testing.T) {
	cron := notificationScopeCronFixture()
	cron.tasks = append(cron.tasks, &domain.CronTask{ID: 4, Handler: "shared"})
	scope, err := NewNotificationTaskScopeService(&fakeNotificationScopeConfigs{}, cron).snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if key := scope.taskKey(map[string]interface{}{"metadata": map[string]interface{}{"cron_handler": "shared"}}); key != "" {
		t.Fatalf("只有处理器不能区分有键任务和无键任务: %s", key)
	}
	if key := scope.taskKey(map[string]interface{}{"metadata": map[string]interface{}{"cron_task_id": 4}}); key != "handler:shared" {
		t.Fatalf("有编号时可以准确回落无键任务: %s", key)
	}
}

func TestNotificationTaskScopeReadErrorKeepsAccountBaselineAlive(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	configs := &fakeNotificationScopeConfigs{}
	sender := &fakeNotificationSender{}
	accounts := &fakeNotificationAccountStore{accounts: []*domain.Cloud115{{ID: 1, Status: domain.AccountStatusActive}}}
	channel := NotificationEventChannel{Channel: "telegram", NotifyAccountStatus: true}
	monitor := NewNotificationEventMonitor(&fakeTelegramConfigReader{channels: []NotificationEventChannel{channel}}, sender, &fakeNotificationTaskStore{}, accounts, client, NewNotificationTaskScopeService(configs, notificationScopeCronFixture()))
	if err := monitor.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	configs.readErr = errors.New("long scope outage")
	for _, status := range []string{domain.AccountStatusCooling, domain.AccountStatusActive} {
		server.FastForward(13 * time.Hour)
		accounts.accounts[0].Status = status
		if err := monitor.runOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if sender.count() != 2 {
		t.Fatalf("任务范围持续故障不能使账号基线过期并吞掉状态变化: %v", sender.cards)
	}
}

func TestNotificationTaskScopeCanceledUpdateAndGroupedKeylessTasks(t *testing.T) {
	configs := &fakeNotificationScopeConfigs{}
	cron := notificationScopeCronFixture()
	cron.tasks = append(cron.tasks, &domain.CronTask{ID: 4, TaskName: "第二个旧任务", Handler: "legacy"})
	scope := NewNotificationTaskScopeService(configs, cron)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scope.Update(ctx, []string{}); !errors.Is(err, context.Canceled) || configs.writes != 0 {
		t.Fatalf("取消必须传递到写入，且不能写配置: %v", err)
	}
	if _, err := scope.Update(context.Background(), []string{"handler:legacy", "handler:legacy"}); err != nil {
		t.Fatal(err)
	}
	view, err := scope.Get(context.Background())
	if err != nil || len(view.Tasks) != 4 || !reflect.DeepEqual(view.Selected, []string{"handler:legacy"}) {
		t.Fatalf("相同处理器的无键任务仅合并选择键，不合并列表行: %+v %v", view, err)
	}
}
