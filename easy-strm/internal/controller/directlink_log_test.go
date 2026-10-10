package controller

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

func TestDirectLinkRequestLogsIdentifyCallerWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger.SetOutputs(&output, &output, &output, &output)
	logger.SetFormat("json")
	t.Cleanup(func() { logger.SetFormat("") })
	defer logger.SetOutputs(os.Stderr, os.Stderr, os.Stderr, os.Stderr)
	c := NewDirectLinkController()
	c.SetGetCloud115ByID(func(id int) (*Cloud115AccountBrief, error) {
		return &Cloud115AccountBrief{ID: id, Cookie: "private-cookie"}, nil
	})
	c.SetGetFileDirectLink(func(_ int, pick string, _ int, _, _ string) (interface{}, error) {
		if pick == "bad" {
			return nil, errors.New("upstream unavailable")
		}
		return "https://cdn.example/video?signature=private-signature", nil
	})
	r := gin.New()
	r.GET("/direct-link", c.GetDirectLink)
	r.HEAD("/direct-link", c.GetDirectLink)
	ids := map[string]bool{}
	for _, test := range []struct {
		method, pick string
		status       int
	}{{"HEAD", "good", 302}, {"GET", "good", 302}, {"GET", "bad", 500}} {
		output.Reset()
		req := httptest.NewRequest(test.method, "/direct-link?cloud115_id=2&pickcode="+test.pick+"&token=private-token", nil)
		req.RemoteAddr = "192.0.2.10:4567"
		req.Header.Set("User-Agent", "Emby/4.9")
		req.Header.Set("Range", "bytes=0-1023")
		req.Header.Set("Authorization", "Bearer private-auth")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		id := response.Header().Get("X-Request-ID")
		if id == "" || ids[id] || response.Code != test.status {
			t.Fatalf("请求关联或状态异常: id=%q status=%d", id, response.Code)
		}
		ids[id] = true
		logs := output.String()
		for _, want := range []string{`"event":"request"`, `"event":"resolve"`, `"event":"complete"`, `"request_id":"` + id + `"`, `"source":"strm_playback"`, `"method":"` + test.method + `"`, `"route":"/direct-link"`, `"duration_ms":`} {
			if !strings.Contains(logs, want) {
				t.Errorf("日志缺少 %q: %s", want, logs)
			}
		}
		for _, secret := range []string{"private-cookie", "private-auth", "private-token", "private-signature", "Emby/4.9", "bytes=0-1023", "192.0.2.10"} {
			if strings.Contains(logs, secret) {
				t.Errorf("日志泄露敏感字段 %q", secret)
			}
		}
	}
}
