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
	cancelGuards sync.Map // map[string]func() error，持久化操作只在等待阶段允许取消
	cancelFuncs  sync.Map // map[string]context.CancelFunc
}

func NewTaskService(taskRedisDAO *dao.TaskRedisDAO) *TaskService {
	return &TaskService{
		taskRedisDAO: taskRedisDAO,
	}
}

// Create 创建新任务（默认优先级5）
func (s *TaskService) Create(taskID string, taskType, taskName string) error {
	return s.CreateContext(nil, taskID, taskType, taskName)
}

// CreateContext 创建任务并将入口标识传至 DAO 与任务日志，兼容后台无请求调用。
func (s *TaskService) CreateContext(ctx context.Context, taskID string, taskType, taskName string) error {
	if err := s.taskRedisDAO.WithContext(ctx).Create(taskID, taskType, taskName); err != nil {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.ERROR, "创建任务失败", nil, err)
		return fmt.Errorf("创建任务失败: %v", err)
	}
	logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.INFO, "创建任务成功", logger.Fields{"type": taskType}, nil)
	return nil
}

// CreateWithPriority 创建带优先级的任务
func (s *TaskService) CreateWithPriority(taskID string, taskType, taskName string, priority int) error {
	if err := s.taskRedisDAO.CreateWithPriority(taskID, taskType, taskName, priority); err != nil {
		logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.ERROR, "创建任务失败", nil, err)
		return fmt.Errorf("创建任务失败: %v", err)
	}
	logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.INFO, "创建任务成功", logger.Fields{"type": taskType, "priority": priority}, nil)
	return nil
}

// Get 获取任务状态
func (s *TaskService) Get(taskID string) (map[string]interface{}, error) {
	return s.GetContext(nil, taskID)
}

// GetContext 在入口上下文中读取任务，并返回已脱敏的展示状态。
func (s *TaskService) GetContext(ctx context.Context, taskID string) (map[string]interface{}, error) {
	task, err := s.taskRedisDAO.WithContext(ctx).Get(taskID)
	if err != nil || task == nil {
		return task, err
	}
	task["display_status"] = taskDisplayStatus(task)
	return buildPublicTask(task), nil
}

// UpdateStatus 更新任务状态
func (s *TaskService) UpdateStatus(taskID, status string) error {
	return s.UpdateStatusContext(nil, taskID, status)
}

// UpdateStatusContext 更新任务状态并保留入口诊断标识。
func (s *TaskService) UpdateStatusContext(ctx context.Context, taskID, status string) error {
	if err := s.taskRedisDAO.WithContext(ctx).UpdateStatus(taskID, status); err != nil {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.ERROR, "更新任务状态失败", nil, err)
		return fmt.Errorf("更新任务状态失败: %v", err)
	}
	return nil
}

// UpdateProgress 更新任务进度
func (s *TaskService) UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error {
	return s.UpdateProgressContext(nil, taskID, totalFiles, processedFiles, successFiles, failedFiles)
}

// UpdateProgressContext 持久化进度，失败日志保留请求与任务上下文。
func (s *TaskService) UpdateProgressContext(ctx context.Context, taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error {
	if err := s.taskRedisDAO.WithContext(ctx).UpdateProgress(taskID, totalFiles, processedFiles, successFiles, failedFiles); err != nil {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.ERROR, "更新任务进度失败", nil, err)
		return fmt.Errorf("更新任务进度失败: %v", err)
	}
	return nil
}

// UpdateProgressPercent 更新非文件型任务的可信进度百分比。
func (s *TaskService) UpdateProgressPercent(taskID string, progress int) error {
	return s.UpdateProgressPercentContext(nil, taskID, progress)
}

// UpdateProgressPercentContext 写入非文件型进度并保留入口上下文。
func (s *TaskService) UpdateProgressPercentContext(ctx context.Context, taskID string, progress int) error {
	if err := s.taskRedisDAO.WithContext(ctx).UpdateProgressPercent(taskID, progress); err != nil {
		return fmt.Errorf("更新任务进度失败: %v", err)
	}
	return nil
}

// SetError 设置任务错误信息
func (s *TaskService) SetError(taskID, errMsg string) error {
	return s.SetErrorContext(nil, taskID, errMsg)
}

