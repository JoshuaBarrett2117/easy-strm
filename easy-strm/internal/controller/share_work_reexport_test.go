package controller

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func TestShareWorkReexportEmptyNeverFallsBack(test *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{}`, `{"work_keys":[]}`, `{"work_keys":[""]}`} {
		test.Run(body, func(test *testing.T) {
			stub := &strmActionsStub{}
			controller := &ShareStrmController{s: stub}
			router := gin.New()
			router.POST("/export", func(ctx *gin.Context) {
				ctx.Set("userID", 7)
				controller.Export(ctx)
			})
			request := httptest.NewRequest(http.MethodPost, "/export?mode=work_reexport", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				test.Fatalf("空范围必须拒绝，不能落入普通全量导出：status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type workReexportActionsStub struct {
	strmActionsStub
	request domain.ShareWorkReexportRequest
	called  bool
}

func (stub *workReexportActionsStub) StartWorkReexportContext(_ context.Context, request domain.ShareWorkReexportRequest) (string, error) {
	stub.request, stub.called = request, true
	return "share_work_reexport_test", stub.err
}

func TestShareWorkReexportHTTPContract(test *testing.T) {
	valid := `{"work_keys":["tmdb:tv:1","tmdb:movie:2"],"reason":"人工核验后修复映射","expected_output_path":"/isolated/export","expected_config_revision":3}`
	for _, scenario := range []string{"success", "start-error", "unauthenticated", "unknown-field", "trailing-json", "bad-json", "null-json", "oversized", "duplicate", "cap", "mixed-filter", "wrong-mode", "duplicate-mode", "missing-mode"} {
		test.Run(scenario, func(test *testing.T) {
			stub := &workReexportActionsStub{}
			controller := &ShareStrmController{s: stub}
			body, route, actor, status := valid, "/export?mode=work_reexport", 7, 400
			switch scenario {
			case "success":
				status = 200
			case "start-error":
				stub.err = errors.New("任务创建失败")
			case "unauthenticated":
				actor, status = 0, 401
			case "unknown-field":
				body = strings.TrimSuffix(valid, "}") + `,"full":true}`
			case "trailing-json":
				body = valid + `{}`
			case "bad-json":
				body = "{"
			case "null-json":
				body = "null"
			case "oversized":
				body = strings.Replace(valid, "人工核验后修复映射", strings.Repeat("x", 33000), 1)
			case "duplicate":
				body = strings.Replace(valid, "tmdb:movie:2", "tmdb:tv:1", 1)
			case "cap":
				var request domain.ShareWorkReexportRequest
				_ = json.Unmarshal([]byte(valid), &request)
				request.WorkKeys = make([]string, domain.ShareWorkReexportMaxWorks+1)
				raw, _ := json.Marshal(request)
				body = string(raw)
			case "mixed-filter":
				route += "&media_type=tv"
			case "wrong-mode":
				route = "/export?mode=work_reexpor"
			case "duplicate-mode":
				route += "&mode=work_reexport"
			case "missing-mode":
				route = "/export"
			}
			router := gin.New()
			router.POST("/export", func(ctx *gin.Context) { ctx.Set("userID", actor); controller.Export(ctx) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, route, strings.NewReader(body)))
			if response.Code != status || stub.called != (scenario == "success" || scenario == "start-error") {
				test.Fatalf("status=%d expected=%d called=%v body=%s", response.Code, status, stub.called, response.Body)
			}
			if scenario == "success" && (stub.request.ActorUserID != 7 || len(stub.request.WorkKeys) != 2 || !strings.Contains(response.Body.String(), `"task_id":"share_work_reexport_test"`)) {
				test.Fatal("认证人、范围或可追踪任务丢失")
			}
		})
	}
}

func TestShareWorkReexportHTTPResumeCannotDispatchFull(test *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(test).Addr()})
	defer client.Close()
	tasks := service.NewTaskService(dao.NewTaskRedisDAO(client))
	if err := tasks.Create("share_work_reexport_failed", "strm_generate", "作品重导"); err != nil {
		test.Fatal(err)
	}
	if err := tasks.SetError("share_work_reexport_failed", "中断"); err != nil {
		test.Fatal(err)
	}
	controller := NewTaskController(tasks)
	controller.SetRetryStrmTask(func(context.Context, string, map[string]interface{}) error {
		test.Fatal("禁止回退全量")
		return nil
	})
	router := gin.New()
	router.POST("/tasks/:task_id/resume", controller.Resume)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tasks/share_work_reexport_failed/resume", nil))
	if response.Code != 400 {
		test.Fatalf("%d %s", response.Code, response.Body)
	}
}
