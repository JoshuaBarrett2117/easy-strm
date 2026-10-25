package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 浏览器播放：115 CDN 不返回 CORS 头，302 过去浏览器只会拿到 CORS error，
// 所以浏览器请求必须由服务端中继回传；原生客户端仍走 302。

func newRelayTestController(t *testing.T, directURL string) *DirectLinkController {
	t.Helper()
	c := NewDirectLinkController()
	c.SetGetCloud115ByID(func(id int) (*Cloud115AccountBrief, error) {
		return &Cloud115AccountBrief{ID: id, Cookie: "test-cookie"}, nil
	})
	c.SetGetPickCodeByPath(func(path string, cloud115ID int, cookie string) (string, error) {
		return "test-pickcode", nil
	})
	c.SetGetFileDirectLink(func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error) {
		return directURL, nil
	})
	c.SetRedisGet(func(key string) (string, error) { return "", errors.New("cache miss") })
	c.SetRedisSet(func(key string, value string, expirationSec int) error { return nil })
	return c
}

func TestIsBrowserPlayback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name    string
		ua      string
		headers map[string]string
		want    bool
	}{
		{
			name:    "Chrome 带 Sec-Fetch-Mode",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/141.0 Safari/537.36",
			headers: map[string]string{"Sec-Fetch-Mode": "cors"},
			want:    true,
		},
		{
			name:    "Chrome 带 Origin",
			ua:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
			headers: map[string]string{"Origin": "https://emby.example"},
			want:    true,
		},
		{
			name: "Emby 官方客户端",
			ua:   "Emby/2.0.0 (Android 14)",
			want: false,
		},
		{
			name: "网易爆米花",
			ua:   "popcorn-time/1.0 (iPhone; iOS 18.0)",
			want: false,
		},
		{
			name: "Mozilla UA 但无浏览器特征头",
			ua:   "Mozilla/5.0 (compatible; SomeAgent/1.0)",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			var got bool
			router.GET("/probe", func(ctx *gin.Context) {
				got = IsBrowserPlayback(ctx)
				ctx.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			req.Header.Set("User-Agent", tc.ua)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			router.ServeHTTP(httptest.NewRecorder(), req)
			if got != tc.want {
				t.Fatalf("IsBrowserPlayback=%v，期望 %v（UA=%q）", got, tc.want, tc.ua)
			}
		})
	}
}

func TestDirectLinkPreflightEchoesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.OPTIONS("/direct-link", HandleDirectLinkPreflight)

	req := httptest.NewRequest(http.MethodOptions, "/direct-link", nil)
	req.Header.Set("Origin", "https://emby.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "range,x-emby-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("预检状态码: got %d want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://emby.example" {
		t.Fatalf("ACAO: got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
		t.Fatalf("Allow-Methods 未回显: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "range,x-emby-token" {
		t.Fatalf("Allow-Headers 未回显: %q", got)
	}
}

func TestBrowserRequestIsRelayedWithCORSAndRange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotRange, gotUA string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 0-4/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("hello"))
	}))
	defer cdn.Close()

	c := newRelayTestController(t, cdn.URL+"/movie.mp4")
	router := gin.New()
	router.GET("/direct-link", c.GetDirectLink)

	browserUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/141.0 Safari/537.36"
	req := httptest.NewRequest(http.MethodGet, "/direct-link?path=%2Fmovie.mp4&cloud115_id=2", nil)
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Origin", "https://emby.example")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Range", "bytes=0-4")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("中继状态码: got %d want 206，body=%s", rec.Code, rec.Body.String())
	}
	if gotRange != "bytes=0-4" {
		t.Fatalf("Range 未透传: got %q", gotRange)
	}
	if gotUA != browserUA {
		t.Fatalf("UA 未透传: got %q", gotUA)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://emby.example" {
		t.Fatalf("中继响应缺 ACAO: %q", got)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 0-4/100" {
		t.Fatalf("Content-Range 未透传: %q", got)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("中继内容不符: %q", rec.Body.String())
	}
}

func TestNativeClientStillRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const directURL = "https://cdn.example.test/movie.mp4?token=t"

	c := newRelayTestController(t, directURL)
	router := gin.New()
	router.GET("/direct-link", c.GetDirectLink)

	req := httptest.NewRequest(http.MethodGet, "/direct-link?path=%2Fmovie.mp4&cloud115_id=2", nil)
	req.Header.Set("User-Agent", "Emby/2.0.0 (Android 14)")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("原生客户端应仍 302: got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != directURL {
		t.Fatalf("Location: got %q", got)
	}
}

func TestBrowserRelayCanBeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetBrowserRelayEnabled(false)
	defer SetBrowserRelayEnabled(true)

	c := newRelayTestController(t, "https://cdn.example.test/movie.mp4")
	router := gin.New()
	router.GET("/direct-link", c.GetDirectLink)

	req := httptest.NewRequest(http.MethodGet, "/direct-link?path=%2Fmovie.mp4&cloud115_id=2", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/141.0")
	req.Header.Set("Origin", "https://emby.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("关闭中继后应回退 302: got %d", rec.Code)
	}
}
