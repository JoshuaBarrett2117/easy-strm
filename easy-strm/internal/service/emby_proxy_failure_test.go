package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type embyProxyTestLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (l *embyProxyTestLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.Write(data)
}

func (l *embyProxyTestLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.String()
}

func captureEmbyProxyLog(t *testing.T) *embyProxyTestLog {
	t.Helper()
	output := &embyProxyTestLog{}
	logger.SetOutputs(output, output, output, output)
	t.Cleanup(func() { logger.SetOutputs(os.Stderr, os.Stderr, os.Stderr, os.Stderr) })
	return output
}

func TestEmbyProxySTRMDoesNotFallbackAfterRedirect(t *testing.T) {
	for _, path := range []string{
		"/Videos/42/stream?Static=false",
		"/emby/Videos/42/stream.mp4?VideoCodec=h264",
		"/Videos/42/stream.ts?TranscodeReasons=ContainerNotSupported",
		"/Videos/42/master.m3u8",
		"/Videos/42/main.m3u8",
		"/Videos/42/stream.m3u8",
		"/emby/Videos/42/hls/main/0.ts",
		"/Videos/42/hls1/main/0.ts",
	} {
		t.Run(path, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			var videoCalls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
					_, _ = io.WriteString(w, `{"MediaSources":[{"Id":"s1","Path":"https://cdn.example/video?signature=private-signature"}]}`)
					return
				}
				videoCalls.Add(1)
				w.WriteHeader(206)
				_, _ = io.WriteString(w, "video")
			}))
			defer remote.Close()
			s := NewEmbyProxyService(nil, remote.Client())
			h := s.buildHandler(&domain.EmbyServer{ID: 7, BaseURL: remote.URL, APIKey: "private-key", ProxyPort: 8097})
			first := httptest.NewRecorder()
			h.ServeHTTP(first, httptest.NewRequest("GET", "/Videos/42/stream?MediaSourceId=s1", nil))
			if first.Code != 302 {
				t.Fatalf("首次播放未返回302: %d", first.Code)
			}
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 502 || videoCalls.Load() != 0 {
				t.Fatalf("STRM 回退请求走了视频回源: code=%d calls=%d", response.Code, videoCalls.Load())
			}
			requestID := response.Header().Get("X-Request-ID")
			if requestID == "" || requestID == first.Header().Get("X-Request-ID") {
				t.Fatal("请求标识缺失或重复")
			}
			logs := output.String()
			for _, required := range []string{`"level":"ERROR"`, `"module":"emby_proxy"`, `"request_id":"` + requestID + `"`, `"server_id":7`, `"proxy_port":8097`, `"item_id":"42"`, `"media_source_id":"s1"`, `"reason":"strm_fallback_blocked"`, `"fallback_blocked":true`, "已禁止回源"} {
				if !strings.Contains(logs, required) {
					t.Errorf("失败日志缺少 %q: %s", required, logs)
				}
			}
			if strings.Contains(logs, "private-signature") || strings.Contains(logs, "private-key") {
				t.Fatal("失败日志泄露密钥")
			}
			// 回退失败不影响随后恢复有效的直接播放。
			last := httptest.NewRecorder()
			h.ServeHTTP(last, httptest.NewRequest("GET", "/Videos/42/stream", nil))
			if last.Code != 302 || videoCalls.Load() != 0 {
				t.Fatal("后续直接播放未恢复302")
			}
		})
	}
}

func TestEmbyProxyLocalPathFromSTRMNeverFallsBack(t *testing.T) {
	for _, path := range []string{"/Videos/42/stream", "/Videos/42/original", "/Videos/42/original.mkv", "/emby/Items/42/Download"} {
		t.Run(path, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			var videoCalls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/PlaybackInfo"):
					_, _ = io.WriteString(w, `{"MediaSources":[{"Id":"s1","Path":"/cache/42.mkv","Protocol":"File"}]}`)
				case strings.TrimPrefix(r.URL.Path, "/emby") == "/Items":
					if r.URL.Query().Get("Ids") != "42" {
						t.Errorf("未按项目查询: %s", r.URL.Path)
					}
					_, _ = io.WriteString(w, `{"Items":[{"Id":"42","Path":"/media/42.strm","IsShortcut":true}]}`)
				default:
					videoCalls.Add(1)
					w.WriteHeader(206)
				}
			}))
			defer remote.Close()
			h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{ID: 7, BaseURL: remote.URL, APIKey: "key", ProxyPort: 8097})
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if response.Code != 502 || videoCalls.Load() != 0 {
				t.Fatalf("STRM 被误当成本地文件回源: code=%d calls=%d", response.Code, videoCalls.Load())
			}
			if !strings.Contains(output.String(), `"fallback_blocked":true`) {
				t.Fatalf("没有禁止回源日志: %s", output.String())
			}
		})
	}
}

