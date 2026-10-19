package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

type notificationScopeControllerConfigs struct {
	values   map[string]string
	readErr  error
	writeErr error
	writes   int
}

func (store *notificationScopeControllerConfigs) GetHotConfigs(context.Context, ...string) (map[string]string, error) {
	return store.values, store.readErr
}
func (store *notificationScopeControllerConfigs) BatchUpsertContext(ctx context.Context, values map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.writes++
	if store.writeErr != nil {
		return store.writeErr
	}
	store.values = values
	return nil
}

type notificationScopeControllerCron struct{ err error }

func (store *notificationScopeControllerCron) GetNotificationTasks(context.Context) ([]*domain.CronTask, error) {
	return []*domain.CronTask{{ID: 8, TaskKey: "key:a", TaskName: "任务甲", Handler: "shared", Status: "disabled"}, {ID: 9, TaskName: "旧任务", Handler: "legacy", Status: "enabled"}}, store.err
}

func notificationScopeTestRouter(configs *notificationScopeControllerConfigs, cron *notificationScopeControllerCron) *gin.Engine {
	gin.SetMode(gin.TestMode)
	controller := NewNotificationController(nil, nil, nil)
	controller.SetTaskScopeService(service.NewNotificationTaskScopeService(configs, cron))
	router := gin.New()
	router.GET("/api/notify/task-scope", controller.GetTaskScope)
	router.PUT("/api/notify/task-scope", controller.UpdateTaskScope)
	return router
}

func notificationScopeRequest(router http.Handler, method, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/notify/task-scope", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestNotificationTaskScopeControllerGetPut(t *testing.T) {
	configs := &notificationScopeControllerConfigs{}
	router := notificationScopeTestRouter(configs, &notificationScopeControllerCron{})
	response := notificationScopeRequest(router, http.MethodGet, "")
	var envelope struct {
		State bool                         `json:"state"`
		Data  domain.NotificationTaskScope `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || !envelope.State || len(envelope.Data.Tasks) != 2 || len(envelope.Data.Selected) != 2 || envelope.Data.Tasks[0].TaskKey != "key:a" || envelope.Data.Tasks[0].Status != "disabled" {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	for _, body := range []string{`{"selected":["key:a","handler:legacy","key:a"]}`, `{"selected":[]}`} {
		response = notificationScopeRequest(router, http.MethodPut, body)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"state":true`) {
			t.Fatalf("%d %s", response.Code, response.Body.String())
		}
	}
	response = notificationScopeRequest(router, http.MethodGet, "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"selected":[]`) || configs.values["notify_task_scope_configured"] != "true" || configs.values["notify_task_keys"] != "[]" || configs.writes != 2 {
		t.Fatalf("显式空选择重读丢失: %s %v", response.Body.String(), configs.values)
	}
}

func TestNotificationTaskScopeControllerBadRequests(t *testing.T) {
	configs := &notificationScopeControllerConfigs{}
	router := notificationScopeTestRouter(configs, &notificationScopeControllerCron{})
	for _, body := range []string{
		``, `{`, `null`, `[]`, `{}`, `{"selected":null}`, `{"selected":{}}`, `{"selected":"key:a"}`,
		`{"selected":[null]}`, `{"selected":[1]}`, `{"selected":[true]}`, `{"selected":[[]]}`, `{"selected":[{}]}`,
		`{"selected":[""]}`, `{"selected":["8"]}`, `{"selected":["deleted"]}`, `{"selected":["handler:shared"]}`,
		`{"selected":[]} {}`, `{"selected":["` + strings.Repeat("x", 1<<20) + `"]}`,
	} {
		response := notificationScopeRequest(router, http.MethodPut, body)
		if response.Code != 400 {
			t.Fatalf("body length=%d status=%d response=%s", len(body), response.Code, response.Body.String())
		}
	}
	if configs.writes != 0 {
		t.Fatal("非法请求不得写配置")
	}
}

func TestNotificationTaskScopeControllerInfrastructureErrors(t *testing.T) {
	for _, branch := range []string{"get config", "get cron", "get malformed", "put cron", "put write"} {
		t.Run(branch, func(t *testing.T) {
			configs := &notificationScopeControllerConfigs{}
			cron := &notificationScopeControllerCron{}
			method := http.MethodGet
			switch branch {
			case "get config":
				configs.readErr = errors.New("config unavailable")
			case "get cron":
				cron.err = errors.New("cron unavailable")
			case "get malformed":
				configs.values = map[string]string{"notify_task_keys": "{"}
			case "put cron":
				method = http.MethodPut
				cron.err = errors.New("cron unavailable")
			case "put write":
				method = http.MethodPut
				configs.writeErr = errors.New("write unavailable")
			}
			response := notificationScopeRequest(notificationScopeTestRouter(configs, cron), method, `{"selected":[]}`)
			if response.Code != 500 {
				t.Fatalf("%d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestNotificationTaskScopeControllerCanceledRequest(t *testing.T) {
	configs := &notificationScopeControllerConfigs{}
	router := notificationScopeTestRouter(configs, &notificationScopeControllerCron{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPut, "/api/notify/task-scope", strings.NewReader(`{"selected":[]}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 500 || configs.writes != 0 {
		t.Fatalf("取消请求不能写配置: %d %s", response.Code, response.Body.String())
	}
}
