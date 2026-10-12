package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

func TestEmbyProxyFailureDetailReasonAndRedaction(t *testing.T) {
	const signedURL = "https://relay-user:relay-password@cdn.example/video?signature=relay-signature&custom=relay-query#relay-fragment"
	for _, test := range []struct {
		name, cause string
		err         error
	}{
		{"url", "connection refused", &url.Error{Op: "Get", URL: signedURL, Err: errors.New("connection refused")}},
		{"network", "connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}},
		{"wrapped_url", "DNS failed", fmt.Errorf("媒体请求失败: %w", &url.Error{Op: "Get", URL: signedURL, Err: errors.New("DNS failed")})},
		{"nested_url", "DNS failed", &url.Error{Op: "Get", URL: signedURL, Err: &url.Error{Op: "Get", URL: signedURL, Err: errors.New("DNS failed")}}},
		{"embedded_url", "connection refused", fmt.Errorf("connection refused for %s", signedURL)},
		{"joined", "DNS failed", errors.Join(&url.Error{Op: "Get", URL: signedURL, Err: errors.New("connection refused")}, fmt.Errorf("DNS failed for %s", signedURL))},
		{"headers", "connection refused", errors.New("connection refused\nCookie: session=relay-cookie\nAuthorization: Bearer relay-auth\nX-Emby-Token: relay-emby\nX-MediaBrowser-Token=relay-browser\nToken=relay-token")},
		{"credentials", "DNS failed", errors.New("DNS failed signature=relay-signature password=relay-password token=relay-token")},
		{"canceled", context.Canceled.Error(), context.Canceled},
		{"deadline", context.DeadlineExceeded.Error(), context.DeadlineExceeded},
		{"eof", io.ErrUnexpectedEOF.Error(), io.ErrUnexpectedEOF},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			failure := newEmbyProxyFailure("relay", "connection_failed", "媒体播放上游不可用", 0, test.err)
			request := httptest.NewRequest("GET", "/Videos/42/stream?MediaSourceId=s1", nil)
			logEmbyProxyFailure(&domain.EmbyServer{ID: 1}, request, failure)
			var record struct {
				Fields struct {
					Detail string `json:"detail"`
				} `json:"fields"`
			}
			if err := json.Unmarshal([]byte(output.String()), &record); err != nil {
				t.Fatalf("日志不是有效 JSON: %v", err)
			}
			for _, detail := range []string{failure.detail, record.Fields.Detail} {
				if !strings.Contains(detail, test.cause) {
					t.Errorf("detail 丢失底层原因 %q: %s", test.cause, detail)
				}
			}
			for _, secret := range []string{"relay-user", "relay-password", "relay-signature", "relay-query", "relay-fragment", "relay-cookie", "relay-auth", "relay-emby", "relay-browser", "relay-token", "https://"} {
				if strings.Contains(failure.detail, secret) || strings.Contains(output.String(), secret) {
					t.Errorf("detail 或日志泄露凭据 %q", secret)
				}
			}
		})
	}
	if detail := embyProxyErrorDetail(nil); detail != "" {
		t.Errorf("无错误时 detail=%q", detail)
	}
}