// SetErrorContext 保留任务错误写入的入口上下文，不打印原任务错误正文。
func (s *TaskService) SetErrorContext(ctx context.Context, taskID, errMsg string) error {
	if err := s.taskRedisDAO.WithContext(ctx).SetError(taskID, errMsg); err != nil {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.ERROR, "设置任务错误失败", nil, err)
		return fmt.Errorf("设置任务错误失败: %v", err)
	}
	return nil
}

// UpdateMetadata 更新任务元数据
func (s *TaskService) UpdateMetadata(taskID string, metadata map[string]interface{}) error {
	return s.UpdateMetadataContext(nil, taskID, metadata)
}

// UpdateMetadataContext 写入任务元数据，日志只携带上下文与错误而不输出负载。
func (s *TaskService) UpdateMetadataContext(ctx context.Context, taskID string, metadata map[string]interface{}) error {
	if err := s.taskRedisDAO.WithContext(ctx).UpdateMetadata(taskID, metadata); err != nil {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.ERROR, "更新任务元数据失败", nil, err)
		return fmt.Errorf("更新任务元数据失败: %v", err)
	}
	return nil
}

// Delete 删除任务
func (s *TaskService) Delete(taskID string) error {
	if _, guarded := s.cancelGuards.Load(taskID); guarded {
		if err := s.Cancel(taskID); err != nil {
			return err
		}
	}
	// 先尝试取消运行中的任务
	s.CancelContext(taskID)
	if err := s.taskRedisDAO.Delete(taskID); err != nil {
		logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.ERROR, "删除任务失败", nil, err)
		return fmt.Errorf("删除任务失败: %v", err)
	}
	return nil
}

// GetAll 获取所有任务
func (s *TaskService) GetAll() ([]map[string]interface{}, error) {
	tasks, err := s.taskRedisDAO.GetAll()
	if err != nil {
		return nil, err
	}
	for index, task := range tasks {
		tasks[index] = buildPublicTask(task)
	}
	return tasks, nil
}

// GetUnified 获取统一格式的任务列表
func (s *TaskService) GetUnified() ([]map[string]interface{}, error) {
	tasks, err := s.taskRedisDAO.GetUnified()
	if err != nil {
		return nil, err
	}
	for index, task := range tasks {
		task["display_status"] = taskDisplayStatus(task)
		tasks[index] = buildPublicTask(task)
	}
	return tasks, nil
}

// RecoverInterruptedTasks 将服务重启后没有执行协程承接的任务标记为失败，避免任务永久停留在待执行/执行中。
func (s *TaskService) RecoverInterruptedTasks() {
	tasks, err := s.taskRedisDAO.GetUnified()
	if err != nil {
		logger.Warnf("TaskService[RecoverInterruptedTasks] 读取任务失败: %v", err)
		return
	}
	for _, task := range tasks {
		kind, _ := task["task_type"].(string)
		if domain.IsShareOperationType(kind) {
			continue
		}
		status, _ := task["status"].(string)
		if status != "pending" && status != "running" {
			continue
		}
		taskID, _ := task["task_id"].(string)
		if taskID == "" {
			continue
		}
		if err := s.SetError(taskID, "服务重启导致任务中断，请重新执行"); err != nil {
			logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.WARN, "恢复中断任务状态失败", nil, err)
		}
	}
}

// Cancel 取消任务（设置Redis取消标记 + 调用context cancel + 更新状态）
func (s *TaskService) Cancel(taskID string) error {
	if guard, ok := s.cancelGuards.Load(taskID); ok {
		if err := guard.(func() error)(); err != nil {
			return err
		}
	} else if task, err := s.Get(taskID); err != nil {
		return err
	} else if task != nil {
		kind, _ := task["task_type"].(string)
		if domain.IsShareOperationType(kind) {
			return fmt.Errorf("分享清理操作已结束或正在恢复，不能取消")
		}
	}
	// 先调用 context cancel 中断实际运行的 goroutine
	s.CancelContext(taskID)

	// 设置 Redis 取消标记，让轮询检查的 goroutine 也能感知
	if err := s.taskRedisDAO.SetCancelFlag(taskID); err != nil {
		logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.WARN, "设置任务取消标记失败", nil, err)
	}

	// 再更新 Redis 中的任务状态
	if err := s.taskRedisDAO.Cancel(taskID); err != nil {
		logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.ERROR, "取消任务失败", nil, err)
		return fmt.Errorf("取消任务失败: %v", err)
	}

	logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.INFO, "取消任务成功", nil, nil)
	return nil
}

