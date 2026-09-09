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
	taskKeyPrefix         = "easy_strm:task:"
	taskListKey           = "easy_strm:task:list"
	taskCancelKeyPrefix   = "easy_strm:task:cancel:"   // 取消标记 key
	taskProgressKeyPrefix = "easy_strm:task:progress:" // 已处理文件ID集合 key
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
	client := RedisClient
	if client == nil {
		client = redisClient
	}
	// 兼容历史实现：旧代码路径仍通过包级 redisClient 访问
	redisClient = client
	return &TaskRedisDAO{client: client}
}

var redisClient *redis.Client

// InitTaskRedisDAO 初始化全局Redis客户端
func InitTaskRedisDAO(client *redis.Client) {
	redisClient = client
	RedisClient = client
}

// GetGlobalRedisClient 获取全局Redis客户端实例
func GetGlobalRedisClient() *redis.Client {
	if redisClient != nil {
		return redisClient
	}
	return RedisClient
}

// Create 创建新任务
func (t *TaskRedisDAO) Create(taskID string, taskType, taskName string) error {
	return t.CreateWithPriority(taskID, taskType, taskName, 5)
}

// CreateWithPriority 创建带优先级的任务
// priority: 0=最高优先级, 默认5
func (t *TaskRedisDAO) CreateWithPriority(taskID string, taskType, taskName string, priority int) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	task := map[string]interface{}{
		"task_id":         taskID,
		"task_type":       taskType,
		"task_name":       taskName,
		"status":          "pending",
		"priority":        priority,
		"progress":        0,
		"total_files":     0,
		"processed_files": 0,
		"success_files":   0,
		"failed_files":    0,
		"metadata":        map[string]interface{}{},
		"error_message":   "",
		"create_time":     now,
		"update_time":     now,
	}

	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("TaskRedisDAO[CreateWithPriority] 序列化失败: %v", err)
	}

	ctx := context.Background()
	key := taskKeyPrefix + taskID
	if err := redisClient.Set(ctx, key, taskJSON, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[CreateWithPriority] 保存失败: %v", err)
	}

	if err := redisClient.RPush(ctx, taskListKey, taskID).Err(); err != nil {
		return fmt.Errorf("TaskRedisDAO[CreateWithPriority] 添加到列表失败: %v", err)
	}

	logger.Infof("TaskRedisDAO[CreateWithPriority] 创建任务成功: %s, type: %s, name: %s, priority: %d", taskID, taskType, taskName, priority)
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

