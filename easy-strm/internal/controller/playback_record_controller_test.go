package controller

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlaybackRecordEndpoint(t *testing.T) {
	t.Skip("需要 PostgreSQL 迁移表的集成环境")
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	controller := NewPlaybackRecordController(service.NewPlaybackRecordService(dao.NewPlaybackRecordDAO(client)))
	router := gin.New()
	router.GET("/playback-records", controller.List)
	for _, query := range []string{"?limit=0", "?offset=-1", "?limit=101", "?offset=no"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/playback-records"+query, nil))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/playback-records", nil))
	var body map[string]interface{}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatal(w.Body.String())
	}
	server.Close()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/playback-records", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestDirectLinkPlaybackRecording(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		for _, success := range []bool{true, false} {
			c := NewDirectLinkController()
			c.SetGetCloud115ByID(func(id int) (*Cloud115AccountBrief, error) { return &Cloud115AccountBrief{ID: id}, nil })
			c.SetGetFileDirectLink(func(int, string, int, string, string) (interface{}, error) {
				if !success {
					return nil, fmtErrorForPlayback()
				}
				return "https://cdn.test/movie", nil
			})
			called := false
			c.SetRecordPlayback(func(p, code string, account int, link, ip, m string) {
				called = true
				if p != "/movie.mkv" || code != "abc" || account != 3 || link != "https://cdn.test/movie" || m != method || ip != "127.0.0.1" {
					t.Fatalf("调用数据错误 %s %s %d %s %s %s", p, code, account, link, ip, m)
				}
			})
			router := gin.New()
			router.Handle(method, "/direct-link", c.GetDirectLink)
			req := httptest.NewRequest(method, "/direct-link?path=/movie.mkv&pickcode=abc&cloud115_id=3", nil)
			req.RemoteAddr = "127.0.0.1:1234"
			router.ServeHTTP(httptest.NewRecorder(), req)
			if called != success {
				t.Fatalf("记录状态不符: %s %v", method, success)
			}
		}
	}
}

func fmtErrorForPlayback() error { return &playbackTestError{} }

type playbackTestError struct{}

func (*playbackTestError) Error() string { return "test failure" }