func TestEmbyProxyPlaybackFailureLogsWithoutSecrets(t *testing.T) {
	output := captureEmbyProxyLog(t)
	var videoCalls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, "private-upstream-body")
			return
		}
		videoCalls.Add(1)
	}))
	defer remote.Close()
	h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{ID: 7, BaseURL: remote.URL, APIKey: "private-api-key", ProxyPort: 8097})
	request := httptest.NewRequest("GET", "/emby/Videos/42/stream?MediaSourceId=s1&api_key=private-query-token", nil)
	request.RemoteAddr = "192.0.2.5:12345"
	request.Header.Set("Authorization", "Bearer private-auth")
	request.Header.Set("Cookie", "session=private-cookie")
	request.Header.Set("User-Agent", "EmbyPlayer/1.0")
	request.Header.Set("Range", "bytes=0-1023")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 502 || videoCalls.Load() != 0 {
		t.Fatal("上游失败不应回源")
	}
	logs := output.String()
	for _, required := range []string{`"level":"ERROR"`, `"request_id":"` + response.Header().Get("X-Request-ID") + `"`, `"server_id":7`, `"proxy_port":8097`, `"item_id":"42"`, `"media_source_id":"s1"`, `"method":"GET"`, `"stage":"playback_info"`, `"status":502`, `"upstream_status":503`, `"duration_ms":`, `"fallback_blocked":true`} {
		if !strings.Contains(logs, required) {
			t.Errorf("日志缺少 %q: %s", required, logs)
		}
	}
	if strings.Count(logs, `"level":"ERROR"`) != 1 {
		t.Errorf("应只记录一次主要失败: %s", logs)
	}
	for _, secret := range []string{"private-query-token", "private-auth", "private-cookie", "private-api-key", "private-upstream-body", "EmbyPlayer/1.0", "bytes=0-1023"} {
		if strings.Contains(logs, secret) || strings.Contains(response.Body.String(), secret) {
			t.Errorf("泄露敏感信息 %s", secret)
		}
	}
}