// UpdateProgressPercent 更新非文件型任务的真实百分比，不伪造文件统计。
func (t *TaskRedisDAO) UpdateProgressPercent(taskID string, progress int) error {
	task, err := t.Get(taskID)
	if err != nil || task == nil {
		return err
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	task["progress"] = progress
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

// UpdateMetadata 更新任务附加元数据
func (t *TaskRedisDAO) UpdateMetadata(taskID string, metadata map[string]interface{}) error {
	task, err := t.Get(taskID)
	if err != nil || task == nil {
		return err
	}

	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	task["metadata"] = metadata
	task["update_time"] = time.Now().Format("2006-01-02 15:04:05")
	return t.save(taskID, task)
}

// Cancel 取消任务（设置状态为 cancelled）
func (t *TaskRedisDAO) Cancel(taskID string) error {
	task, err := t.Get(taskID)
	if err != nil {
		return fmt.Errorf("TaskRedisDAO[Cancel] 获取任务失败: %v", err)
	}
	if task == nil {
		return fmt.Errorf("TaskRedisDAO[Cancel] 任务不存在: %s", taskID)
	}

	status, _ := task["status"].(string)
	// 处理器可能在取消函数返回后先写入终态；重复取消保持幂等。
	if status == "cancelled" { return nil }
	if status != "pending" && status != "running" {
		return fmt.Errorf("TaskRedisDAO[Cancel] 任务状态不允许取消: %s (当前: %s)", taskID, status)
	}

	task["status"] = "cancelled"
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

// GetAll 获取所有任务（按创建时间降序）
func (t *TaskRedisDAO) GetAll() ([]map[string]interface{}, error) {
	ctx := context.Background()
	taskIDs, err := redisClient.LRange(ctx, taskListKey, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("TaskRedisDAO[GetAll] 获取列表失败: %v", err)
	}

	tasks := make([]map[string]interface{}, 0)
	for _, taskID := range taskIDs {
		task, err := t.Get(taskID)
		if err != nil {
			continue
		}
		if task != nil {
			tasks = append(tasks, task)
		}
	}

	sortTasksByCreateTimeDesc(tasks)

	return tasks, nil
}

// GetUnified 获取统一格式的任务列表（按创建时间降序）
func (t *TaskRedisDAO) GetUnified() ([]map[string]interface{}, error) {
	tasks, err := t.GetAll()
	if err != nil {
		return nil, err
	}
	if tasks == nil {
		return []map[string]interface{}{}, nil
	}

	sortTasksByCreateTimeDesc(tasks)

	return tasks, nil
}

// sortTasksByCreateTimeDesc 统一保证任务中心按任务创建时间倒序展示。
func sortTasksByCreateTimeDesc(tasks []map[string]interface{}) {
	sort.SliceStable(tasks, func(i, j int) bool {
		return fmt.Sprint(tasks[i]["create_time"]) > fmt.Sprint(tasks[j]["create_time"])
	})
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

// --- 任务取消标记操作 ---

// SetCancelFlag 设置任务取消标记
func (t *TaskRedisDAO) SetCancelFlag(taskID string) error {
	ctx := context.Background()
	key := taskCancelKeyPrefix + taskID
	return redisClient.Set(ctx, key, "1", 24*time.Hour).Err()
}

// IsCancelled 检查任务是否已被取消
func (t *TaskRedisDAO) IsCancelled(taskID string) bool {
	ctx := context.Background()
	key := taskCancelKeyPrefix + taskID
	val, err := redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		logger.Warnf("TaskRedisDAO[IsCancelled] 检查取消标记失败 %s: %v", taskID, err)
		return false
	}
	return val == "1"
}

// ClearCancelFlag 清除任务取消标记
func (t *TaskRedisDAO) ClearCancelFlag(taskID string) error {
	ctx := context.Background()
	key := taskCancelKeyPrefix + taskID
	return redisClient.Del(ctx, key).Err()
}

// --- 任务进度追踪操作（用于恢复） ---

// AddProcessedFileID 记录已处理的文件ID到 Redis Set
func (t *TaskRedisDAO) AddProcessedFileID(taskID string, fileID string) error {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	return redisClient.SAdd(ctx, key, fileID).Err()
}

// IsFileProcessed 检查文件是否已被处理过
func (t *TaskRedisDAO) IsFileProcessed(taskID string, fileID string) bool {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	isMember, err := redisClient.SIsMember(ctx, key, fileID).Result()
	if err != nil {
		logger.Warnf("TaskRedisDAO[IsFileProcessed] 检查文件处理状态失败 %s/%s: %v", taskID, fileID, err)
		return false
	}
	return isMember
}

// GetProcessedFileIDs 获取已处理的文件ID集合
func (t *TaskRedisDAO) GetProcessedFileIDs(taskID string) ([]string, error) {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	return redisClient.SMembers(ctx, key).Result()
}

// ClearProgress 清除任务的进度记录
func (t *TaskRedisDAO) ClearProgress(taskID string) error {
	ctx := context.Background()
	key := taskProgressKeyPrefix + taskID
	return redisClient.Del(ctx, key).Err()
}

// Resume 恢复已取消或失败的任务
func (t *TaskRedisDAO) Resume(taskID string) error {
	task, err := t.Get(taskID)
	if err != nil {
		return fmt.Errorf("TaskRedisDAO[Resume] 获取任务失败: %v", err)
	}
	if task == nil {
		return fmt.Errorf("TaskRedisDAO[Resume] 任务不存在: %s", taskID)
	}

	status, _ := task["status"].(string)
	if status != "cancelled" && status != "failed" {
		return fmt.Errorf("TaskRedisDAO[Resume] 任务状态不允许恢复: %s (当前: %s)", taskID, status)
	}

	// 清除取消标记
	t.ClearCancelFlag(taskID)

	// 重置状态为 pending，保留进度信息
	task["status"] = "pending"
	task["error_message"] = ""
	task["update_time"] = time.Now().Format("2006-01-02 15:04:05")
	return t.save(taskID, task)
}
