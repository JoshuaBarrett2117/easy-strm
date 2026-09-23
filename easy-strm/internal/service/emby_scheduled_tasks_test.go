package service

import (
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateScheduledTaskTriggers(t *testing.T) {
	for _, tc := range []struct {
		name, body, id string
		status         int
		wantError      bool
	}{
		{"修改每天时间", `[{"Type":"DailyTrigger","TimeOfDayTicks":216000000000,"MaxRuntimeTicks":600000000}]`, "task1", 204, false},
		{"删除全部规则", `[]`, "task1", 204, false},
		{"任务不存在", `[]`, "missing", 204, true},
		{"上游拒绝保存", `[]`, "task1", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Emby-Token") != "test-key" {
					t.Error("缺少实例凭据")
				}
				if r.Method == http.MethodGet && r.URL.Path == "/emby/ScheduledTasks" {
					fmt.Fprint(w, `[{"Id":"task1","State":"Running"}]`)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/emby/ScheduledTasks/task1/Triggers" {
					t.Errorf("意外请求 %s %s", r.Method, r.URL)
				}
				var actual, expected interface{}
				if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
					t.Error(err)
				}
				json.Unmarshal([]byte(tc.body), &expected)
				a, _ := json.Marshal(actual)
				e, _ := json.Marshal(expected)
				if string(a) != string(e) {
					t.Errorf("规则丢失: %s != %s", a, e)
				}
				posts++
				w.WriteHeader(tc.status)
			}))
			defer remote.Close()
			svc, mock, _, cleanup := newEmbyManagementTestService(t, func(http.ResponseWriter, *http.Request) {})
			defer cleanup()
			expectEmbyServer(t, mock, remote.URL)
			var req domain.EmbyTaskTriggersRequest
			json.Unmarshal([]byte(tc.body), &req.Triggers)
			err := svc.UpdateScheduledTaskTriggers(1, tc.id, req)
			if (err != nil) != tc.wantError {
				t.Fatalf("错误结果: %v", err)
			}
			wantPosts := 1
			if tc.id == "missing" {
				wantPosts = 0
			}
			if posts != wantPosts {
				t.Fatalf("写入次数=%d", posts)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateEmbyTaskTriggers(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`[]`, true}, {`null`, false},
		{`[{"Type":"DailyTrigger","TimeOfDayTicks":0}]`, true},
		{`[{"Type":"DailyTrigger","TimeOfDayTicks":864000000000}]`, false},
		{`[{"Type":"DailyTrigger","TimeOfDayTicks":"0"}]`, false},
		{`[{"Type":"WeeklyTrigger","TimeOfDayTicks":0,"DayOfWeek":"Monday"}]`, true},
		{`[{"Type":"WeeklyTrigger","TimeOfDayTicks":0,"DayOfWeek":"Bad"}]`, false},
		{`[{"Type":"IntervalTrigger","IntervalTicks":600000000}]`, true},
		{`[{"Type":"IntervalTrigger","IntervalTicks":0}]`, false},
		{`[{"Type":"IntervalTrigger","IntervalTicks":1.5}]`, false},
		{`[{"Type":"StartupTrigger"}]`, true},
		{`[{"Type":"Unknown"}]`, false}, {`[null]`, false},
	} {
		var rules []map[string]interface{}
		json.Unmarshal([]byte(tc.body), &rules)
		if err := ValidateEmbyTaskTriggers(rules); (err == nil) != tc.valid {
			t.Errorf("%s: %v", tc.body, err)
		}
	}
}

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