// Resume 恢复已取消或失败的任务
func (s *TaskService) Resume(taskID string) error {
	return s.ResumeContext(nil, taskID)
}

// ResumeContext 恢复任务状态并保留重试入口的请求与动作关联。
func (s *TaskService) ResumeContext(ctx context.Context, taskID string) error {
	task, err := s.GetContext(ctx, taskID)
	if err != nil {
		return err
	}
	if task != nil {
		kind, _ := task["task_type"].(string)
		metadata, _ := task["metadata"].(map[string]interface{})
		handler, _ := metadata["cron_handler"].(string)
		if domain.IsShareStrmCronHandler(handler) {
			return fmt.Errorf("分享库调度任务请等待下次调度，或在定时任务管理中手动触发")
		}
		if domain.IsShareOperationType(kind) {
			return fmt.Errorf("清理操作需重新确认并提交，请返回分享管理重试")
		}
	}
	if err := s.taskRedisDAO.WithContext(ctx).Resume(taskID); err != nil {
		logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.ERROR, "恢复任务失败", nil, err)
		return fmt.Errorf("恢复任务失败: %v", err)
	}
	logger.WithContext(logger.WithTaskID(ctx, taskID), "task").Log(logger.INFO, "恢复任务成功", nil, nil)
	return nil
}

// IsCancelled 检查任务是否已被取消
func (s *TaskService) IsCancelled(taskID string) bool {
	return s.IsCancelledContext(nil, taskID)
}

// IsCancelledContext 检查取消标记，异常日志关联到原任务入口。
func (s *TaskService) IsCancelledContext(ctx context.Context, taskID string) bool {
	return s.taskRedisDAO.WithContext(ctx).IsCancelled(taskID)
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
	logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.DEBUG, "注册任务取消函数", nil, nil)
}

// CancelContext 调用已注册的 cancel 函数来中断任务 goroutine
func (s *TaskService) CancelContext(taskID string) {
	if cancelFn, ok := s.cancelFuncs.LoadAndDelete(taskID); ok {
		if fn, ok := cancelFn.(context.CancelFunc); ok {
			fn()
			logger.WithContext(logger.WithTaskID(nil, taskID), "task").Log(logger.INFO, "已调用任务取消函数", nil, nil)
		}
	}
}

// RemoveCancel 任务完成后清理 cancel 函数
func (s *TaskService) RemoveCancel(taskID string) {
	s.cancelFuncs.Delete(taskID)
}

// TaskStatusToDomain 将map转换为TaskStatus domain对象
func (s *TaskService) TaskStatusToDomain(taskMap map[string]interface{}) *domain.TaskStatus {
	taskMap = buildPublicTask(taskMap)
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

// 原任务响应直接透传内部元数据；复制后过滤新保存的分享密码，避免泄露且不破坏重试所需原值。
func buildPublicTask(task map[string]interface{}) map[string]interface{} {
	publicTask := make(map[string]interface{}, len(task))
	for key, value := range task {
		publicTask[key] = value
	}
	if metadata, ok := task["metadata"].(map[string]interface{}); ok {
		publicMetadata := make(map[string]interface{}, len(metadata))
		for key, value := range metadata {
			if key != shareTransferPasswordMetadataKey {
				publicMetadata[key] = value
			}
		}
		publicTask["metadata"] = publicMetadata
	}
	return publicTask
}

// RegisterCancelGuard 注册持久化操作的原子取消校验，须早于接口返回。
func (s *TaskService) RegisterCancelGuard(id string, guard func() error) {
	s.cancelGuards.Store(id, guard)
}

// RemoveCancelGuard 清理操作结束后移除取消校验。
func (s *TaskService) RemoveCancelGuard(id string) { s.cancelGuards.Delete(id) }