func TestEmbyProxyFailsClosedForUnknownOrInvalidMedia(t *testing.T) {
	for _, tc := range []struct {
		name, playbackBody, itemBody, stage, reason        string
		playbackStatus, itemStatus, status, upstreamStatus int
	}{
		{"直链无效", `{"MediaSources":[{"Id":"s1","Path":"https:///invalid"}]}`, "", "redirect", "invalid_strm_location", 200, 200, 502, 200},
		{"STRM容器无直链", `{"MediaSources":[{"Id":"s1","Path":"/media/video.mkv","Container":"strm"}]}`, "", "redirect", "invalid_strm_location", 200, 200, 502, 200},
		{"STRM路径无直链", `{"MediaSources":[{"Id":"s1","Path":"C:\\media\\video.STRM"}]}`, "", "redirect", "invalid_strm_location", 200, 200, 502, 200},
		{"解析失败", `{"MediaSources":[private-secret]}`, "", "playback_info", "invalid_json", 200, 200, 502, 200},
		{"类型错误", `{"MediaSources":"private-secret"}`, "", "playback_info", "invalid_json", 200, 200, 502, 200},
		{"缺少媒体源", `{"MediaSources":[]}`, "", "media_source", "media_source_missing", 200, 200, 502, 200},
		{"媒体源不匹配", `{"MediaSources":[{"Id":"other","Path":"https://cdn.example/video"}]}`, "", "media_source", "media_source_missing", 200, 200, 502, 200},
		{"播放信息401", "private-secret", "", "playback_info", "authorization_failed", 401, 200, 401, 401},
		{"播放信息403", "private-secret", "", "playback_info", "authorization_failed", 403, 200, 403, 403},
		{"播放信息重定向", "private-secret", "", "playback_info", "upstream_http_error", 302, 200, 502, 302},
		{"项目查询失败", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, "private-secret", "item_info", "upstream_http_error", 200, 503, 502, 503},
		{"项目查询401", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, "private-secret", "item_info", "authorization_failed", 200, 401, 401, 401},
		{"项目查询403", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, "private-secret", "item_info", "authorization_failed", 200, 403, 403, 403},
		{"项目解析失败", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, "private-secret", "item_info", "invalid_json", 200, 200, 502, 200},
		{"项目缺少路径", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, `{"Items":[{"Id":"42"}]}`, "media_kind", "media_kind_unknown", 200, 200, 502, 200},
		{"项目不匹配", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, `{"Items":[{"Id":"other","Path":"/local/file.mkv"}]}`, "media_kind", "media_kind_unknown", 200, 200, 502, 200},
		{"原项目媒体源不匹配", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, `{"Items":[{"Id":"42","Path":"/local/file.mkv","MediaSources":[{"Id":"other","Path":"/local/file.mkv"}]}]}`, "media_kind", "media_kind_unknown", 200, 200, 502, 200},
		{"Shortcut标记", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, `{"Items":[{"Id":"42","Path":"/cache/video.mkv","IsShortcut":true}]}`, "redirect", "invalid_strm_location", 200, 200, 502, 200},
		{"原项目媒体源为STRM", `{"MediaSources":[{"Id":"s1","Path":"/cache/video.mkv"}]}`, `{"Items":[{"Id":"42","Path":"/cache/video.mkv","MediaSources":[{"Id":"s1","Protocol":"Http","Path":"https://cdn.example/file?secret=private-secret"}]}]}`, "redirect", "invalid_strm_location", 200, 200, 502, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			var videoCalls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/PlaybackInfo"):
					w.WriteHeader(tc.playbackStatus)
					_, _ = io.WriteString(w, tc.playbackBody)
				case r.URL.Path == "/Items":
					w.WriteHeader(tc.itemStatus)
					_, _ = io.WriteString(w, tc.itemBody)
				default:
					videoCalls.Add(1)
					w.WriteHeader(206)
				}
			}))
			defer remote.Close()
			h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{ID: 7, BaseURL: remote.URL, APIKey: "private-secret", ProxyPort: 8097})
			for _, method := range []string{"GET", "HEAD"} {
				response := httptest.NewRecorder()
				h.ServeHTTP(response, httptest.NewRequest(method, "/Videos/42/stream?MediaSourceId=s1", nil))
				if response.Code != tc.status || videoCalls.Load() != 0 {
					t.Fatalf("%s code=%d video_calls=%d", method, response.Code, videoCalls.Load())
				}
				if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Location") != "" {
					t.Fatal("失败响应不应缓存或重定向")
				}
				if method == "HEAD" && response.Body.Len() != 0 {
					t.Fatal("HEAD 失败响应不应传输响应体")
				}
			}
			logs := output.String()
			for _, want := range []string{`"stage":"` + tc.stage + `"`, `"reason":"` + tc.reason + `"`, `"upstream_status":` + fmt.Sprint(tc.upstreamStatus), `"fallback_blocked":true`} {
				if !strings.Contains(logs, want) {
					t.Errorf("日志缺少 %q: %s", want, logs)
				}
			}
			if strings.Count(logs, `"level":"ERROR"`) != 2 || strings.Contains(logs, "private-secret") {
				t.Fatalf("失败日志次数或脱敏错误: %s", logs)
			}
		})
	}
}

