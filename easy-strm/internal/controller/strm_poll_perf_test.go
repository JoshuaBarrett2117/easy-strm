package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLegacyStrmPollingCancelledSkipsRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controller := NewStrmController(nil)
	called := false
	controller.SetGetTaskByID(func(context.Context, string) (interface{}, error) {
		called = true
		return map[string]string{"task_id": "cancelled"}, nil
	})
	engine := gin.New()
	engine.GET("/strm/task/:task_id", controller.GetTaskStatus)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/strm/task/cancelled", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if called || response.Code != http.StatusInternalServerError {
		t.Fatalf("已取消轮询不得调用任务读取：调用=%v 状态=%d", called, response.Code)
	}
}

func TestLegacyStrmPollingContextBudgetAndResponse(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		result interface{}
		err    error
		status int
	}{
		{name: "成功", result: map[string]string{"status": "running"}, status: http.StatusOK},
		{name: "缺失", status: http.StatusNotFound},
		{name: "故障", err: errors.New("模拟任务读取故障"), status: http.StatusInternalServerError},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			controller := NewStrmController(nil)
			type requestKey struct{}
			controller.SetGetTaskByID(func(ctx context.Context, taskID string) (interface{}, error) {
				deadline, present := ctx.Deadline()
				if taskID != "fixture" || ctx.Value(requestKey{}) != "测试标识" || !present || time.Until(deadline) > 5*time.Second {
					t.Fatal("轮询上下文、任务ID或总预算未透传")
				}
				return scenario.result, scenario.err
			})
			engine := gin.New()
			engine.GET("/strm/task/:task_id", controller.GetTaskStatus)
			ctx := context.WithValue(context.Background(), requestKey{}, "测试标识")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/strm/task/fixture", nil).WithContext(ctx))
			if response.Code != scenario.status {
				t.Fatalf("响应状态改变：%d %s", response.Code, response.Body.String())
			}
			if scenario.status == http.StatusOK && !strings.Contains(response.Body.String(), `"data":{"status":"running"}`) {
				t.Fatalf("旧data响应契约改变：%s", response.Body.String())
			}
		})
	}
}
