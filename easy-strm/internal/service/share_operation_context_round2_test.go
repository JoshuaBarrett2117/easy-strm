package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/pkg/logger"
)

func TestShareOperationQueueRetainsRequestContextAfterParentCancellation(t *testing.T) {
	output := captureEmbyProxyLog(t)
	service, mock, _ := operationFixture(t)
	expectOperationTarget(mock, 1)
	expectOperationClear(mock, 2)
	parent := logger.WithTraceID(logger.WithRequestID(context.Background(), "queue-request"), "queue-action")
	parent, cancel := context.WithCancel(parent)
	taskID, err := service.StartClearMedia(parent, []int{1})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, service, taskID, "completed")
	deadline := time.Now().Add(time.Second)
	for {
		service.operations.mu.Lock()
		active := service.operations.active[taskID]
		service.operations.mu.Unlock()
		if !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("队列没有退出")
		}
		time.Sleep(time.Millisecond)
	}
	found := 0
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry struct {
			RequestID string `json:"request_id"`
			TraceID   string `json:"trace_id"`
			TaskID    string `json:"task_id"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.TaskID != taskID {
			continue
		}
		found++
		if entry.RequestID != "queue-request" || entry.TraceID != "queue-action" {
			t.Fatalf("持久化操作丢失入口关联: %s", line)
		}
	}
	if found < 2 {
		t.Fatalf("未捕获真实任务与 DAO 日志: %d", found)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
