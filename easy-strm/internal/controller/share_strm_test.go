package controller

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type strmActionsStub struct {
	err   error
	ua    string
	query domain.ShareLibraryQuery
}

func (s *strmActionsStub) Settings() (domain.ShareStrmSettings, error) {
	return domain.ShareStrmSettings{Cloud115ID: 7}, s.err
}
func (s *strmActionsStub) SaveSettings(domain.ShareStrmSettings) error { return s.err }
func (s *strmActionsStub) StartExport(q domain.ShareLibraryQuery) (string, error) {
	s.query = q
	return "task-1", s.err
}
func (s *strmActionsStub) Playback(_ context.Context, _ string, ua string) (string, error) {
	s.ua = ua
	return "https://cdn.example/video", s.err
}

func TestShareStrmController(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name, method, route, body string
		err                       error
		status                    int
	}{
		{"settings", "GET", "/settings", "", nil, 200},
		{"settings failure", "GET", "/settings", "", errors.New("数据库失败"), 500},
		{"save", "PUT", "/settings", `{}`, nil, 200},
		{"bad json", "PUT", "/settings", `{`, nil, 400},
		{"bad config", "PUT", "/settings", `{}`, errors.New("配置错误"), 400},
		{"export", "POST", "/export?media_type=tv&genres=18", "", nil, 200},
		{"bad filter", "POST", "/export?media_type=music", "", nil, 400},
		{"start failure", "POST", "/export", "", errors.New("任务失败"), 400},
		{"play", "GET", "/play/56c94081-7619-4824-9e6f-ea13141599e1", "", nil, 302},
		{"head", "HEAD", "/play/56c94081-7619-4824-9e6f-ea13141599e1", "", nil, 302},
		{"bad id", "GET", "/play/bad", "", nil, 400},
		{"missing", "GET", "/play/56c94081-7619-4824-9e6f-ea13141599e1", "", sql.ErrNoRows, 404},
		{"transfer failure", "GET", "/play/56c94081-7619-4824-9e6f-ea13141599e1", "", errors.New("转存失败"), 502},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stub := &strmActionsStub{err: tt.err}
			c := &ShareStrmController{s: stub}
			var recordedID, recordedURL, recordedMethod string
			c.SetRecordPlayback(func(id, directURL, _, method string) {
				recordedID, recordedURL, recordedMethod = id, directURL, method
			})
			r := gin.New()
			r.GET("/settings", c.Settings)
			r.PUT("/settings", c.SaveSettings)
			r.POST("/export", c.Export)
			r.GET("/play/:id", c.Playback)
			r.HEAD("/play/:id", c.Playback)
			req := httptest.NewRequest(tt.method, tt.route, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Player/1")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if tt.status == http.StatusFound {
				if w.Header().Get("Location") != "https://cdn.example/video" || stub.ua != "Player/1" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("重定向契约错误")
				}
				if recordedID != "56c94081-7619-4824-9e6f-ea13141599e1" || recordedURL != "https://cdn.example/video" || recordedMethod != tt.method {
					t.Fatal("成功重定向未记录分享STRM播放")
				}
			} else if w.Header().Get("Location") != "" {
				t.Fatal("失败不能重定向")
			} else if recordedID != "" {
				t.Fatal("失败请求不能写入播放记录")
			}
			if tt.name == "export" && (stub.query.MediaType != "tv" || stub.query.Genres != "18") {
				t.Fatal("筛选条件丢失")
			}
		})
	}
}
