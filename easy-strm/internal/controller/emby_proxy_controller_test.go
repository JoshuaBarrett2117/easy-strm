package controller

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func TestEmbyServerControllerRejectsInvalidProxyPort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewEmbyManagementController(nil)
	r := gin.New()
	r.POST("/emby/servers", c.CreateServer)
	r.PUT("/emby/servers/:server_id", c.UpdateServer)
	for _, value := range []string{"-1", "65536", "1.5", `"8097"`, "true"} {
		for _, method := range []string{"POST", "PUT"} {
			path := "/emby/servers"
			if method == "PUT" {
				path += "/1"
			}
			request := httptest.NewRequest(method, path, strings.NewReader(`{"proxy_port":`+value+`}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != 400 {
				t.Fatalf("%s port=%s code=%d", method, value, response.Code)
			}
		}
	}
}

func TestEmbyServerControllerSavesProxyPortAndStopsOnDisable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	tasks := service.NewTaskService(dao.NewTaskRedisDAO(client))
	servers := dao.NewEmbyServerDAO(db)
	proxy := service.NewEmbyProxyService(servers, nil, 80, 8082)
	defer proxy.Close()
	manager := service.NewEmbyManagementService(servers, tasks, nil, nil, proxy)
	c := NewEmbyManagementController(manager)
	r := gin.New()
	r.POST("/emby/servers", c.CreateServer)
	r.PUT("/emby/servers/:server_id", c.UpdateServer)
	portListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("emby")) }))
	defer remote.Close()
	rows := func(enabled bool) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time", "proxy_port"}).AddRow(1, "test", remote.URL, "key", enabled, true, time.Now(), time.Now(), port)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE t_emby_server").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO t_emby_server").WithArgs("test", remote.URL, "key", true, true, port).WillReturnRows(rows(true))
	mock.ExpectCommit()
	save := func(method string, enabled bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"name": "test", "base_url": remote.URL, "api_key": "key", "enabled": enabled, "is_default": true, "proxy_port": port})
		path := "/emby/servers"
		if method == "PUT" {
			path += "/1"
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("保存失败: %d %s", response.Code, response.Body.String())
		}
		var result struct {
			Data struct {
				Server struct {
					ProxyPort int `json:"proxy_port"`
				}
				TaskID string `json:"task_id"`
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Data.Server.ProxyPort != port || result.Data.TaskID == "" {
			t.Fatalf("字段未回显: %s", response.Body.String())
		}
	}
	save("POST", true)
	conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_emby_server SET is_default=false").WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("UPDATE t_emby_server SET name").WithArgs(1, "test", remote.URL, "key", false, true, port).WillReturnRows(rows(false))
	mock.ExpectCommit()
	save("PUT", false)
	l, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("停用未释放监听: %v", err)
	}
	_ = l.Close()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
