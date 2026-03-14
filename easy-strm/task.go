package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
)

// TaskStatus 任务状态
type TaskStatus struct {
	TaskID         string `json:"task_id"`
	Status         string `json:"status"` // pending, running, completed, failed
	Progress       int    `json:"progress"`
	TotalFiles     int    `json:"total_files"`
	ProcessedFiles int    `json:"processed_files"`
	SuccessFiles   int    `json:"success_files"`
	FailedFiles    int    `json:"failed_files"`
	ErrorMessage   string `json:"error_message"`
	CreateTime     string `json:"create_time"`
	UpdateTime     string `json:"update_time"`
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

// CreateTask 创建新任务
func CreateTask(taskID string) (*TaskStatus, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	task := &TaskStatus{
		TaskID:     taskID,
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

	Debug("Created task: %s", taskID)
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

	Debug("Deleted task %s from Redis", taskID)
	return nil
}
