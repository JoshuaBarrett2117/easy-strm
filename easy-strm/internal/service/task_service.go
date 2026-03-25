package service

import (
	"fmt"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type TaskService struct {
	taskRedisDAO *dao.TaskRedisDAO
}

func NewTaskService(taskRedisDAO *dao.TaskRedisDAO) *TaskService {
	return &TaskService{
		taskRedisDAO: taskRedisDAO,
	}
}

// Create 创建新任务
func (s *TaskService) Create(taskID string, taskType, taskName string) error {
	if err := s.taskRedisDAO.Create(taskID, taskType, taskName); err != nil {
		logger.Errorf("TaskService[Create] 创建任务失败: %v", err)
		return fmt.Errorf("创建任务失败: %v", err)
	}
	logger.Infof("TaskService[Create] 创建任务成功: %s, type: %s", taskID, taskType)
	return nil
}

// Get 获取任务状态
func (s *TaskService) Get(taskID string) (map[string]interface{}, error) {
	return s.taskRedisDAO.Get(taskID)
}

// UpdateStatus 更新任务状态
func (s *TaskService) UpdateStatus(taskID, status string) error {
	if err := s.taskRedisDAO.UpdateStatus(taskID, status); err != nil {
		logger.Errorf("TaskService[UpdateStatus] 更新状态失败: %v", err)
		return fmt.Errorf("更新任务状态失败: %v", err)
	}
	return nil
}

// UpdateProgress 更新任务进度
func (s *TaskService) UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error {
	if err := s.taskRedisDAO.UpdateProgress(taskID, totalFiles, processedFiles, successFiles, failedFiles); err != nil {
		logger.Errorf("TaskService[UpdateProgress] 更新进度失败: %v", err)
		return fmt.Errorf("更新任务进度失败: %v", err)
	}
	return nil
}

// SetError 设置任务错误信息
func (s *TaskService) SetError(taskID, errMsg string) error {
	if err := s.taskRedisDAO.SetError(taskID, errMsg); err != nil {
		logger.Errorf("TaskService[SetError] 设置错误失败: %v", err)
		return fmt.Errorf("设置任务错误失败: %v", err)
	}
	return nil
}

// Delete 删除任务
func (s *TaskService) Delete(taskID string) error {
	if err := s.taskRedisDAO.Delete(taskID); err != nil {
		logger.Errorf("TaskService[Delete] 删除任务失败: %v", err)
		return fmt.Errorf("删除任务失败: %v", err)
	}
	return nil
}

// GetAll 获取所有任务
func (s *TaskService) GetAll() ([]map[string]interface{}, error) {
	return s.taskRedisDAO.GetAll()
}

// TaskStatusToDomain 将map转换为TaskStatus domain对象
func (s *TaskService) TaskStatusToDomain(taskMap map[string]interface{}) *domain.TaskStatus {
	task := &domain.TaskStatus{
		TaskID:         taskMap["task_id"].(string),
		TaskType:       domain.TaskType(taskMap["task_type"].(string)),
		TaskName:       taskMap["task_name"].(string),
		Status:         taskMap["status"].(string),
		Progress:       int(taskMap["progress"].(float64)),
		TotalFiles:     int(taskMap["total_files"].(float64)),
		ProcessedFiles: int(taskMap["processed_files"].(float64)),
		SuccessFiles:   int(taskMap["success_files"].(float64)),
		FailedFiles:    int(taskMap["failed_files"].(float64)),
		CreateTime:     taskMap["create_time"].(string),
		UpdateTime:     taskMap["update_time"].(string),
	}
	if errMsg, ok := taskMap["error_message"].(string); ok {
		task.ErrorMessage = errMsg
	}
	return task
}
