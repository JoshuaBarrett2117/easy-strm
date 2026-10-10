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

func TestActualAsyncExportHTTPAndScheduledTaskTimeline(t *testing.T) {
	output := captureEmbyProxyLog(t)
	service, store, _ := strmFixture(t)
	store.sources = []domain.ShareStrmSource{phaseSource(1)}
	service.tasks = shareProgressFaultTasks(t, &shareProgressFaultHook{})
	for _, origin := range []string{"http", "scheduled"} {
		parent, cancel := context.WithCancel(context.Background())
		if origin == "http" {
			parent = logger.WithTraceID(logger.WithRequestID(parent, "async-http-request"), "async-action")
		}
		id, err := service.StartExportContext(parent, domain.ShareLibraryQuery{})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			task, err := service.tasks.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			status, _ := task["status"].(string)
			if status == "completed" {
				break
			}
			if status == "failed" || status == "cancelled" || time.Now().After(deadline) {
				t.Fatalf("异步任务未完成: %+v", task)
			}
			time.Sleep(5 * time.Millisecond)
		}
		found := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
			var entry struct {
				RequestID string                 `json:"request_id"`
				TraceID   string                 `json:"trace_id"`
				TaskID    string                 `json:"task_id"`
				Module    string                 `json:"module"`
				Fields    map[string]interface{} `json:"fields"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.TaskID != id || entry.Module != "share_export" {
				continue
			}
			expectedRequest, expectedTrace := "-", "-"
			if origin == "http" {
				expectedRequest, expectedTrace = "async-http-request", "async-action"
			}
			if entry.RequestID != expectedRequest || entry.TraceID != expectedTrace {
				t.Fatalf("异步上下文丢失: %s", line)
			}
			event, _ := entry.Fields["event"].(string)
			found[event] = true
			if event == "export_start" && entry.Fields["origin"] != origin {
				t.Fatalf("origin 错误: %s", line)
			}
		}
		for _, event := range []string{"export_start", "export_single", "export_complete"} {
			if !found[event] {
				t.Fatalf("task=%s 缺少 %s", id, event)
			}
		}
		t.Logf("origin=%s task=%s real asynchronous export start/file/complete logs reconstructed", origin, id)
	}
}
