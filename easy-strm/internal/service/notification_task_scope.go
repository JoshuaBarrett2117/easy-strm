package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
)

const (
	notifyTaskKeysKey            = "notify_task_keys"
	notifyTaskScopeConfiguredKey = "notify_task_scope_configured"
)

// ErrInvalidNotificationTaskSelection 表示选择为空值或包含已失效、未知的任务键。
var ErrInvalidNotificationTaskSelection = errors.New("任务选择无效，请刷新任务列表后重试")

type notificationScopeConfigStore interface {
	GetHotConfigs(context.Context, ...string) (map[string]string, error)
	BatchUpsertContext(context.Context, map[string]string) error
}

type notificationScopeCronStore interface {
	GetNotificationTasks(context.Context) ([]*domain.CronTask, error)
}

// NotificationTaskScopeService 管理任务通知选择及兼容旧配置的只读快照。
type NotificationTaskScopeService struct {
	configs notificationScopeConfigStore
	cron    notificationScopeCronStore
}

// NewNotificationTaskScopeService 注入配置与调度仓储，不读取真实外部资源。
func NewNotificationTaskScopeService(configs notificationScopeConfigStore, cron notificationScopeCronStore) *NotificationTaskScopeService {
	return &NotificationTaskScopeService{configs: configs, cron: cron}
}

type notificationTaskScopeSnapshot struct {
	all             bool
	selected        map[string]bool
	byID            map[int]string
	byKey           map[string]bool
	keylessHandlers map[string]string
}

func notificationCronTaskKey(task *domain.CronTask) string {
	if task.TaskKey != "" {
		return task.TaskKey
	}
	if task.Handler != "" {
		return "handler:" + task.Handler
	}
	return ""
}

func parseNotificationTaskSelection(values map[string]string) (bool, []string, error) {
	marker := strings.TrimSpace(values[notifyTaskScopeConfiguredKey])
	if marker != "" && marker != "true" {
		return false, nil, fmt.Errorf("任务通知范围标记损坏")
	}
	raw := strings.TrimSpace(values[notifyTaskKeysKey])
	selected := []string{}
	if raw != "" {
		var elements []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &elements); err != nil || elements == nil {
			return false, nil, fmt.Errorf("任务通知范围配置损坏")
		}
		for _, element := range elements {
			var key string
			if string(element) == "null" || json.Unmarshal(element, &key) != nil || strings.TrimSpace(key) == "" {
				return false, nil, fmt.Errorf("任务通知范围包含无效任务键")
			}
			selected = append(selected, key)
		}
	} else if marker == "true" {
		return false, nil, fmt.Errorf("已配置的任务通知范围缺失")
	}
	return marker == "" && len(selected) == 0, selected, nil
}

func (s *NotificationTaskScopeService) load(ctx context.Context) (notificationTaskScopeSnapshot, []*domain.CronTask, error) {
	values, err := s.configs.GetHotConfigs(ctx, notifyTaskKeysKey, notifyTaskScopeConfiguredKey)
	if err != nil {
		return notificationTaskScopeSnapshot{}, nil, fmt.Errorf("读取任务通知范围失败: %w", err)
	}
	all, selected, err := parseNotificationTaskSelection(values)
	if err != nil {
		return notificationTaskScopeSnapshot{}, nil, err
	}
	tasks, err := s.cron.GetNotificationTasks(ctx)
	if err != nil {
		return notificationTaskScopeSnapshot{}, nil, err
	}
	scope := notificationTaskScopeSnapshot{all: all, selected: make(map[string]bool), byID: make(map[int]string), byKey: make(map[string]bool), keylessHandlers: make(map[string]string)}
	for _, key := range selected {
		scope.selected[key] = true
	}
	for _, task := range tasks {
		key := notificationCronTaskKey(task)
		if key == "" {
			continue
		}
		scope.byID[task.ID] = key
		scope.byKey[key] = true
		if task.TaskKey == "" {
			scope.keylessHandlers[task.Handler] = key
		}
	}
	for _, task := range tasks {
		if task.TaskKey != "" {
			delete(scope.keylessHandlers, task.Handler)
		}
	}
	return scope, tasks, nil
}

func (s *NotificationTaskScopeService) snapshot(ctx context.Context) (notificationTaskScopeSnapshot, error) {
	scope, _, err := s.load(ctx)
	return scope, err
}

// Get 返回当前任务列表；旧版默认全选，已保存的空数组保持全不选。
func (s *NotificationTaskScopeService) Get(ctx context.Context) (domain.NotificationTaskScope, error) {
	scope, tasks, err := s.load(ctx)
	if err != nil {
		return domain.NotificationTaskScope{}, err
	}
	view := domain.NotificationTaskScope{Selected: []string{}, Tasks: []domain.NotificationScopeTask{}}
	seen := make(map[string]bool)
	for _, task := range tasks {
		key := notificationCronTaskKey(task)
		view.Tasks = append(view.Tasks, domain.NotificationScopeTask{TaskKey: key, TaskName: task.TaskName, Handler: task.Handler, Status: task.Status})
		if key != "" && !seen[key] && (scope.all || scope.selected[key]) {
			view.Selected = append(view.Selected, key)
			seen[key] = true
		}
	}
	return view, nil
}

// Update 校验当前调度表中的精确任务键，以一次事务保存 JSON 数组及显式配置标记。
func (s *NotificationTaskScopeService) Update(ctx context.Context, selected []string) ([]string, error) {
	if selected == nil {
		return nil, ErrInvalidNotificationTaskSelection
	}
	tasks, err := s.cron.GetNotificationTasks(ctx)
	if err != nil {
		return nil, err
	}
	valid := make(map[string]bool)
	for _, task := range tasks {
		if key := notificationCronTaskKey(task); key != "" {
			valid[key] = true
		}
	}
	unique := []string{}
	seen := make(map[string]bool)
	for _, key := range selected {
		if !valid[key] {
			return nil, ErrInvalidNotificationTaskSelection
		}
		if !seen[key] {
			unique = append(unique, key)
			seen[key] = true
		}
	}
	encoded, err := json.Marshal(unique)
	if err != nil {
		return nil, err
	}
	if err := s.configs.BatchUpsertContext(ctx, map[string]string{notifyTaskKeysKey: string(encoded), notifyTaskScopeConfiguredKey: "true"}); err != nil {
		return nil, fmt.Errorf("保存任务通知范围失败: %w", err)
	}
	return unique, nil
}

func (scope notificationTaskScopeSnapshot) taskKey(task map[string]interface{}) string {
	metadata, ok := task["metadata"].(map[string]interface{})
	if !ok || metadata == nil {
		return ""
	}
	if raw, exists := metadata["cron_task_key"]; exists && raw != nil {
		key, valid := raw.(string)
		if !valid {
			return ""
		}
		if key != "" {
			if scope.byKey[key] {
				return key
			}
			return ""
		}
	}
	if raw, exists := metadata["cron_task_id"]; exists {
		id, err := strconv.Atoi(fmt.Sprint(raw))
		if err == nil && id > 0 {
			return scope.byID[id]
		}
		return ""
	}
	handler, _ := metadata["cron_handler"].(string)
	return scope.keylessHandlers[handler]
}
