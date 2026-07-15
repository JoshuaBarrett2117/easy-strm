package service

import (
	"context"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"strings"
	"time"
)

type OrganizePreviewTask struct {
	TaskID     string        `json:"task_id"`
	SourceID   int           `json:"source_id"`
	SourcePath string        `json:"source_path"`
	TargetPath string        `json:"target_path"`
	MediaType  string        `json:"media_type"`
	FileIDs    []string      `json:"file_ids"`
	Status     string        `json:"status"` // pending, processing, completed, failed
	Progress   *TaskProgress `json:"progress,omitempty"`
	Result     *TaskResult   `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type OrganizeCandidateTask struct {
	TaskID     string              `json:"task_id"`
	SourceID   int                 `json:"source_id"`
	SourcePath string              `json:"source_path"`
	MediaType  string              `json:"media_type"`
	FileIDs    []string            `json:"file_ids"`
	Status     string              `json:"status"` // pending, processing, completed, failed
	Progress   *TaskProgress       `json:"progress,omitempty"`
	Result     []OrganizeCandidate `json:"result,omitempty"`
	Total      int                 `json:"total"`
	Error      string              `json:"error,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type TaskProgress struct {
	Total       int    `json:"total"`
	Processed   int    `json:"processed"`
	CurrentFile string `json:"current_file,omitempty"`
}

type TaskResult struct {
	Previews []OrganizePreview `json:"previews"`
	Summary  *TaskSummary      `json:"summary"`
}

type TaskSummary struct {
	Total       int `json:"total"`
	Conflicts   int `json:"conflicts"`
	Failed      int `json:"failed"`
	Processable int `json:"processable"`
}

type OrganizeExecutionProgress struct {
	Total     int
	Processed int
	Success   int
	Failed    int
	Skipped   int
	Result    *OrganizeResult
}

func GenerateTaskKey(sourceID int, sourcePath string, fileIDs []string) string {
	key := fmt.Sprintf("organize:preview:%d:%s", sourceID, sourcePath)
	if len(fileIDs) > 0 {
		key += ":" + strings.Join(fileIDs, ",")
	}
	return key
}

func (s *OrganizeService) persistPreviewTask(ctx context.Context, taskKey string, task *OrganizePreviewTask) error {
	if s.redisClient == nil {
		return fmt.Errorf("redis client is not initialized")
	}
	if task == nil {
		return fmt.Errorf("preview task is nil")
	}

	task.UpdatedAt = time.Now()
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal preview task failed: %w", err)
	}

	if err := s.redisClient.Set(ctx, taskKey, string(taskJSON), 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("persist preview task failed: %w", err)
	}
	return nil
}

func (s *OrganizeService) persistCandidateTask(ctx context.Context, taskKey string, task *OrganizeCandidateTask) error {
	if s.redisClient == nil {
		return fmt.Errorf("redis client is not initialized")
	}
	if task == nil {
		return fmt.Errorf("candidate task is nil")
	}

	task.UpdatedAt = time.Now()
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal candidate task failed: %w", err)
	}

	if err := s.redisClient.Set(ctx, taskKey, string(taskJSON), 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("persist candidate task failed: %w", err)
	}
	return nil
}

