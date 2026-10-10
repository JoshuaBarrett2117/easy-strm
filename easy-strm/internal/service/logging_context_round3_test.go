package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestActualCronHTTPAndScheduledTaskTimeline(t *testing.T) {
	output := captureEmbyProxyLog(t)
	service, mock := cronTestService(t)
	service.Register(CronHandler{Key: "log_cleanup", Execute: func(ctx context.Context, _ *domain.CronTask, id string) (string, error) {
		logger.WithContext(ctx, "cron_worker").Log(logger.INFO, "处理器执行", logger.Fields{"event": "cron_worker"}, nil)
		return "合成处理器完成", nil
	}})
	for _, origin := range []string{"manual", "scheduled"} {
		parent := context.Background()
		requestID, traceID := "-", "-"
		if origin == "manual" {
			requestID, traceID = "cron-http-request", "cron-action"
			parent = logger.WithTraceID(logger.WithRequestID(parent, requestID), traceID)
		}
		expectCronRead(mock, 9, 0, "log_cleanup", "enabled", true)
		mock.ExpectExec(`INSERT INTO t_cron_task_run`).WithArgs(9, sqlmock.AnyArg(), origin, "pending").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`UPDATE t_cron_task_run SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "running", "", 9).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs("success", "合成处理器完成", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "success", "合成处理器完成", 9).WillReturnResult(sqlmock.NewResult(0, 1))
		taskID, err := service.RunContext(parent, 9, origin)
		if err != nil {
			t.Fatal(err)
		}
		waitCronIdle(t, service)
		found := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
			var entry struct {
				RequestID string                 `json:"request_id"`
				TraceID   string                 `json:"trace_id"`
				TaskID    string                 `json:"task_id"`
				Fields    map[string]interface{} `json:"fields"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.TaskID != taskID {
				continue
			}
			if entry.RequestID != requestID || entry.TraceID != traceID {
				t.Fatalf("cron context lost: %s", line)
			}
			event, _ := entry.Fields["event"].(string)
			found[event] = true
		}
		for _, event := range []string{"cron_start", "cron_worker", "cron_complete"} {
			if !found[event] {
				t.Fatalf("task=%s missing %s", taskID, event)
			}
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		t.Logf("origin=%s task=%s actual CronService goroutine timeline joined", origin, taskID)
	}
	t.Logf("validated captured cron logs:\n%s", output.String())
}

func TestShareIdentifyPreparationLogsRetainRequestContext(t *testing.T) {
	output := captureEmbyProxyLog(t)
	service, mock, _ := operationFixture(t)
	mock.ExpectQuery(`SELECT s.id,s.media_type,f.id`).WillReturnError(errors.New("isolated preparation failure"))
	ctx := logger.WithTraceID(logger.WithRequestID(context.Background(), "identify-request"), "identify-action")
	taskID, err := service.StartBatchIdentify(ctx, nil, false, []int{7})
	if err != nil {
		t.Fatal(err)
	}
	waitOperationStatus(t, service, taskID, "failed")
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["request_id"] != "identify-request" || entry["trace_id"] != "identify-action" || entry["task_id"] != taskID {
			t.Fatalf("identify context lost: %s", line)
		}
	}
	if !strings.Contains(output.String(), `"event":"identify_start"`) {
		t.Fatal("no actual identify log")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyRefreshFailureLogsRetainRequestContext(t *testing.T) {
	output := captureEmbyProxyLog(t)
	remote := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	service, mock, tasks, cleanup := newEmbyManagementTestService(t, nil)
	defer cleanup()
	service.httpClient = remote.Client()
	expectEmbyServer(t, mock, remote.URL)
	expectEmbyServer(t, mock, remote.URL)
	ctx, cancel := context.WithCancel(logger.WithTraceID(logger.WithRequestID(context.Background(), "emby-request"), "emby-action"))
	taskID, err := service.StartRefreshContext(ctx, 1, "fixture-library")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := tasks.Get(taskID)
		if err != nil {
			t.Fatal(err)
		}
		if task["status"] == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Emby fixture did not terminate")
		}
		time.Sleep(time.Millisecond)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["task_id"] != taskID {
			continue
		}
		if entry["request_id"] != "emby-request" || entry["trace_id"] != "emby-action" {
			t.Fatalf("Emby context lost: %s", line)
		}
		if entry["module"] == "emby_task" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing actual Emby failure log")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
