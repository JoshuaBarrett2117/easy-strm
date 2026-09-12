package service

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"

	"easy-strm/internal/dao"
)

func setupTaskRedisMock(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}
	for _, pattern := range []string{"easy_strm:task:*", "easy_strm:task:cancel:*", "easy_strm:task:progress:*"} {
		keys, _ := client.Keys(ctx, pattern).Result()
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
	}
	_ = client.Del(ctx, "easy_strm:task:list").Err()
	return client
}

func TestTaskServiceCreate(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-001"
	err := svc.Create(taskID, "strm_generate", "娴嬭瘯浠诲姟")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	task, err := svc.Get(taskID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task == nil {
		t.Fatal("Task should not be nil")
	}
	if task["task_id"] != taskID {
		t.Fatalf("Task ID mismatch: got %v, want %s", task["task_id"], taskID)
	}
	if task["status"] != "pending" {
		t.Fatalf("Task status mismatch: got %v, want pending", task["status"])
	}
}

func TestTaskServiceCreateWithPriority(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-priority"
	err := svc.CreateWithPriority(taskID, "strm_generate", "楂樹紭鍏堢骇浠诲姟", 1)
	if err != nil {
		t.Fatalf("CreateWithPriority failed: %v", err)
	}

	task, err := svc.Get(taskID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task["priority"] != float64(1) {
		t.Fatalf("Priority mismatch: got %v, want 1", task["priority"])
	}
}

func TestTaskServiceUpdateStatus(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-status"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")

	err := svc.UpdateStatus(taskID, "running")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	if task["status"] != "running" {
		t.Fatalf("Status not updated: got %v", task["status"])
	}
}

func TestTaskServiceUpdateProgress(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-progress"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")

	err := svc.UpdateProgress(taskID, 100, 50, 45, 5)
	if err != nil {
		t.Fatalf("UpdateProgress failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	if task["total_files"].(float64) != 100 {
		t.Fatalf("Total files mismatch: got %v", task["total_files"])
	}
	if task["processed_files"].(float64) != 50 {
		t.Fatalf("Processed files mismatch: got %v", task["processed_files"])
	}
	if task["success_files"].(float64) != 45 {
		t.Fatalf("Success files mismatch: got %v", task["success_files"])
	}
	if task["failed_files"].(float64) != 5 {
		t.Fatalf("Failed files mismatch: got %v", task["failed_files"])
	}
}

func TestTaskServiceCancel(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-cancel"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")

	err := svc.Cancel(taskID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	if task["status"] != "cancelled" {
		t.Fatalf("Task should be cancelled, got: %v", task["status"])
	}

	isCancelled := svc.IsCancelled(taskID)
	if !isCancelled {
		t.Fatal("IsCancelled should return true after cancel")
	}
}

func TestTaskServiceCancelInvalidStatus(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-cancel-invalid"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")

	svc.UpdateStatus(taskID, "completed")

	err := svc.Cancel(taskID)
	if err == nil {
		t.Fatal("Cancel should fail for completed task")
	}
}

func TestTaskServiceResume(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-resume"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")
	svc.UpdateStatus(taskID, "cancelled")
	svc.UpdateProgress(taskID, 100, 30, 25, 5)

	err := svc.Resume(taskID)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	if task["status"] != "pending" {
		t.Fatalf("Task should be pending after resume, got: %v", task["status"])
	}
}

func TestTaskServiceResumeInvalidStatus(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-resume-invalid"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")
	svc.UpdateStatus(taskID, "running")

	err := svc.Resume(taskID)
	if err == nil {
		t.Fatal("Resume should fail for running task")
	}
}

func TestTaskServiceProgressTracking(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-progress-track"

	err := svc.AddProcessedFileID(taskID, "file-001")
	if err != nil {
		t.Fatalf("AddProcessedFileID failed: %v", err)
	}
	err = svc.AddProcessedFileID(taskID, "file-002")
	if err != nil {
		t.Fatalf("AddProcessedFileID failed: %v", err)
	}

	isProcessed := svc.IsFileProcessed(taskID, "file-001")
	if !isProcessed {
		t.Fatal("file-001 should be marked as processed")
	}

	isProcessed = svc.IsFileProcessed(taskID, "file-003")
	if isProcessed {
		t.Fatal("file-003 should not be marked as processed")
	}
}

func TestTaskServiceGetAll(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	_ = svc.Create("task-1", "strm_generate", "浠诲姟1")
	_ = svc.Create("task-2", "incremental_sync", "浠诲姟2")
	_ = svc.Create("task-3", "strm_generate", "浠诲姟3")

	tasks, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(tasks) < 3 {
		t.Fatalf("Expected at least 3 tasks, got %d", len(tasks))
	}
}

