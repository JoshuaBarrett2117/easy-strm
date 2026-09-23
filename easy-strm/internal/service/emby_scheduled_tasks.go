package service

import (
	"easy-strm/internal/domain"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
)

// ValidateEmbyTaskTriggers 校验 Emby 支持的周期规则，保留未编辑的附加字段。
func ValidateEmbyTaskTriggers(triggers []map[string]interface{}) error {
	if triggers == nil {
		return fmt.Errorf("请提供触发规则数组，停用自动触发请传空数组")
	}
	for i, trigger := range triggers {
		number := func(key string, min, max float64) bool {
			n, ok := trigger[key].(float64)
			return ok && !math.IsNaN(n) && n >= min && n <= max && math.Trunc(n) == n
		}
		valid := false
		switch trigger["Type"] {
		case "DailyTrigger":
			valid = number("TimeOfDayTicks", 0, 864000000000-1)
		case "WeeklyTrigger":
			day, _ := trigger["DayOfWeek"].(string)
			valid = number("TimeOfDayTicks", 0, 864000000000-1) && strings.Contains("|Sunday|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|", "|"+day+"|") && day != ""
		case "IntervalTrigger":
			valid = number("IntervalTicks", 1, 9007199254740991)
		case "StartupTrigger":
			valid = true
		}
		if !valid {
			return fmt.Errorf("第 %d 条触发规则的类型、时间或周期无效", i+1)
		}
		if _, ok := trigger["MaxRuntimeTicks"]; ok && trigger["MaxRuntimeTicks"] != nil && !number("MaxRuntimeTicks", 1, 9007199254740991) {
			return fmt.Errorf("第 %d 条规则的最长运行时间无效", i+1)
		}
	}
	return nil
}

// UpdateScheduledTaskTriggers 保存完整规则，删除最后一条时向 Emby 发送空数组。
func (s *EmbyManagementService) UpdateScheduledTaskTriggers(serverID int, remoteID string, req domain.EmbyTaskTriggersRequest) error {
	if strings.TrimSpace(remoteID) == "" {
		return fmt.Errorf("定时任务 ID 不能为空")
	}
	if err := ValidateEmbyTaskTriggers(req.Triggers); err != nil {
		return err
	}
	server, err := s.requireServer(serverID)
	if err != nil {
		return err
	}
	tasks, err := s.listScheduledTasks(server)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ID == remoteID {
			return s.requestJSON(server, http.MethodPost, "/emby/ScheduledTasks/"+url.PathEscape(remoteID)+"/Triggers", nil, req.Triggers, nil)
		}
	}
	return fmt.Errorf("定时任务不存在，请刷新列表后重试")
}

// ListScheduledTasks 返回所选实例的全部计划任务及 Emby 实时执行状态。
func (s *EmbyManagementService) ListScheduledTasks(serverID int) ([]domain.EmbyScheduledTask, error) {
	server, err := s.requireServer(serverID)
	if err != nil {
		return nil, err
	}
	return s.listScheduledTasks(server)
}

func (s *EmbyManagementService) listScheduledTasks(server *domain.EmbyServer) ([]domain.EmbyScheduledTask, error) {
	result := make([]domain.EmbyScheduledTask, 0)
	err := s.requestJSON(server, http.MethodGet, "/emby/ScheduledTasks", nil, nil, &result)
	return result, err
}

// StartScheduledTask 校验远端状态后立即触发；任务中心记录提交结果，执行结果由 Emby 列表提供。
func (s *EmbyManagementService) StartScheduledTask(serverID int, remoteID string) (string, error) {
	if strings.TrimSpace(remoteID) == "" {
		return "", fmt.Errorf("定时任务 ID 不能为空")
	}
	server, err := s.requireServer(serverID)
	if err != nil {
		return "", err
	}
	tasks, err := s.listScheduledTasks(server)
	if err != nil {
		return "", err
	}
	for _, task := range tasks {
		if task.ID != remoteID {
			continue
		}
		if !strings.EqualFold(task.State, "Idle") {
			return "", fmt.Errorf("任务当前状态为 %s，不能重复触发", task.State)
		}
		metadata := taskMetadata(server, "start_scheduled_task", remoteID)
		metadata["remote_task_id"] = remoteID
		metadata["remote_task_name"] = task.Name
		metadata["result_scope"] = "仅记录触发请求，实际执行状态请查看 Emby 定时任务列表"
		return s.runShortTask(domain.TaskTypeEmbyScheduled, "触发 Emby 定时任务："+task.Name, metadata, func() error {
			return s.requestJSON(server, http.MethodPost, "/emby/ScheduledTasks/Running/"+url.PathEscape(remoteID), nil, nil, nil)
		})
	}
	return "", fmt.Errorf("定时任务不存在，请刷新列表后重试")
}
