package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"easy-strm/internal/domain"
	"github.com/google/uuid"
)

const embyProxyTestOrigin = "https://player.example"

func assertEmbyProxyCORS(t *testing.T, header http.Header, origin string) {
	t.Helper()
	if origin == "" {
		for key := range header {
			if strings.HasPrefix(strings.ToLower(key), "access-control-") {
				t.Errorf("无 Origin 响应不应新增 %s", key)
			}
		}
		return
	}
	if header.Get("Access-Control-Allow-Origin") != origin || header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("CORS 未回显 Origin/凭据许可: %v", header)
	}
	for _, name := range []string{"Content-Range", "Content-Length", "Accept-Ranges", "Content-Type"} {
		if !embyProxyHeaderContains(header, "Access-Control-Expose-Headers", name) {
			t.Errorf("未暴露 %s", name)
		}
	}
	if !embyProxyHeaderContains(header, "Vary", "Origin") {
		t.Errorf("缺少 Vary Origin: %v", header)
	}
}

func embyProxyHeaderContains(header http.Header, key, value string) bool {
	for _, line := range header.Values(key) {
		for _, part := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return true
			}
		}
	}
	return false
}

func TestEmbyProxyBrowserAllowsLocalRelayNativeRedirect(t *testing.T) {
	for _, tc := range []struct {
		name, method, origin string
		status, calls        int
	}{
		{"浏览器GET本地中继", "GET", embyProxyTestOrigin, 200, 1},
		{"浏览器HEAD本地中继", "HEAD", embyProxyTestOrigin, 200, 1},
		{"原生GET仍302", "GET", "", 302, 0},
		{"原生HEAD仍302", "HEAD", "", 302, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mediaCalls atomic.Int32
			media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mediaCalls.Add(1)
				assertEmbyProxyRelayRequest(t, r, tc.method)
				w.Header().Set("Content-Type", "video/mp4")
				if r.Method != http.MethodHead {
					_, _ = io.WriteString(w, "local-video")
				}
			}))
			defer media.Close()
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"MediaSources":[{"Id":"s1","Path":"`+media.URL+`/video?signature=test"}]}`)
			}))
			defer remote.Close()
			service := NewEmbyProxyService(nil, remote.Client())
			defer service.Close()
			request := embyProxyRelayTestRequest(tc.method, tc.origin)
			response := httptest.NewRecorder()
			service.buildHandler(&domain.EmbyServer{BaseURL: remote.URL}).ServeHTTP(response, request)
			if response.Code != tc.status || mediaCalls.Load() != int32(tc.calls) {
				t.Errorf("status=%d 媒体访问=%d，期望 %d 且访问 %d 次", response.Code, mediaCalls.Load(), tc.status, tc.calls)
			}
			wantBody := ""
			if tc.origin != "" && tc.method == http.MethodGet {
				wantBody = "local-video"
			}
			if response.Body.String() != wantBody {
				t.Errorf("中继正文=%q，期望 %q", response.Body.String(), wantBody)
			}
			if tc.origin == "" && response.Header().Get("Location") != media.URL+"/video?signature=test" {
				t.Error("原生播放器重定向地址改变")
			}
			assertEmbyProxyCORS(t, response.Header(), tc.origin)
		})
	}
}

func TestEmbyProxyFailureCORS(t *testing.T) {
	for _, status := range []int{401, 403, 502, 503} {
		for _, method := range []string{"GET", "HEAD"} {
			for _, origin := range []string{embyProxyTestOrigin, ""} {
				t.Run(http.StatusText(status)+method+origin, func(t *testing.T) {
					request := httptest.NewRequest(method, "/Videos/42/stream", nil)
					request.Header.Set("Origin", origin)
					response := httptest.NewRecorder()
					response.Header().Add("Vary", "Accept-Encoding")
					response.Header().Add("Vary", "Accept-Language")
					failure := newEmbyProxyFailure("playback_info", "connection_failed", "上游不可用", 0, nil)
					failure.status = status
					writeEmbyProxyFailure(&domain.EmbyServer{}, response, request, failure)
					assertEmbyProxyCORS(t, response.Header(), origin)
					if response.Code != status || !embyProxyHeaderContains(response.Header(), "Vary", "Accept-Encoding") || !embyProxyHeaderContains(response.Header(), "Vary", "Accept-Language") {
						t.Errorf("失败状态或原 Vary 被改动: %d %v", response.Code, response.Header())
					}
					if method == "HEAD" && response.Body.Len() != 0 {
						t.Error("HEAD 不应写入错误正文")
					}
				})
			}
		}
	}
}

func TestProxyCORSVaryPreservation(t *testing.T) {
	for _, existing := range []string{"", "Accept-Encoding", "Accept-Encoding, origin", "*"} {
		t.Run(existing, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/video", nil)
			request.Header.Set("Origin", embyProxyTestOrigin)
			header := make(http.Header)
			if existing != "" {
				header.Add("Vary", existing)
			}
			applyProxyCORS(header, request)
			applyProxyCORS(header, request)
			assertEmbyProxyCORS(t, header, embyProxyTestOrigin)
			originCount := 0
			for _, line := range header.Values("Vary") {
				for _, value := range strings.Split(line, ",") {
					if strings.EqualFold(strings.TrimSpace(value), "Origin") {
						originCount++
					}
				}
			}
			if originCount != 1 || (existing != "" && header.Values("Vary")[0] != existing) {
				t.Errorf("Vary 应保留原值且 Origin 只出现一次: %v", header)
			}
		})
	}
}

func TestEmbyProxyPlaybackFailuresKeepPolicyAndCORS(t *testing.T) {
	for _, tc := range []struct {
		name, body, query, stage, reason string
		upstreamStatus, status           int
	}{
		{"未授权", "", "", "playback_info", "authorization_failed", 401, 401},
		{"拒绝授权", "", "", "playback_info", "authorization_failed", 403, 403},
		{"上游失败", "", "", "playback_info", "upstream_http_error", 503, 502},
		{"解析失败", "{bad", "", "playback_info", "invalid_json", 200, 502},
		{"缺少媒体源", `{"MediaSources":[]}`, "", "media_source", "media_source_missing", 200, 502},
		{"STRM转码仍禁止", `{"MediaSources":[{"Path":"https://8.8.8.8/video"}]}`, "?Static=false", "playback_policy", "strm_fallback_blocked", 200, 502},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			t.Run(tc.name+method, func(t *testing.T) {
				output := captureEmbyProxyLog(t)
				calls := 0
				transport := embyProxyRelayTestTransport(func(request *http.Request) (*http.Response, error) {
					calls++
					if !strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
						t.Error("失败后不可视频回源")
					}
					return &http.Response{StatusCode: tc.upstreamStatus, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: request}, nil
				})
				service := NewEmbyProxyService(nil, &http.Client{Transport: transport})
				defer service.Close()
				request := httptest.NewRequest(method, "/Videos/42/stream"+tc.query, nil)
				request.Header.Set("Origin", embyProxyTestOrigin)
				response := httptest.NewRecorder()
				service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(response, request)
				assertEmbyProxyCORS(t, response.Header(), embyProxyTestOrigin)
				if response.Code != tc.status || calls != 1 {
					t.Fatalf("既有失败策略改变: status=%d calls=%d", response.Code, calls)
				}
				for _, field := range []string{`"stage":"` + tc.stage + `"`, `"reason":"` + tc.reason + `"`, `"fallback_blocked":true`} {
					if !strings.Contains(output.String(), field) {
						t.Errorf("既有失败日志缺少 %s", field)
					}
				}
			})
		}
	}
}

func TestEmbyProxyOPTIONSLocalCORS(t *testing.T) {
	for _, path := range []string{"/Videos/42/stream", "/emby/Items", "/login"} {
		for _, origin := range []string{embyProxyTestOrigin, ""} {
			t.Run(path+origin, func(t *testing.T) {
				var upstreamCalls atomic.Int32
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					upstreamCalls.Add(1)
					w.WriteHeader(401)
				}))
				defer remote.Close()
				service := NewEmbyProxyService(nil, remote.Client())
				defer service.Close()
				request := httptest.NewRequest("OPTIONS", path, nil)
				request.Header.Set("Origin", origin)
				response := httptest.NewRecorder()
				service.buildHandler(&domain.EmbyServer{BaseURL: remote.URL}).ServeHTTP(response, request)
				if response.Code != 204 || response.Body.Len() != 0 || upstreamCalls.Load() != 0 {
					t.Errorf("预检必须本地204: status=%d body=%q upstream=%d", response.Code, response.Body.String(), upstreamCalls.Load())
				}
				assertEmbyProxyCORS(t, response.Header(), origin)
				if origin != "" {
					for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
						if !embyProxyHeaderContains(response.Header(), "Access-Control-Allow-Methods", method) {
							t.Errorf("预检不允许 %s", method)
						}
					}
					for _, header := range []string{"Range", "If-Range", "Accept", "Content-Type", "Authorization", "X-Emby-Token", "X-MediaBrowser-Token", "X-Emby-Authorization", "Origin"} {
						if !embyProxyHeaderContains(response.Header(), "Access-Control-Allow-Headers", header) {
							t.Errorf("预检不允许 %s", header)
						}
					}
				}
			})
		}
	}
}

type embyProxyRandomFailure struct{}

func (embyProxyRandomFailure) Read([]byte) (int, error) {
	return 0, errors.New("测试随机源故障")
}

func TestEmbyProxyRequestIDFailureCORS(t *testing.T) {
	uuid.SetRand(embyProxyRandomFailure{})
	defer uuid.SetRand(nil)
	for _, origin := range []string{embyProxyTestOrigin, ""} {
		t.Run(origin, func(t *testing.T) {
			service := NewEmbyProxyService(nil, nil)
			defer service.Close()
			request := httptest.NewRequest("GET", "/Videos/42/stream", nil)
			request.Header.Set("Origin", origin)
			response := httptest.NewRecorder()
			service.buildHandler(&domain.EmbyServer{BaseURL: "http://127.0.0.1:1"}).ServeHTTP(response, request)
			if response.Code != 503 {
				t.Fatalf("随机源故障应503，实际=%d", response.Code)
			}
			assertEmbyProxyCORS(t, response.Header(), origin)
		})
	}
}
