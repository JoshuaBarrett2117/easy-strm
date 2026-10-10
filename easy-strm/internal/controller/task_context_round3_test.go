package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/dao"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func TestTaskRetryCallbacksRetainRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, kind := range []string{"strm_generate", "share_sync"} {
		t.Run(kind, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
			defer client.Close()
			tasks := service.NewTaskService(dao.NewTaskRedisDAO(client))
			if err := tasks.Create("retry-fixture", kind, "fixture"); err != nil {
				t.Fatal(err)
			}
			if err := tasks.SetError("retry-fixture", "isolated failure"); err != nil {
				t.Fatal(err)
			}
			controller := NewTaskController(tasks)
			called := false
			check := func(ctx context.Context, id string, task map[string]interface{}) {
				called = true
				if logger.RequestID(ctx) != "retry-request" || logger.TraceID(ctx) != "retry-action" || id != "retry-fixture" || task["task_type"] != kind {
					t.Fatal("retry lost request or task")
				}
			}
			controller.SetRetryStrmTask(func(ctx context.Context, id string, task map[string]interface{}) error {
				check(ctx, id, task)
				return nil
			})
			controller.SetRetryShareSyncTask(func(ctx context.Context, id string, task map[string]interface{}) (string, error) {
				check(ctx, id, task)
				return "new-sync-fixture", nil
			})
			router := gin.New()
			router.POST("/tasks/:task_id/resume", controller.Resume)
			ctx := logger.WithTraceID(logger.WithRequestID(context.Background(), "retry-request"), "retry-action")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tasks/retry-fixture/resume", nil).WithContext(ctx))
			if response.Code != http.StatusOK || !called {
				t.Fatalf("retry not dispatched: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
