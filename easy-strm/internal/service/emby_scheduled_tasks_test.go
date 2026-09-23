package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartScheduledTask(t *testing.T) {
	for _, tc := range []struct {
		name, state, id string
		status          int
		wantError       bool
	}{
		{"触发成功", "Idle", "task1", 204, false},
		{"拒绝重复执行", "Running", "task1", 204, true},
		{"不存在", "Idle", "missing", 204, true},
		{"上游失败", "Idle", "task1", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Emby-Token") != "test-key" {
					t.Error("缺少实例凭据")
				}
				if r.Method == http.MethodGet && r.URL.Path == "/emby/ScheduledTasks" {
					fmt.Fprintf(w, `[{"Id":"task1","Name":"扫描","State":%q}]`, tc.state)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/emby/ScheduledTasks/Running/task1" {
					t.Errorf("意外请求: %s %s", r.Method, r.URL)
				}
				posts++
				w.WriteHeader(tc.status)
			}))
			defer remote.Close()
			svc, mock, tasks, cleanup := newEmbyManagementTestService(t, func(http.ResponseWriter, *http.Request) {})
			defer cleanup()
			expectEmbyServer(t, mock, remote.URL)
			id, err := svc.StartScheduledTask(1, tc.id)
			if (err != nil) != tc.wantError {
				t.Fatalf("结果错误: %v", err)
			}
			if tc.state != "Idle" || tc.id == "missing" {
				if posts != 0 {
					t.Fatal("不应触发")
				}
			} else if posts != 1 {
				t.Fatal("应仅触发一次")
			}
			if !tc.wantError {
				record, err := tasks.Get(id)
				if err != nil || record == nil || !strings.HasPrefix(id, "emby-") {
					t.Fatalf("缺少任务记录: %v", err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