func TestEmbyProxyLocalPlaybackModesRemainTransparent(t *testing.T) {
	for _, path := range []string{"/Videos/42/stream.webm", "/Videos/42/stream?Static=false", "/Videos/42/stream?VideoCodec=h264", "/emby/Videos/42/main.m3u8", "/Videos/42/hls1/main/1.ts", "/Videos/42/original.mkv", "/emby/Items/42/Download"} {
		t.Run(path, func(t *testing.T) {
			var metadataCalls, videoCalls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") || strings.TrimPrefix(r.URL.Path, "/emby") == "/Items" {
					metadataCalls.Add(1)
					if r.Header.Get("X-Emby-Token") != "user-token" || r.Header.Get("Cookie") != "session=user" || r.Header.Get("Range") != "" || r.URL.Query().Get("api_key") != "query-token" || r.URL.Query().Get("UserId") != "u1" || r.Header.Get("X-Request-ID") == "" {
						t.Error("媒体识别未沿用身份或错误透传Range")
					}
					if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
						_, _ = io.WriteString(w, `{"MediaSources":[{"Id":"s1","Path":"/media/video.mkv","Protocol":"File"}]}`)
					} else {
						_, _ = io.WriteString(w, `{"Items":[{"Id":"42","Path":"/media/video.mkv","IsShortcut":false,"MediaSources":[{"Id":"s1","Path":"/media/video.mkv"}]}]}`)
					}
					return
				}
				videoCalls.Add(1)
				if r.Header.Get("Range") != "bytes=1-2" {
					t.Error("本地回源丢失Range")
				}
				w.Header().Set("X-Request-ID", "upstream-id")
				w.Header().Set("Content-Range", "bytes 1-2/4")
				w.WriteHeader(206)
				_, _ = io.WriteString(w, "ok")
			}))
			defer remote.Close()
			request := httptest.NewRequest("GET", path, nil)
			query := request.URL.Query()
			query.Set("MediaSourceId", "s1")
			query.Set("api_key", "query-token")
			query.Set("UserId", "u1")
			request.URL.RawQuery = query.Encode()
			request.Header.Set("X-Emby-Token", "user-token")
			request.Header.Set("Cookie", "session=user")
			request.Header.Set("Range", "bytes=1-2")
			h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{BaseURL: remote.URL, APIKey: "admin-key"})
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			if response.Code != 206 || response.Body.String() != "ok" || videoCalls.Load() != 1 || metadataCalls.Load() != 2 {
				t.Fatalf("本地播放未正常回源: code=%d metadata=%d video=%d", response.Code, metadataCalls.Load(), videoCalls.Load())
			}
			if response.Header().Get("X-Request-ID") == "upstream-id" || len(response.Header().Values("X-Request-ID")) != 1 {
				t.Fatal("请求标识被上游覆盖")
			}
		})
	}
}

func TestEmbyProxyDirectSTRMPlaybackDoesNotProbeTarget(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1); w.WriteHeader(403) }))
	defer target.Close()
	location := target.URL + "/video?signature=private-signature"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
			t.Error("不应读取其他上游接口")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaSources": []map[string]string{{"Id": "s1", "Path": location}}})
	}))
	defer remote.Close()
	h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{BaseURL: remote.URL, APIKey: "key"})
	for _, path := range []string{"/Videos/42/stream.webm", "/Videos/42/original", "/emby/Items/42/Download"} {
		for _, method := range []string{"GET", "HEAD"} {
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest(method, path, nil))
			if response.Code != 302 || response.Header().Get("Location") != location || response.Body.Len() != 0 || targetCalls.Load() != 0 {
				t.Fatalf("直接播放意外探测或传输了视频: code=%d target_calls=%d", response.Code, targetCalls.Load())
			}
		}
	}
}

func TestEmbyProxyMetadataTimeoutLogsOnce(t *testing.T) {
	output := captureEmbyProxyLog(t)
	var videoCalls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
			videoCalls.Add(1)
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer remote.Close()
	h := NewEmbyProxyService(nil, &http.Client{Timeout: 40 * time.Millisecond}).buildHandler(&domain.EmbyServer{ID: 7, BaseURL: remote.URL, APIKey: "private-key"})
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "/Videos/42/stream?api_key=private-token", nil))
	logs := output.String()
	if response.Code != 502 || videoCalls.Load() != 0 || strings.Count(logs, `"level":"ERROR"`) != 1 || !strings.Contains(logs, "超时") || strings.Contains(logs, "private-token") {
		t.Fatalf("超时处理或日志错误: %d %s", response.Code, logs)
	}
}

type embyProxyErrorTransport struct{ err error }

func (transport embyProxyErrorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, transport.err
}