func TestTaskServiceDelete(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-delete"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")

	err := svc.Delete(taskID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	if task != nil {
		t.Fatal("Task should be nil after delete")
	}
}

func TestTaskServiceSetError(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-error"
	_ = svc.Create(taskID, "strm_generate", "娴嬭瘯")

	errMsg := "缃戠粶瓒呮椂"
	err := svc.SetError(taskID, errMsg)
	if err != nil {
		t.Fatalf("SetError failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	if task["error_message"] != errMsg {
		t.Fatalf("Error message mismatch: got %v", task["error_message"])
	}
	if task["status"] != "failed" {
		t.Fatalf("Status should be failed, got: %v", task["status"])
	}
}

func TestTaskServiceUpdateMetadata(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-metadata"
	_ = svc.Create(taskID, "watch_auto_organize", "115鑷姩鏁寸悊-娴嬭瘯婧?")

	metadata := map[string]interface{}{
		"source_name":          "娴嬭瘯婧?",
		"organize_target_path": "/鐢靛奖搴?",
		"watch_interval":       1800,
		"trigger_mode":         "polling",
	}

	if err := svc.UpdateMetadata(taskID, metadata); err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	task, _ := svc.Get(taskID)
	gotMetadata, ok := task["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata map, got %#v", task["metadata"])
	}
	if gotMetadata["source_name"] != "娴嬭瘯婧?" {
		t.Fatalf("source_name mismatch: got %#v", gotMetadata["source_name"])
	}
	if gotMetadata["organize_target_path"] != "/鐢靛奖搴?" {
		t.Fatalf("organize_target_path mismatch: got %#v", gotMetadata["organize_target_path"])
	}
	if gotMetadata["watch_interval"] != float64(1800) {
		t.Fatalf("watch_interval mismatch: got %#v", gotMetadata["watch_interval"])
	}
}

func TestTaskServiceContextCancel(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	taskID := "test-task-context"

	ctx, cancel := context.WithCancel(context.Background())
	svc.RegisterCancel(taskID, cancel)

	svc.CancelContext(taskID)

	select {
	case <-ctx.Done():
	default:
		t.Fatal("Context should be cancelled")
	}

	svc.RemoveCancel(taskID)
}

func TestTaskServiceGetUnified(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	_ = svc.Create("task-unified-1", "strm_generate", "STRM鐢熸垚浠诲姟")
	_ = svc.Create("task-unified-2", "incremental_sync", "澧為噺鍚屾浠诲姟")

	tasks, err := svc.GetUnified()
	if err != nil {
		t.Fatalf("GetUnified failed: %v", err)
	}
	if len(tasks) < 2 {
		t.Fatalf("Expected at least 2 tasks, got %d", len(tasks))
	}
}

func TestTaskServiceGetUnifiedOrdersByCreateTimeDesc(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	taskDAO := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(taskDAO)

	if err := svc.CreateWithPriority("task-old-high-priority", "strm_generate", "旧任务", 1); err != nil {
		t.Fatalf("CreateWithPriority failed: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	if err := svc.CreateWithPriority("task-new-low-priority", "strm_generate", "新任务", 9); err != nil {
		t.Fatalf("CreateWithPriority failed: %v", err)
	}

	tasks, err := svc.GetUnified()
	if err != nil {
		t.Fatalf("GetUnified failed: %v", err)
	}
	if len(tasks) < 2 {
		t.Fatalf("Expected at least 2 tasks, got %d", len(tasks))
	}

	if got := tasks[0]["task_id"]; got != "task-new-low-priority" {
		t.Fatalf("Expected newest task first, got %v", got)
	}
	if got := tasks[1]["task_id"]; got != "task-old-high-priority" {
		t.Fatalf("Expected older task second, got %v", got)
	}
}

func TestTaskServiceMultiplePriorities(t *testing.T) {
	client := setupTaskRedisMock(t)
	defer client.Close()

	dao := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(dao)

	_ = svc.CreateWithPriority("task-p5", "strm_generate", "浣庝紭鍏堢骇", 5)
	_ = svc.CreateWithPriority("task-p1", "strm_generate", "楂樹紭鍏堢骇", 1)
	_ = svc.CreateWithPriority("task-p3", "strm_generate", "涓紭鍏堢骇", 3)

	tasks, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	for _, task := range tasks {
		t.Logf("Task: %s, Priority: %v", task["task_id"], task["priority"])
	}
}
