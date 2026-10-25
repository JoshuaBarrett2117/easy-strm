package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDirectLinkHeadRedirectsTo115CDN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		requestUserAgent = "Emby/4.9"
		redirectURL      = "https://cdn.example.test/movie.mkv?token=temporary"
	)

	directLinkController := NewDirectLinkController()
	directLinkController.SetGetCloud115ByID(func(id int) (*Cloud115AccountBrief, error) {
		if id != 3 {
			t.Fatalf("账号 ID 不符合预期: got %d want 3", id)
		}
		return &Cloud115AccountBrief{ID: id, Cookie: "test-cookie"}, nil
	})
	directLinkController.SetGetPickCodeByPath(func(path string, cloud115ID int, cookie string) (string, error) {
		if path != "/影视资源/电影.mkv" {
			t.Fatalf("路径解码结果不符合预期: %q", path)
		}
		return "test-pickcode", nil
	})
	directLinkController.SetGetFileDirectLink(func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error) {
		if userAgent != requestUserAgent {
			t.Fatalf("User-Agent 未透传: got %q want %q", userAgent, requestUserAgent)
		}
		return redirectURL, nil
	})
	directLinkController.SetRedisGet(func(key string) (string, error) {
		return "", errors.New("cache miss")
	})
	directLinkController.SetRedisSet(func(key string, value string, expirationSec int) error {
		return nil
	})

	router := gin.New()
	router.HEAD("/direct-link", directLinkController.GetDirectLink)

	request := httptest.NewRequest(
		http.MethodHead,
		"/direct-link?path=%2F%E5%BD%B1%E8%A7%86%E8%B5%84%E6%BA%90%2F%E7%94%B5%E5%BD%B1.mkv&cloud115_id=3",
		nil,
	)
	request.Header.Set("User-Agent", requestUserAgent)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusFound {
		t.Fatalf("HEAD 状态码不符合预期: got %d want %d", response.Code, http.StatusFound)
	}
	if location := response.Header().Get("Location"); location != redirectURL {
		t.Fatalf("HEAD Location 不符合预期: got %q want %q", location, redirectURL)
	}
}

func TestDirectLinkServerUAResolution(t *testing.T) {
	const (
		embyUA   = "Emby/4.9.1.90 (Linux; 4.9.1.90) EmbyServer/4.9.1.90"
		chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/155.0.0.0"
		ffmpegUA = "Lavf/59.27.100"
		diskUA   = "115disk/1.0"
	)

	t.Run("未配置服务端 UA 时保持原样", func(t *testing.T) {
		c := NewDirectLinkController()
		if got := c.resolveClientUA(embyUA); got != embyUA {
			t.Fatalf("got %q want %q", got, embyUA)
		}
	})

	t.Run("浏览器 UA 不受影响", func(t *testing.T) {
		c := NewDirectLinkController()
		c.SetGetServerUA(func() string { return ffmpegUA })
		if got := c.resolveClientUA(chromeUA); got != chromeUA {
			t.Fatalf("got %q want %q", got, chromeUA)
		}
	})

	t.Run("Emby 服务端取源改用 ffmpeg UA", func(t *testing.T) {
		c := NewDirectLinkController()
		c.SetGetServerUA(func() string { return ffmpegUA })
		if got := c.resolveClientUA(embyUA); got != ffmpegUA {
			t.Fatalf("got %q want %q", got, ffmpegUA)
		}
	})

	t.Run("配置为空字符串时保持原样", func(t *testing.T) {
		c := NewDirectLinkController()
		c.SetGetServerUA(func() string { return "   " })
		if got := c.resolveClientUA(embyUA); got != embyUA {
			t.Fatalf("got %q want %q", got, embyUA)
		}
	})

	t.Run("非 Emby 前缀不改写", func(t *testing.T) {
		c := NewDirectLinkController()
		c.SetGetServerUA(func() string { return ffmpegUA })
		if got := c.resolveClientUA(diskUA); got != diskUA {
			t.Fatalf("got %q want %q", got, diskUA)
		}
	})
}
