package controller

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func TestShareStrmCronResumeCannotReplayOldFull(t *testing.T) {
	for _, handler := range []string{"share_strm_incremental_export", "share_strm_full_reconciliation"} {
		t.Run(handler, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
			defer client.Close()
			tasks := service.NewTaskService(dao.NewTaskRedisDAO(client))
			if err := tasks.Create("cron_failed", "strm_generate", handler); err != nil {
				t.Fatal(err)
			}
			if err := tasks.UpdateMetadata("cron_failed", map[string]interface{}{"cron_handler": handler, "share_export": true}); err != nil {
				t.Fatal(err)
			}
			if err := tasks.SetError("cron_failed", "interrupted"); err != nil {
				t.Fatal(err)
			}
			controller := NewTaskController(tasks)
			controller.SetRetryStrmTask(func(context.Context, string, map[string]interface{}) error {
				t.Error("must not dispatch old full")
				return nil
			})
			router := gin.New()
			router.POST("/tasks/:task_id/resume", controller.Resume)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tasks/cron_failed/resume", nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%d %s", response.Code, response.Body)
			}
			if err := tasks.Resume("cron_failed"); err == nil {
				t.Fatal("generic service resume must reject cron replay")
			}
			task, err := tasks.Get("cron_failed")
			if err != nil || task["status"] != "failed" {
				t.Fatalf("false pending state: %v %v", task, err)
			}
		})
	}
}
