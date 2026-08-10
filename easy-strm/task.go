package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
)

// TaskType 任务类型
type TaskType string

const (
	TaskTypeStrmGenerate          TaskType = "strm_generate"           // STRM文件生成（全量）
	TaskTypeIncrementalSync       TaskType = "incremental_sync"        // STRM文件增量同步
	TaskTypeShareTransfer         TaskType = "share_transfer"          // 分享转存
	TaskTypeShareParse            TaskType = "share_parse"             // 分享解析
	TaskTypeShareTransferOrganize TaskType = "share_transfer_organize" // 转存后自动整理
	TaskTypeShareTransferScrape   TaskType = "share_transfer_scrape"   // 转存后自动刮削
)

// TaskTypeNames 任务类型中文名称
var TaskTypeNames = map[TaskType]string{
	TaskTypeStrmGenerate:          "STRM文件生成",
	TaskTypeIncrementalSync:       "增量同步",
	TaskTypeShareTransfer:         "分享转存",
	TaskTypeShareParse:            "分享解析",
	TaskTypeShareTransferOrganize: "转存后整理",
	TaskTypeShareTransferScrape:   "转存后刮削",
}

// TaskStatus 任务状态
type TaskStatus struct {
	TaskID         string   `json:"task_id"`
	TaskType       TaskType `json:"task_type"`       // 任务类型
	TaskName       string   `json:"task_name"`       // 任务名称
	Status         string   `json:"status"`          // pending, running, completed, failed, cancelled
	Priority       int      `json:"priority"`        // 优先级：0=最高，默认5
	Progress       int      `json:"progress"`        // 进度百分比
	TotalFiles     int      `json:"total_files"`     // 待生成文件总数
	ProcessedFiles int      `json:"processed_files"` // 已处理文件数
	SuccessFiles   int      `json:"success_files"`   // 成功文件数
	FailedFiles    int      `json:"failed_files"`    // 失败文件数
	ErrorMessage   string   `json:"error_message"`   // 错误信息
	CreateTime     string   `json:"create_time"`     // 创建时间
	UpdateTime     string   `json:"update_time"`     // 更新时间
}

// 任务状态常量
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// 任务Redis key前缀
const taskKeyPrefix = "easy_strm:task:"
const taskListKey = "easy_strm:task:list"
const taskCancelKeyPrefix = "easy_strm:task:cancel:"     // 取消标记 key
const taskProgressKeyPrefix = "easy_strm:task:progress:" // 已处理文件ID集合 key

// CreateTask 创建新任务（默认优先级5）
func CreateTask(taskID string, taskType TaskType, taskName string) (*TaskStatus, error) {
	return CreateTaskWithPriority(taskID, taskType, taskName, 5)
}

// CreateTaskWithPriority 创建带优先级的任务
// priority: 0=最高优先级，数值越小越优先
func CreateTaskWithPriority(taskID string, taskType TaskType, taskName string, priority int) (*TaskStatus, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	task := &TaskStatus{
		TaskID:     taskID,
		TaskType:   taskType,
		TaskName:   taskName,
		Status:     TaskStatusPending,
		Priority:   priority,
		Progress:   0,
		CreateTime: now,
		UpdateTime: now,
	}

	err := SaveTask(task)
	if err != nil {
		Error("Failed to create task %s: %v", taskID, err)
		return nil, err
	}

	AddTaskToList(taskID)
	Debug("Created task: %s, type: %s, name: %s, priority: %d", taskID, taskType, taskName, priority)
	return task, nil
}

// SaveTask 保存任务状态到Redis
func SaveTask(task *TaskStatus) error {
	ctx := context.Background()
	key := taskKeyPrefix + task.TaskID

	taskJSON, err := json.Marshal(task)
	if err != nil {
		Error("Failed to marshal task %s: %v", task.TaskID, err)
		return err
	}

	err = redisClient.Set(ctx, key, taskJSON, 24*time.Hour).Err()
	if err != nil {
		Error("Failed to save task %s to Redis: %v", task.TaskID, err)
		return err
	}

	Debug("Saved task %s to Redis", task.TaskID)
	return nil
}