func TestEmbyProxyTransportErrorDoesNotLeakURL(t *testing.T) {
	output := captureEmbyProxyLog(t)
	client := &http.Client{Transport: embyProxyErrorTransport{err: &url.Error{Op: "Get", URL: "https://cdn.example/video?signature=private-signature", Err: errors.New("private-upstream-error")}}}
	h := NewEmbyProxyService(nil, client).buildHandler(&domain.EmbyServer{ID: 7, BaseURL: "http://emby.example", APIKey: "private-key"})
	for _, path := range []string{"/Videos/42/stream?api_key=private-token", "/System/Info?api_key=private-token"} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 502 || response.Header().Get("X-Request-ID") == "" {
			t.Fatal("上游连接失败没有返回错误及请求标识")
		}
	}
	logs := output.String()
	if strings.Count(logs, `"level":"ERROR"`) != 2 {
		t.Fatalf("每个失败请求应只记录一次: %s", logs)
	}
	for _, secret := range []string{"private-token", "private-signature", "private-key", "private-upstream-error"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("错误日志泄露 %s: %s", secret, logs)
		}
	}
}

func TestEmbyProxyCanceledMetadataRequestDoesNotFallback(t *testing.T) {
	output := captureEmbyProxyLog(t)
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer remote.Close()
	h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{BaseURL: remote.URL, APIKey: "key"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("GET", "/Videos/42/stream", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 502 || calls.Load() != 0 || !strings.Contains(output.String(), "请求已取消") {
		t.Fatalf("取消请求错误回源: code=%d calls=%d logs=%s", response.Code, calls.Load(), output.String())
	}
}

func TestEmbyProxyNonPlaybackRequestsRemainTransparent(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/emby/Users/AuthenticateByName?api_key=user-token", `{"Username":"user"}`},
		{"GET", "/Items?searchTerm=movie&api_key=user-token", ""},
		{"GET", "/emby/Items/42/Images/Primary?api_key=user-token", ""},
		{"HEAD", "/Videos/42/s1/Subtitles/0/Stream.vtt?api_key=user-token", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			var calls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != tc.method || r.URL.RequestURI() != tc.path || r.Header.Get("Cookie") != "session=user" || r.Header.Get("Authorization") != "Bearer user" {
					t.Error("普通 Emby 请求未透明反代")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != tc.body || r.Header.Get("X-Request-ID") == "" || r.Header.Get("X-Request-ID") == "client-id" {
					t.Error("请求体或请求标识异常")
				}
				w.Header().Set("X-Request-ID", "upstream-id")
				w.Header().Set("Set-Cookie", "session=upstream")
				if r.Method != "HEAD" {
					_, _ = io.WriteString(w, "response")
				}
			}))
			defer remote.Close()
			h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{BaseURL: remote.URL})
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Cookie", "session=user")
			request.Header.Set("Authorization", "Bearer user")
			request.Header.Set("X-Request-ID", "client-id")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			if response.Code != 200 || calls.Load() != 1 || response.Header().Get("Set-Cookie") != "session=upstream" {
				t.Fatalf("普通请求被播放识别干扰: code=%d calls=%d", response.Code, calls.Load())
			}
			if id := response.Header().Get("X-Request-ID"); id == "" || id == "upstream-id" || id == "client-id" || len(response.Header().Values("X-Request-ID")) != 1 {
				t.Fatal("请求标识未由反代生成或出现重复")
			}
			if output.String() != "" {
				t.Fatalf("正常请求不应记录失败日志: %s", output.String())
			}
		})
	}
}

func TestEmbyProxyOrdinaryUpstreamFailureLogsOnce(t *testing.T) {
	output := captureEmbyProxyLog(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "upstream-id")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "private-upstream-body")
	}))
	defer remote.Close()
	h := NewEmbyProxyService(nil, remote.Client()).buildHandler(&domain.EmbyServer{ID: 7, ProxyPort: 8097, BaseURL: remote.URL})
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "/System/Info?api_key=private-token", nil))
	logs := output.String()
	if response.Code != 503 || response.Body.String() != "private-upstream-body" || strings.Count(logs, `"level":"ERROR"`) != 1 {
		t.Fatalf("普通上游失败应保持响应并记录一次: %d %s", response.Code, logs)
	}
	for _, want := range []string{`"request_id":"` + response.Header().Get("X-Request-ID") + `"`, `"stage":"proxy"`, `"status":503`, `"upstream_status":503`, `"fallback_blocked":false`} {
		if !strings.Contains(logs, want) {
			t.Errorf("日志缺少 %q: %s", want, logs)
		}
	}
	if strings.Contains(logs, "private-token") || strings.Contains(logs, "private-upstream-body") {
		t.Fatal("普通反代失败日志泄露敏感信息")
	}
}
