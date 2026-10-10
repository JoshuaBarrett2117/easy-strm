package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

func TestFileManagerTransferTaskAndFailureLogsRetainRequestContext(t *testing.T) {
	output := captureEmbyProxyLog(t)
	tasks := shareProgressFaultTasks(t, &shareProgressFaultHook{})
	service := NewFileManagerService(&fakeFileManagerMediaSources{items: map[int]*domain.MediaSource{
		1: {ID: 1, SourceType: domain.SourceTypeLocal, Path: t.TempDir()},
		2: {ID: 2, SourceType: domain.SourceTypeLocal, Path: t.TempDir()},
	}}, &fakeFileManagerAccounts{items: map[int]*domain.Cloud115{}}, &fakeFileManagerCloudClient{}, tasks)
	parent := logger.WithTraceID(logger.WithRequestID(context.Background(), "transfer-request"), "transfer-action")
	response, err := service.StartTransferContext(parent, domain.FileManagerTransferRequest{
		Operation: "copy", Source: domain.FileManagerLocationRef{Type: "local", ID: 1},
		Target: domain.FileManagerLocationRef{Type: "local", ID: 2},
		Items:  []domain.FileManagerTransferItem{{ID: "missing.mkv", Path: "missing.mkv", Name: "missing.mkv"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		task, err := tasks.Get(response.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if task["status"] == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("缺失文件未产生预期失败终态")
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
		found++
		if entry.RequestID != "transfer-request" || entry.TraceID != "transfer-action" || entry.TaskID != response.TaskID {
			t.Fatalf("传输任务日志丢失入口关联: %s", line)
		}
	}
	if found < 3 {
		t.Fatalf("未捕获 DAO、任务与传输失败日志: %d", found)
	}
}