func (s *OrganizeService) StartCandidateTask(sourceID int, sourcePath, mediaType string, fileIDs []string) (string, error) {
	if s.redisClient == nil {
		return "", fmt.Errorf("候选扫描任务存储未初始化")
	}

	taskID := fmt.Sprintf("candidates_%d_%d", sourceID, time.Now().UnixNano())
	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	task := OrganizeCandidateTask{
		TaskID:     taskID,
		SourceID:   sourceID,
		SourcePath: sourcePath,
		MediaType:  mediaType,
		FileIDs:    fileIDs,
		Status:     "pending",
		Progress:   &TaskProgress{Total: 3, Processed: 0, CurrentFile: "等待开始"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	ctx := context.Background()
	if err := s.persistCandidateTask(ctx, taskKey, &task); err != nil {
		return "", fmt.Errorf("保存候选扫描任务失败: %v", err)
	}

	go s.runCandidateTask(taskID, sourceID, sourcePath, mediaType, fileIDs)
	return taskID, nil
}

func (s *OrganizeService) runCandidateTask(taskID string, sourceID int, sourcePath, mediaType string, fileIDs []string) {
	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	ctx := context.Background()

	taskJSON, err := s.redisClient.Get(ctx, taskKey).Result()
	if err != nil {
		logger.Errorf("OrganizeService[runCandidateTask] 获取任务失败: %v", err)
		return
	}

	var task OrganizeCandidateTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		task = OrganizeCandidateTask{
			TaskID:     taskID,
			SourceID:   sourceID,
			SourcePath: sourcePath,
			MediaType:  mediaType,
			FileIDs:    fileIDs,
			Status:     "failed",
			Progress:   &TaskProgress{Total: 3, Processed: 0, CurrentFile: "任务初始化失败"},
			CreatedAt:  time.Now(),
			Error:      fmt.Sprintf("候选扫描任务数据损坏: %v", err),
		}
		if persistErr := s.persistCandidateTask(ctx, taskKey, &task); persistErr != nil {
			logger.Errorf("OrganizeService[runCandidateTask] 更新任务失败: %v", persistErr)
		}
		return
	}

	defer func() {
		if panicErr := recover(); panicErr != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("候选扫描任务异常中断: %v", panicErr)
			if persistErr := s.persistCandidateTask(ctx, taskKey, &task); persistErr != nil {
				logger.Errorf("OrganizeService[runCandidateTask] 更新异常任务失败: %v", persistErr)
			}
		}
	}()

	task.Status = "processing"
	if task.Progress == nil {
		task.Progress = &TaskProgress{Total: 3}
	}
	task.Progress.Total = 3
	task.Progress.Processed = 1
	task.Progress.CurrentFile = "正在加载媒体源"
	if err := s.persistCandidateTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runCandidateTask] 更新任务状态失败: %v", err)
	}

	task.Progress.Processed = 2
	task.Progress.CurrentFile = "正在扫描候选文件"
	if err := s.persistCandidateTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runCandidateTask] 更新扫描进度失败: %v", err)
	}

	candidates, err := s.ListOrganizeCandidates(sourceID, sourcePath, mediaType, fileIDs)
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		task.Progress.CurrentFile = "扫描失败"
	} else {
		task.Progress.Processed = 3
		task.Progress.CurrentFile = fmt.Sprintf("已完成，共 %d 项", len(candidates))
		task.Status = "completed"
		task.Result = candidates
		task.Total = len(candidates)
	}

	if err := s.persistCandidateTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runCandidateTask] 保存最终任务失败: %v", err)
	}
}

func (s *OrganizeService) GetCandidateTaskStatus(taskID string) (*OrganizeCandidateTask, error) {
	if s.redisClient == nil {
		return nil, fmt.Errorf("候选扫描任务存储未初始化")
	}

	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	ctx := context.Background()
	taskJSON, err := s.redisClient.Get(ctx, taskKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("获取候选扫描任务状态失败: %v", err)
	}

	var task OrganizeCandidateTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return nil, fmt.Errorf("解析候选扫描任务失败: %v", err)
	}
	return &task, nil
}

// StartPreviewTask 启动异步预览任务