// GetTask 从Redis获取任务状态
func GetTask(taskID string) (*TaskStatus, error) {
	ctx := context.Background()
	key := taskKeyPrefix + taskID

	taskJSON, err := redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		Error("Task %s not found in Redis", taskID)
		return nil, nil
	}
	if err != nil {
		Error("Failed to get task %s from Redis: %v", taskID, err)
		return nil, err
	}

	var task TaskStatus
	err = json.Unmarshal([]byte(taskJSON), &task)
	if err != nil {
		Error("Failed to unmarshal task %s: %v", taskID, err)
		return nil, err
	}

	return &task, nil
}

// UpdateTaskStatus 更新任务状态
func UpdateTaskStatus(taskID string, status string) error {
	task, err := GetTask(taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}

	task.Status = status
	task.UpdateTime = time.Now().Format("2006-01-02 15:04:05")
	return SaveTask(task)
}

// UpdateTaskProgress 更新任务进度
func UpdateTaskProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error {
	task, err := GetTask(taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}

	task.TotalFiles = totalFiles
	task.ProcessedFiles = processedFiles
	task.SuccessFiles = successFiles
	task.FailedFiles = failedFiles

	if totalFiles > 0 {
		task.Progress = int(float64(processedFiles) / float64(totalFiles) * 100)
	}

	task.UpdateTime = time.Now().Format("2006-01-02 15:04:05")
	return SaveTask(task)
}

// SetTaskError 设置任务错误
func SetTaskError(taskID string, errMsg string) error {
	task, err := GetTask(taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}

	task.Status = TaskStatusFailed
	task.ErrorMessage = errMsg
	task.UpdateTime = time.Now().Format("2006-01-02 15:04:05")
	return SaveTask(task)
}

// DeleteTask 删除任务
func DeleteTask(taskID string) error {
	ctx := context.Background()
	key := taskKeyPrefix + taskID

	err := redisClient.Del(ctx, key).Err()
	if err != nil {
		Error("Failed to delete task %s from Redis: %v", taskID, err)
		return err
	}

	RemoveTaskFromList(taskID)
	Debug("Deleted task %s from Redis", taskID)
	return nil
}

// GetAllTasks 获取所有任务
func GetAllTasks() ([]*TaskStatus, error) {
	ctx := context.Background()
	taskIDs, err := redisClient.LRange(ctx, taskListKey, 0, -1).Result()
	if err != nil && err != redis.Nil {
		Error("Failed to get task list from Redis: %v", err)
		return nil, err
	}

	tasks := make([]*TaskStatus, 0)
	for _, taskID := range taskIDs {
		task, err := GetTask(taskID)
		if err != nil {
			Warn("Failed to get task %s: %v", taskID, err)
			continue
		}
		if task != nil {
			tasks = append(tasks, task)
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreateTime > tasks[j].CreateTime
	})

	return tasks, nil
}

// AddTaskToList 将任务ID添加到任务列表
func AddTaskToList(taskID string) error {
	ctx := context.Background()
	err := redisClient.RPush(ctx, taskListKey, taskID).Err()
	if err != nil {
		Error("Failed to add task %s to list: %v", taskID, err)
		return err
	}
	Debug("Added task %s to list", taskID)
	return nil
}

// RemoveTaskFromList 从任务列表中移除任务ID
func RemoveTaskFromList(taskID string) error {
	ctx := context.Background()
	err := redisClient.LRem(ctx, taskListKey, 0, taskID).Err()
	if err != nil {
		Error("Failed to remove task %s from list: %v", taskID, err)
		return err
	}
	Debug("Removed task %s from list", taskID)
	return nil
}

// --- 任务取消机制 ---

