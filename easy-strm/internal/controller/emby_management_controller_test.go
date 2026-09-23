package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"github.com/DATA-DOG/go-sqlmock"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func TestEmbyUserLibrariesController(t *testing.T) {
	for _, tc := range []struct {
		name, id             string
		remoteStatus, status int
	}{
		{"参数错误", "bad", 200, 400},
		{"返回权限名称和Guid", "1", 200, 200},
		{"上游失败", "1", 500, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/emby/Library/SelectableMediaFolders" {
					t.Errorf("路径错误: %s", r.URL.Path)
				}
				w.WriteHeader(tc.remoteStatus)
				w.Write([]byte(`[{"Name":"电影","Id":"127953","Guid":"eef44ceeaf834d4293445eef5872ba74","IsUserAccessConfigurable":true}]`))
			}))
			defer remote.Close()
			if tc.id == "1" {
				mock.ExpectQuery("SELECT id, name, base_url").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).AddRow(1, "测试", remote.URL, "test-key", true, true, time.Now(), time.Now()))
			}
			manager := service.NewEmbyManagementService(dao.NewEmbyServerDAO(db), nil, nil, remote.Client())
			router := gin.New()
			router.GET("/emby/servers/:server_id/user-libraries", NewEmbyManagementController(manager).ListUserLibraries)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/emby/servers/"+tc.id+"/user-libraries", nil))
			if response.Code != tc.status {
				t.Fatalf("状态码=%d body=%s", response.Code, response.Body.String())
			}
			if tc.status == 200 {
				var body struct {
					Data struct {
						Data []struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						}
						Total int
					}
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Data.Total != 1 || len(body.Data.Data) != 1 || body.Data.Data[0].ID != "eef44ceeaf834d4293445eef5872ba74" || body.Data.Data[0].Name != "电影" {
					t.Fatalf("错误响应: %s", response.Body.String())
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmbyManagementControllerRejectsInvalidServerID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewEmbyManagementController(nil)
	router.GET("/emby/servers/:server_id/users", controller.ListUsers)

	request := httptest.NewRequest(http.MethodGet, "/emby/servers/not-a-number/users", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 %d", response.Code, http.StatusBadRequest)
	}
}

func TestRunPluginTaskRequiresAutoConfigureConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewEmbyManagementController(nil)
	router.POST("/emby/servers/:server_id/plugin/strm-assistant/tasks", controller.RunPluginTask)

	request := httptest.NewRequest(http.MethodPost, "/emby/servers/1/plugin/strm-assistant/tasks", bytes.NewBufferString(`{"action":"strm_scan_capture","library_id":"lib-1"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 %d", response.Code, http.StatusBadRequest)
	}
}

func TestEmbyScheduledTasksController(t *testing.T) {
	for _, tc := range []struct {
		name, id             string
		remoteStatus, status int
	}{
		{"参数错误", "bad", 200, 400},
		{"返回定时任务", "1", 200, 200},
		{"上游失败", "1", 500, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/emby/ScheduledTasks" {
					t.Errorf("路径错误: %s", r.URL.Path)
				}
				w.WriteHeader(tc.remoteStatus)
				w.Write([]byte(`[{"Name":"扫描","Id":"task1","State":"Idle"}]`))
			}))
			defer remote.Close()
			if tc.id == "1" {
				mock.ExpectQuery("SELECT id, name, base_url").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).AddRow(1, "测试", remote.URL, "test-key", true, true, time.Now(), time.Now()))
			}
			manager := service.NewEmbyManagementService(dao.NewEmbyServerDAO(db), nil, nil, remote.Client())
			router := gin.New()
			router.GET("/emby/servers/:server_id/scheduled-tasks", NewEmbyManagementController(manager).ListScheduledTasks)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/emby/servers/"+tc.id+"/scheduled-tasks", nil))
			if response.Code != tc.status {
				t.Fatalf("状态码=%d body=%s", response.Code, response.Body.String())
			}
			if tc.status == 200 {
				var body struct {
					Data struct {
						Data []struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						}
						Total int
					}
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Data.Total != 1 || len(body.Data.Data) != 1 || body.Data.Data[0].ID != "task1" || body.Data.Data[0].Name != "扫描" {
					t.Fatalf("错误响应: %s", response.Body.String())
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStartEmbyScheduledTaskController(t *testing.T) {
	for _, tc := range []struct {
		name, id, state      string
		remoteStatus, status int
	}{
		{"参数错误", "bad", "Idle", 204, 400},
		{"提交成功", "1", "Idle", 204, 200},
		{"运行中拒绝重复", "1", "Running", 204, 400},
		{"上游触发失败", "1", "Idle", 500, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mini := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
			defer client.Close()
			tasks := service.NewTaskService(dao.NewTaskRedisDAO(client))
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					json.NewEncoder(w).Encode([]map[string]string{{"Id": "task1", "Name": "扫描", "State": tc.state}})
					return
				}
				w.WriteHeader(tc.remoteStatus)
			}))
			defer remote.Close()
			if tc.id == "1" {
				mock.ExpectQuery("SELECT id, name, base_url").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).AddRow(1, "测试", remote.URL, "key", true, true, time.Now(), time.Now()))
			}
			manager := service.NewEmbyManagementService(dao.NewEmbyServerDAO(db), tasks, nil, remote.Client())
			router := gin.New()
			router.POST("/servers/:server_id/scheduled-tasks/:task_id/run", NewEmbyManagementController(manager).StartScheduledTask)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/servers/"+tc.id+"/scheduled-tasks/task1/run", nil))
			if response.Code != tc.status {
				t.Fatalf("状态码 %d: %s", response.Code, response.Body.String())
			}
			if tc.status == 200 {
				var body struct {
					Data struct {
						TaskID string `json:"task_id"`
					}
				}
				json.Unmarshal(response.Body.Bytes(), &body)
				if body.Data.TaskID == "" {
					t.Fatal("未返回任务 ID")
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
