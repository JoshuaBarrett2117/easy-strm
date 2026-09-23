package controller

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestConfigSecretController(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old })
	c := NewConfigSecretController(service.NewConfigSecretService(dao.NewSystemConfigDAO(), nil, nil, nil, nil))
	r := gin.New()
	r.GET("/settings/secrets/:key", c.Get)
	r.GET("/emby/secrets/:key", c.GetEmby)
	for _, path := range []string{"/settings/secrets/jwt_secret", "/settings/secrets/emby_cover_ai_api_key", "/settings/secrets/emby_server_api_key", "/emby/secrets/global_api_key", "/emby/secrets/emby_server_api_key", "/emby/secrets/emby_server_api_key?server_id=-1", "/emby/secrets/emby_server_api_key?server_id=abc"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s 应拒绝: %d", path, w.Code)
		}
	}
	for _, key := range []string{"global_api_key", "emby_cover_ai_api_key"} {
		prefix := "/settings/secrets/"
		if strings.HasPrefix(key, "emby_") {
			prefix = "/emby/secrets/"
		}
		mock.ExpectQuery("SELECT id, config_key").WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).AddRow(1, key, "full-secret", time.Now(), time.Now()))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", prefix+key, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"value":"full-secret"`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("明文响应错误: %d %s", w.Code, w.Body.String())
		}
	}
	mock.ExpectQuery("SELECT id, config_key").WithArgs("global_api_key").WillReturnError(errors.New("sensitive storage detail"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/settings/secrets/global_api_key", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "sensitive storage detail") {
		t.Fatalf("错误响应错误: %d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
