package service

import (
	"easy-strm/internal/domain"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

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
