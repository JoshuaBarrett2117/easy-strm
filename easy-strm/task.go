package main

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
)

// TaskType 任务类型
type TaskType string

const (
	TaskTypeStrmGenerate    TaskType = "strm_generate"    // STRM文件生成（全量）
	TaskTypeIncrementalSync TaskType = "incremental_sync" // STRM文件增量同步
)

// TaskTypeNames 任务类型中文名称
var TaskTypeNames = map[TaskType]string{
	TaskTypeStrmGenerate:    "STRM文件生成",
	TaskTypeIncrementalSync: "增量同步",
}

// TaskStatus 任务状态
type TaskStatus struct {
	TaskID         string   `json:"task_id"`
	TaskType       TaskType `json:"task_type"`       // 任务类型
	TaskName       string   `json:"task_name"`       // 任务名称
	Status         string   `json:"status"`          // pending, running, completed, failed
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
)

// 任务Redis key前缀
const taskKeyPrefix = "easy_strm:task:"
const taskListKey = "easy_strm:task:list"

// CreateTask 创建新任务
func CreateTask(taskID string, taskType TaskType, taskName string) (*TaskStatus, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	task := &TaskStatus{
		TaskID:     taskID,
		TaskType:   taskType,
		TaskName:   taskName,
		Status:     TaskStatusPending,
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
	Debug("Created task: %s, type: %s, name: %s", taskID, taskType, taskName)
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