// SetTaskCancelFlag 设置任务取消标记
// goroutine 在每次迭代时检查此标记，实现协作式取消
func SetTaskCancelFlag(taskID string) error {
	ctx := context.Background()
	key := taskCancelKeyPrefix + taskID
	// 设置取消标记，24小时过期（与任务本身同步）
	return redisClient.Set(ctx, key, "1", 24*time.Hour).Err()
}

// IsTaskCancelled 检查任务是否已被取消
// 在 goroutine 的每次迭代中调用此函数检查取消标记
func IsTaskCancelled(taskID string) bool {
	ctx := context.Background()
	key := taskCancelKeyPrefix + taskID
	val, err := redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		Warn("检查任务取消标记失败 %s: %v", taskID, err)
		return false
	}
	return val == "1"
}

// ClearTaskCancelFlag 清除任务取消标记
func ClearTaskCancelFlag(taskID string) error {
	ctx := context.Background()
	key := taskCancelKeyPrefix + taskID
	return redisClient.Del(ctx, key).Err()
}

// CancelTask 取消任务（设置取消标记 + 更新状态）
func CancelTask(taskID string) error {
	task, err := GetTask(taskID)
	if err != nil {
		return fmt.Errorf("获取任务失败: %v", err)
	}
	if task == nil {
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	// 仅 pending/running 状态可取消
	if task.Status != TaskStatusPending && task.Status != TaskStatusRunning {
		return fmt.Errorf("任务状态不允许取消: %s (当前: %s)", taskID, task.Status)
	}

	// 设置取消标记，让运行中的 goroutine 感知
	if err := SetTaskCancelFlag(taskID); err != nil {
		Error("设置取消标记失败 %s: %v", taskID, err)
	}

	// 更新任务状态为 cancelled
	task.Status = TaskStatusCancelled
	task.UpdateTime = time.Now().Format("2006-01-02 15:04:05")
	return SaveTask(task)
}

// --- 任务进度追踪（用于恢复） ---

// AddProcessedFileID 记录已处理的文件ID到 Redis Set
// 恢复任务时通过此集合跳过已处理文件
func AddProcessedFileID(taskID string, fileID string) error {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	return redisClient.SAdd(ctx, key, fileID).Err()
}

// IsFileProcessed 检查文件是否已被处理过（用于恢复时跳过）
func IsFileProcessed(taskID string, fileID string) bool {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	isMember, err := redisClient.SIsMember(ctx, key, fileID).Result()
	if err != nil {
		Warn("检查文件处理状态失败 %s/%s: %v", taskID, fileID, err)
		return false
	}
	return isMember
}

// GetProcessedFileIDs 获取已处理的文件ID集合
func GetProcessedFileIDs(taskID string) ([]string, error) {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	members, err := redisClient.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return members, nil
}

// ClearTaskProgress 清除任务的进度记录
func ClearTaskProgress(taskID string) error {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	return redisClient.Del(ctx, key).Err()
}

// --- 任务恢复 ---

// ResumeTask 恢复已取消或失败的任务
// 将任务状态重置为 pending，保留进度信息，等待调度器重新执行
func ResumeTask(taskID string) (*TaskStatus, error) {
	task, err := GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %v", err)
	}
	if task == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}

	// 仅 cancelled/failed 状态可恢复
	if task.Status != TaskStatusCancelled && task.Status != TaskStatusFailed {
		return nil, fmt.Errorf("任务状态不允许恢复: %s (当前: %s)", taskID, task.Status)
	}

	// 清除取消标记
	ClearTaskCancelFlag(taskID)

	// 重置状态为 pending，保留进度信息（processed_files 等字段保留）
	task.Status = TaskStatusPending
	task.ErrorMessage = ""
	task.UpdateTime = time.Now().Format("2006-01-02 15:04:05")

	if err := SaveTask(task); err != nil {
		return nil, fmt.Errorf("保存任务失败: %v", err)
	}

	Info("任务已恢复: %s, 保留进度: %d/%d", taskID, task.ProcessedFiles, task.TotalFiles)
	return task, nil
}
