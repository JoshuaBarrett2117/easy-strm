package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"easy-strm/internal/pkg/logger"

	"github.com/go-redis/redis/v8"
)

const (
	taskKeyPrefix = "easy_strm:task:"
	taskListKey   = "easy_strm:task:list"
)

// TaskRedisDAO 任务状态Redis数据访问层
type TaskRedisDAO struct {
	client *redis.Client
}

// NewTaskRedisDAO 创建任务Redis DAO实例
func NewTaskRedisDAO(client *redis.Client) *TaskRedisDAO {
	return &TaskRedisDAO{client: client}
}

// NewTaskRedisDAOWithGlobal 使用全局Redis客户端创建实例
func NewTaskRedisDAOWithGlobal() *TaskRedisDAO {
	return &TaskRedisDAO{client: redisClient}
}

var redisClient *redis.Client

// InitTaskRedisDAO 初始化全局Redis客户端
func InitTaskRedisDAO(client *redis.Client) {
	redisClient = client
}

// GetGlobalRedisClient 获取全局Redis客户端实例
func GetGlobalRedisClient() *redis.Client {
	return redisClient
}

// Create 创建新任务
func (t *TaskRedisDAO) Create(taskID string, taskType, taskName string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	task := map[string]interface{}{
		"task_id":          taskID,
		"task_type":        taskType,
		"task_name":        taskName,
		"status":           "pending",
		"progress":         0,
		"total_files":      0,
		"processed_files":  0,
		"success_files":    0,
		"failed_files":     0,
		"error_message":    "",
		"create_time":      now,
		"update_time":      now,
	}

	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("TaskRedisDAO[Create] 序列化失败: %v", err)
	}

	ctx := context.Background()
	key := taskKeyPrefix + taskID
	if err := redisClient.Set(ctx, key, taskJSON, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[Create] 保存失败: %v", err)
	}

	if err := redisClient.RPush(ctx, taskListKey, taskID).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[Create] 添加到列表失败: %v", err)
	}

	logger.Infof("TaskRedisDAO[Create] 创建任务成功: %s, type: %s, name: %s", taskID, taskType, taskName)
	return nil
}

// Get 获取任务状态
func (t *TaskRedisDAO) Get(taskID string) (map[string]interface{}, error) {
	ctx := context.Background()
	key := taskKeyPrefix + taskID

	taskJSON, err := redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("TaskRedisDAO[Get] 获取失败: %v", err)
	}

	var task map[string]interface{}
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return nil, fmt.Errorf("TaskRedisDAO[Get] 反序列化失败: %v", err)
	}

	return task, nil
}

// UpdateStatus 更新任务状态
func (t *TaskRedisDAO) UpdateStatus(taskID, status string) error {
	task, err := t.Get(taskID)
	if err != nil || task == nil {
		return err
	}

	task["status"] = status
	task["update_time"] = time.Now().Format("2006-01-02 15:04:05")
	return t.save(taskID, task)
}

// UpdateProgress 更新任务进度
func (t *TaskRedisDAO) UpdateProgress(taskID string, totalFiles, processedFiles, successFiles, failedFiles int) error {
	task, err := t.Get(taskID)
	if err != nil || task == nil {
		return err
	}

	task["total_files"] = totalFiles
	task["processed_files"] = processedFiles
	task["success_files"] = successFiles
	task["failed_files"] = failedFiles

	if totalFiles > 0 {
		task["progress"] = int(float64(processedFiles) / float64(totalFiles) * 100)
	}

	task["update_time"] = time.Now().Format("2006-01-02 15:04:05")
	return t.save(taskID, task)
}

// SetError 设置任务错误信息
func (t *TaskRedisDAO) SetError(taskID, errMsg string) error {
	task, err := t.Get(taskID)
	if err != nil || task == nil {
		return err
	}

	task["status"] = "failed"
	task["error_message"] = errMsg
	task["update_time"] = time.Now().Format("2006-01-02 15:04:05")
	return t.save(taskID, task)
}

// Delete 删除任务
func (t *TaskRedisDAO) Delete(taskID string) error {
	ctx := context.Background()
	key := taskKeyPrefix + taskID

	if err := redisClient.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[Delete] 删除失败: %v", err)
	}

	if err := redisClient.LRem(ctx, taskListKey, 0, taskID).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[Delete] 从列表移除失败: %v", err)
	}

	logger.Infof("TaskRedisDAO[Delete] 删除任务成功: %s", taskID)
	return nil
}

// GetAll 获取所有任务
func (t *TaskRedisDAO) GetAll() ([]map[string]interface{}, error) {
	ctx := context.Background()
	taskIDs, err := redisClient.LRange(ctx, taskListKey, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("TaskRedisDAO[GetAll] 获取列表失败: %v", err)
	}

	var tasks []map[string]interface{}
	for _, taskID := range taskIDs {
		task, err := t.Get(taskID)
		if err != nil {
			continue
		}
		if task != nil {
			tasks = append(tasks, task)
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		t1 := tasks[i]["create_time"].(string)
		t2 := tasks[j]["create_time"].(string)
		return t1 > t2
	})

	return tasks, nil
}

// save 保存任务到Redis
func (t *TaskRedisDAO) save(taskID string, task map[string]interface{}) error {
	ctx := context.Background()
	key := taskKeyPrefix + taskID

	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("TaskRedisDAO[save] 序列化失败: %v", err)
	}

	if err := redisClient.Set(ctx, key, taskJSON, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[save] 保存失败: %v", err)
	}

	return nil
}
