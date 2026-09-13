package controller

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
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

type countingPlaybackStore struct {
	*dao.PlaybackRecordDAO
	writes int
}

func (s *countingPlaybackStore) Save(ctx context.Context, r domain.PlaybackRecord) error {
	s.writes++
	return s.PlaybackRecordDAO.Save(ctx, r)
}

// TestDirectLinkRepeatedRequestsPersistOneRecord 覆盖真实控制器到 Service、DAO 的重复解析链路。
func TestDirectLinkRepeatedRequestsPersistOneRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := dao.DB
	dao.DB = db
	defer func() { dao.DB = previous }()
	mock.ExpectQuery("SELECT f.file_name").WithArgs(3, "same-file").
		WillReturnRows(sqlmock.NewRows([]string{"file_name"}).AddRow("movie.mkv"))
	mock.ExpectQuery("SELECT i.title").
		WillReturnRows(sqlmock.NewRows([]string{"title", "poster_path", "tmdb_id", "media_type", "season_number", "episode_number"}).AddRow("龙珠", "", nil, "tv", 2, 3))
	mock.ExpectExec("INSERT INTO t_strm_playback_record").
		WithArgs(sqlmock.AnyArg(), "龙珠 · 第 2 季 · 第 3 集", "", "https://cdn.test/video-1", sqlmock.AnyArg(), "172.17.0.3", "未知", "HEAD").
		WillReturnResult(sqlmock.NewResult(1, 1))
	c := NewDirectLinkController()
	store := &countingPlaybackStore{PlaybackRecordDAO: dao.NewPlaybackRecordDAO(nil)}
	c.SetRecordPlayback(service.NewPlaybackRecordService(store).Record)
	c.SetGetCloud115ByID(func(id int) (*Cloud115AccountBrief, error) { return &Cloud115AccountBrief{ID: id}, nil })
	resolved := 0
	c.SetGetFileDirectLink(func(int, string, int, string, string) (interface{}, error) {
		resolved++
		return fmt.Sprintf("https://cdn.test/video-%d", resolved), nil
	})
	router := gin.New()
	router.GET("/direct-link", c.GetDirectLink)
	router.HEAD("/direct-link", c.GetDirectLink)
	for i := 0; i < 12; i++ {
		method := http.MethodGet
		if i == 0 {
			method = http.MethodHead
		}
		req := httptest.NewRequest(method, "/direct-link?path=/movie.mkv&pickcode=same-file&cloud115_id=3", nil)
		req.RemoteAddr = "172.17.0.3:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusFound || w.Header().Get("Location") != fmt.Sprintf("https://cdn.test/video-%d", i+1) {
			t.Fatalf("第 %d 次解析未正常返回新直链", i+1)
		}
	}
	if store.writes != 1 {
		t.Fatalf("12 次解析写入了 %d 条记录，期望 1", store.writes)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type playbackTestError struct{}

func (*playbackTestError) Error() string { return "test failure" }
