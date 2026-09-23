package controller

import (
	"bytes"
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdateScheduledTaskTriggersController(t *testing.T) {
	for _, tc := range []struct {
		name, id, body   string
		upstream, status int
	}{
		{"非法实例", "bad", `{"triggers":[]}`, 204, 400},
		{"缺少规则", "1", `{}`, 204, 400},
		{"类型错误", "1", `{"triggers":"bad"}`, 204, 400},
		{"非法时间", "1", `{"triggers":[{"Type":"DailyTrigger","TimeOfDayTicks":-1}]}`, 204, 400},
		{"删除全部", "1", `{"triggers":[]}`, 204, 200},
		{"上游失败", "1", `{"triggers":[]}`, 500, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Write([]byte(`[{"Id":"task1"}]`))
					return
				}
				w.WriteHeader(tc.upstream)
			}))
			defer remote.Close()
			if tc.status != 400 {
				mock.ExpectQuery("SELECT id, name, base_url").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).AddRow(1, "测试", remote.URL, "test-key", true, true, time.Now(), time.Now()))
			}
			manager := service.NewEmbyManagementService(dao.NewEmbyServerDAO(db), nil, nil, remote.Client())
			router := gin.New()
			router.PUT("/servers/:server_id/tasks/:task_id/triggers", NewEmbyManagementController(manager).UpdateScheduledTaskTriggers)
			req := httptest.NewRequest(http.MethodPut, "/servers/"+tc.id+"/tasks/task1/triggers", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("状态码 %d: %s", response.Code, response.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
