package service

import (
	"context"
	"fmt"
	"sync"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type TaskService struct {
	taskRedisDAO *dao.TaskRedisDAO
	// cancelFuncs 存储运行中任务的取消函数，用于从外部中断长时间运行的任务
	cancelFuncs sync.Map // map[string]context.CancelFunc
}

func NewTaskService(taskRedisDAO *dao.TaskRedisDAO) *TaskService {
	return &TaskService{
		taskRedisDAO: taskRedisDAO,
	}
}

// Create 创建新任务（默认优先级5）
func (s *TaskService) Create(taskID string, taskType, taskName string) error {
	if err := s.taskRedisDAO.Create(taskID, taskType, taskName); err != nil {
		logger.Errorf("TaskService[Create] 创建任务失败: %v", err)
		return fmt.Errorf("创建任务失败: %v", err)
	}
	logger.Infof("TaskService[Create] 创建任务成功: %s, type: %s", taskID, taskType)
	return nil
}

// CreateWithPriority 创建带优先级的任务
func (s *TaskService) CreateWithPriority(taskID string, taskType, taskName string, priority int) error {
	if err := s.taskRedisDAO.CreateWithPriority(taskID, taskType, taskName, priority); err != nil {
		logger.Errorf("TaskService[CreateWithPriority] 创建任务失败: %v", err)
		return fmt.Errorf("创建任务失败: %v", err)
	}
	logger.Infof("TaskService[CreateWithPriority] 创建任务成功: %s, type: %s, priority: %d", taskID, taskType, priority)
	return nil
}

// Get 获取任务状态
func (s *TaskService) Get(taskID string) (map[string]interface{}, error) {
	task, err := s.taskRedisDAO.Get(taskID)
	if err != nil || task == nil {
		return task, err
	}
	return task, nil
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

// UpdateMetadata 更新任务元数据
func (s *TaskService) UpdateMetadata(taskID string, metadata map[string]interface{}) error {
	if err := s.taskRedisDAO.UpdateMetadata(taskID, metadata); err != nil {
		logger.Errorf("TaskService[UpdateMetadata] 更新元数据失败: %v", err)
		return fmt.Errorf("更新任务元数据失败: %v", err)
	}
	return nil
}

// Delete 删除任务
func (s *TaskService) Delete(taskID string) error {
	// 先尝试取消运行中的任务
	s.CancelContext(taskID)
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

// GetUnified 获取统一格式的任务列表
func (s *TaskService) GetUnified() ([]map[string]interface{}, error) {
	tasks, err := s.taskRedisDAO.GetUnified()
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// Cancel 取消任务（设置Redis取消标记 + 调用context cancel + 更新状态）
func (s *TaskService) Cancel(taskID string) error {
	// 先调用 context cancel 中断实际运行的 goroutine
	s.CancelContext(taskID)

	// 设置 Redis 取消标记，让轮询检查的 goroutine 也能感知
	if err := s.taskRedisDAO.SetCancelFlag(taskID); err != nil {
		logger.Warnf("TaskService[Cancel] 设置取消标记失败: %v", err)
	}

	// 再更新 Redis 中的任务状态
	if err := s.taskRedisDAO.Cancel(taskID); err != nil {
		logger.Errorf("TaskService[Cancel] 取消任务失败: %v", err)
		return fmt.Errorf("取消任务失败: %v", err)
	}

	logger.Infof("TaskService[Cancel] 取消任务成功: %s", taskID)
	return nil
}

// Resume 恢复已取消或失败的任务
func (s *TaskService) Resume(taskID string) error {
	if err := s.taskRedisDAO.Resume(taskID); err != nil {
		logger.Errorf("TaskService[Resume] 恢复任务失败: %v", err)
		return fmt.Errorf("恢复任务失败: %v", err)
	}
	logger.Infof("TaskService[Resume] 恢复任务成功: %s", taskID)
	return nil
}

// IsCancelled 检查任务是否已被取消
func (s *TaskService) IsCancelled(taskID string) bool {
	return s.taskRedisDAO.IsCancelled(taskID)
}

// AddProcessedFileID 记录已处理的文件ID
func (s *TaskService) AddProcessedFileID(taskID string, fileID string) error {
	return s.taskRedisDAO.AddProcessedFileID(taskID, fileID)
}

// IsFileProcessed 检查文件是否已被处理过
func (s *TaskService) IsFileProcessed(taskID string, fileID string) bool {
	return s.taskRedisDAO.IsFileProcessed(taskID, fileID)
}

// RegisterCancel 注册任务的 context.CancelFunc
// 在启动长时间运行的 goroutine 时调用，将 cancel 函数存储以便外部取消
func (s *TaskService) RegisterCancel(taskID string, cancel context.CancelFunc) {
	s.cancelFuncs.Store(taskID, cancel)
	logger.Debugf("TaskService[RegisterCancel] 注册取消函数: %s", taskID)
}

// CancelContext 调用已注册的 cancel 函数来中断任务 goroutine
func (s *TaskService) CancelContext(taskID string) {
	if cancelFn, ok := s.cancelFuncs.LoadAndDelete(taskID); ok {
		if fn, ok := cancelFn.(context.CancelFunc); ok {
			fn()
			logger.Infof("TaskService[CancelContext] 已调用取消函数: %s", taskID)
		}
	}
}

// RemoveCancel 任务完成后清理 cancel 函数
func (s *TaskService) RemoveCancel(taskID string) {
	s.cancelFuncs.Delete(taskID)
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
	if metadata, ok := taskMap["metadata"].(map[string]interface{}); ok {
		task.Metadata = metadata
	}
	if errMsg, ok := taskMap["error_message"].(string); ok {
		task.ErrorMessage = errMsg
	}
	if priority, ok := taskMap["priority"].(float64); ok {
		task.Priority = int(priority)
	}
	return task
}
