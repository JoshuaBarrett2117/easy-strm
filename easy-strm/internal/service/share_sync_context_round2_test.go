package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"easy-strm/internal/pkg/logger"
)

func TestShareSyncTaskAndWorkerFailureLogsRetainRequestContext(t *testing.T) {
	checkShareSyncRequestContext(t, false)
}

func TestShareSyncRetryWorkerLogsRetainRequestContext(t *testing.T) {
	checkShareSyncRequestContext(t, true)
}

func checkShareSyncRequestContext(t *testing.T, retry bool) {
	t.Helper()
	output := captureEmbyProxyLog(t)
	service, mock, _ := operationFixture(t)
	service.parser = shareSyncParser{err: errors.New("synthetic parser failure")}
	expectSyncRecord(mock)
	parent := logger.WithTraceID(logger.WithRequestID(context.Background(), "sync-request"), "sync-action")
	var taskID string
	var err error
	if retry {
		taskID, err = service.RetryBatchSyncTaskContext(parent, "old-sync-fixture", map[string]interface{}{"metadata": map[string]interface{}{"record_ids": []interface{}{float64(7)}}})
	} else {
		taskID, err = service.StartRecordSync(parent, 7)
	}
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, service, taskID, "failed")
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
		found++
		if entry.RequestID != "sync-request" || entry.TraceID != "sync-action" || entry.TaskID != taskID {
			t.Fatalf("分享同步任务日志丢失入口关联: %s", line)
		}
	}
	if found < 3 {
		t.Fatalf("未捕获 DAO、任务与 worker 失败日志: %d", found)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