func (s *OrganizeService) StartPreviewTask(sourceID int, sourcePath, targetPath, mediaType, template string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) (string, error) {
	if s.redisClient == nil {
		return "", fmt.Errorf("预览任务存储未初始化")
	}

	taskID := fmt.Sprintf("preview_%d_%d", sourceID, time.Now().UnixNano())
	taskKey := fmt.Sprintf("organize:task:%s", taskID)

	task := OrganizePreviewTask{
		TaskID:     taskID,
		SourceID:   sourceID,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		MediaType:  mediaType,
		FileIDs:    fileIDs,
		Status:     "pending",
		Progress:   &TaskProgress{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	ctx := context.Background()
	if err := s.persistPreviewTask(ctx, taskKey, &task); err != nil {
		return "", fmt.Errorf("保存任务失败: %v", err)
	}

	go s.runPreviewTask(taskID, sourceID, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)

	return taskID, nil
}

// runPreviewTask 后台运行预览任务

func (s *OrganizeService) runPreviewTask(taskID string, sourceID int, sourcePath, targetPath, mediaType, template string, fileIDs []string, useCategory bool, manualItems []domain.OrganizeManualOverride) {
	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	ctx := context.Background()

	task := OrganizePreviewTask{
		TaskID:     taskID,
		SourceID:   sourceID,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		MediaType:  mediaType,
		FileIDs:    fileIDs,
		Status:     "pending",
		Progress:   &TaskProgress{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	taskJSON, err := s.redisClient.Get(ctx, taskKey).Result()
	if err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 获取任务失败: %v", err)
		return
	}

	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 解析任务失败: %v", err)
		task.Status = "failed"
		task.Error = fmt.Sprintf("预览任务数据损坏: %v", err)
		if persistErr := s.persistPreviewTask(ctx, taskKey, &task); persistErr != nil {
			logger.Errorf("OrganizeService[runPreviewTask] 更新任务失败: %v", persistErr)
		}
		return
	}

	defer func() {
		if panicErr := recover(); panicErr != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("预览任务异常中断: %v", panicErr)
			if persistErr := s.persistPreviewTask(ctx, taskKey, &task); persistErr != nil {
				logger.Errorf("OrganizeService[runPreviewTask] 更新异常任务失败: %v", persistErr)
			}
		}
	}()

	task.Status = "processing"
	if err := s.persistPreviewTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 更新任务状态失败: %v", err)
	}

	previews, err := s.PreviewOrganize(sourceID, sourcePath, targetPath, mediaType, template, fileIDs, useCategory, manualItems)
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		conflictCount := 0
		failedCount := 0
		for _, preview := range previews {
			if preview.Conflict {
				conflictCount++
			}
			if preview.IdentifyError != "" {
				failedCount++
			}
		}

		task.Status = "completed"
		task.Result = &TaskResult{
			Previews: previews,
			Summary: &TaskSummary{
				Total:       len(previews),
				Conflicts:   conflictCount,
				Failed:      failedCount,
				Processable: len(previews) - failedCount,
			},
		}
	}

	if err := s.persistPreviewTask(ctx, taskKey, &task); err != nil {
		logger.Errorf("OrganizeService[runPreviewTask] 保存最终任务失败: %v", err)
	}
}

// GetPreviewTaskStatus 获取预览任务状态

func (s *OrganizeService) GetPreviewTaskStatus(taskID string) (*OrganizePreviewTask, error) {
	taskKey := fmt.Sprintf("organize:task:%s", taskID)
	ctx := context.Background()

	taskJSON, err := s.redisClient.Get(ctx, taskKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("获取任务状态失败: %v", err)
	}

	var task OrganizePreviewTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return nil, fmt.Errorf("解析任务失败: %v", err)
	}

	return &task, nil
}

// CheckRestorableTask 检查是否有可恢复的任务

func (s *OrganizeService) CheckRestorableTask(sourceID int, sourcePath string, fileIDs []string) (*OrganizePreviewTask, error) {
	// StartPreviewTask 使用的是 organize:task:preview_<sourceID>_<timestamp>
	pattern := fmt.Sprintf("organize:task:preview_%d_*", sourceID)
	ctx := context.Background()

	keys, err := s.redisClient.Keys(ctx, pattern).Result()
	if err != nil || len(keys) == 0 {
		return nil, nil
	}

	for _, key := range keys {
		taskJSON, err := s.redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var task OrganizePreviewTask
		if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
			continue
		}

		if task.Status == "processing" || task.Status == "pending" {
			return &task, nil
		}
	}

	return nil, nil
}

// OrganizeDirectory 整理目录
// 参数:
//   - sourceID: 媒体源ID
//   - sourcePath: 源目录路径
//   - targetPath: 目标目录路径
//   - conflictPolicy: 冲突处理策略 (skip: 跳过, overwrite: 覆盖, auto_rename: 自动重命名)
//   - moveFiles: 是否删除源文件 (true: 移动, false: 复制)
//   - fileIDs: 指定要处理的文件ID列表（可空）
//   - useCategory: 是否使用媒体分类自动生成目标目录
//
// 返回:
//   - []OrganizeResult: 整理结果列表
//   - error: 错误信息
