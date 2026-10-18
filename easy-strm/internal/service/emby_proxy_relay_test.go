package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

const embyProxyTestMediaURL = "https://8.8.8.8/video?signature=media-signature"

type embyProxyRelayTestTransport func(*http.Request) (*http.Response, error)

func (transport embyProxyRelayTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type embyProxyRelayTestBody struct {
	io.Reader
	closed atomic.Bool
}

func (body *embyProxyRelayTestBody) Close() error {
	body.closed.Store(true)
	return nil
}

func newEmbyProxyRelayTestService(t *testing.T, transport embyProxyRelayTestTransport) *EmbyProxyService {
	t.Helper()
	metadata := embyProxyRelayTestTransport(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/Items") {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Request: request, Body: io.NopCloser(strings.NewReader(`{"Items":[{"Id":"42","Type":"Movie","Path":"/media/movie.strm"}]}`))}, nil
		}
		if !strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
			t.Errorf("中继不可回退视频回源: %s", request.URL.Path)
		}
		return &http.Response{
			StatusCode: 200, Header: make(http.Header), Request: request,
			Body: io.NopCloser(strings.NewReader(`{"MediaSources":[{"Id":"s1","Path":"` + embyProxyTestMediaURL + `"}]}`)),
		}, nil
	})
	service := NewEmbyProxyService(nil, &http.Client{Transport: metadata, Timeout: time.Second})
	service.relayClient.CloseIdleConnections()
	service.relayClient.Transport = transport
	t.Cleanup(service.Close)
	return service
}

func embyProxyRelayTestRequest(method, origin string) *http.Request {
	request := httptest.NewRequest(method, "/Videos/42/stream?api_key=emby-query-secret&MediaSourceId=s1", nil)
	request.Header = http.Header{
		"Origin": {origin}, "Range": {"bytes=1-5"}, "If-Range": {`"version"`}, "Accept": {"video/*"}, "User-Agent": {"test-player"},
		"If-Match": {`"version"`}, "If-None-Match": {`"old-version"`}, "If-Modified-Since": {"Mon, 12 Oct 2026 00:00:00 GMT"}, "If-Unmodified-Since": {"Mon, 12 Oct 2026 01:00:00 GMT"},
		"Cookie": {"session=emby-cookie-secret"}, "Authorization": {"Bearer emby-auth-secret"},
		"X-Emby-Token": {"emby-token-secret"}, "X-Mediabrowser-Token": {"emby-token-secret"}, "X-Emby-Authorization": {"emby-auth-secret"},
		"Referer": {"https://emby.example/?api_key=emby-query-secret"}, "X-Request-Id": {"relay-test-request"},
	}
	return request
}

func assertEmbyProxyRelayRequest(t *testing.T, outgoing *http.Request, method string) {
	t.Helper()
	if outgoing.Method != method {
		t.Errorf("重定向改变方法: %s，期望 %s", outgoing.Method, method)
	}
	for _, name := range []string{"Range", "If-Range", "Accept", "User-Agent", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
		if outgoing.Header.Get(name) != embyProxyRelayTestRequest(method, "").Header.Get(name) {
			t.Errorf("媒体请求丢失 %s: %v", name, outgoing.Header)
		}
	}
	for _, name := range []string{"Cookie", "Authorization", "X-Emby-Token", "X-MediaBrowser-Token", "X-Emby-Authorization", "Origin", "Referer", "X-Request-ID"} {
		if outgoing.Header.Get(name) != "" {
			t.Errorf("媒体请求泄露 %s", name)
		}
	}
	if strings.Contains(outgoing.URL.String(), "emby-query-secret") || outgoing.URL.User != nil {
		t.Error("媒体请求泄露 Emby 查询凭据")
	}
}

func TestEmbyProxyBrowserRelayVersusNative302(t *testing.T) {
	for _, status := range []int{200, 206, 416} {
		for _, method := range []string{"GET", "HEAD"} {
			for _, origin := range []string{embyProxyTestOrigin, ""} {
				t.Run(fmt.Sprintf("%d/%s/%s", status, method, origin), func(t *testing.T) {
					calls := 0
					body := &embyProxyRelayTestBody{Reader: strings.NewReader("video")}
					upstreamHeader := http.Header{
						"Content-Type": {"video/mp4"}, "Content-Length": {"5"}, "Content-Range": {"bytes 1-5/10"}, "Accept-Ranges": {"bytes"},
						"Last-Modified": {"Mon, 12 Oct 2026 00:00:00 GMT"}, "Etag": {`"version"`}, "Vary": {"Accept-Encoding"},
						"Set-Cookie": {"cdn-secret"}, "X-Request-Id": {"untrusted"}, "Access-Control-Allow-Origin": {"*"},
					}
					if status == 416 {
						upstreamHeader.Set("Content-Range", "bytes */10")
					}
					service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
						calls++
						assertEmbyProxyRelayRequest(t, request, method)
						if request.URL.String() != embyProxyTestMediaURL {
							t.Error("中继改变签名直链")
						}
						return &http.Response{StatusCode: status, Header: upstreamHeader, Body: body, Request: request}, nil
					})
					request := embyProxyRelayTestRequest(method, origin)
					response := httptest.NewRecorder()
					service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example", APIKey: "metadata-key"}).ServeHTTP(response, request)
					assertEmbyProxyCORS(t, response.Header(), origin)
					if origin == "" {
						if response.Code != 302 || calls != 0 || response.Body.Len() != 0 || response.Header().Get("Location") != embyProxyTestMediaURL {
							t.Fatalf("原生播放不应中继: %d calls=%d body=%q", response.Code, calls, response.Body.String())
						}
						return
					}
					if response.Code != status || calls != 1 || !body.closed.Load() || response.Header().Get("Location") != "" {
						t.Fatalf("浏览器中继失败: %d calls=%d closed=%v", response.Code, calls, body.closed.Load())
					}
					for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
						if response.Header().Get(name) != upstreamHeader.Get(name) {
							t.Errorf("丢失响应头 %s", name)
						}
					}
					if response.Header().Get("X-Request-ID") != "relay-test-request" || response.Header().Get("Set-Cookie") != "" || !embyProxyHeaderContains(response.Header(), "Vary", "Accept-Encoding") || response.Header().Get("Cache-Control") != "no-store" {
						t.Errorf("响应隔离或 Vary 错误: %v", response.Header())
					}
					wantBody := "video"
					if method == "HEAD" {
						wantBody = ""
					}
					if response.Body.String() != wantBody {
						t.Errorf("中继正文=%q，期望 %q", response.Body.String(), wantBody)
					}
				})
			}
		}
	}
}

func TestProxyRelayRedirectRangeAndMethod(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		for _, status := range []int{301, 302, 303, 307, 308} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				var bodies []*embyProxyRelayTestBody
				service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
					assertEmbyProxyRelayRequest(t, request, method)
					index := len(bodies)
					if index > 0 && !bodies[index-1].closed.Load() {
						t.Error("未关闭前一跳正文")
					}
					body := &embyProxyRelayTestBody{Reader: strings.NewReader("video")}
					bodies = append(bodies, body)
					response := &http.Response{StatusCode: status, Body: body, Request: request, Header: make(http.Header)}
					switch index {
					case 0:
						response.Header.Set("Location", "/next?signature=next")
					case 1:
						if request.URL.String() != "https://8.8.8.8/next?signature=next" {
							t.Error("相对重定向解析错误")
						}
						response.Header.Set("Location", "http://1.1.1.1/final")
					default:
						if request.URL.String() != "http://1.1.1.1/final" {
							t.Error("跨域重定向解析错误")
						}
						response.StatusCode = 206
					}
					return response, nil
				})
				response := httptest.NewRecorder()
				service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(response, embyProxyRelayTestRequest(method, embyProxyTestOrigin))
				if response.Code != 206 || len(bodies) != 3 {
					t.Fatalf("重定向中继失败: %d hops=%d", response.Code, len(bodies))
				}
				for _, body := range bodies {
					if !body.closed.Load() {
						t.Error("未关闭中继正文")
					}
				}
			})
		}
	}
}

func TestProxyRelayRedirectLimitAndUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name, location                   string
		redirects, wantStatus, wantCalls int
	}{
		{"五跳成功", "/next", 5, 200, 6},
		{"六跳拒绝", "/next", 6, 502, 6},
		{"循环拒绝", embyProxyTestMediaURL, 100, 502, 6},
		{"本地地址", "http://127.0.0.1/video?signature=private", 1, 200, 2},
		{"私网地址", "https://10.0.0.1/video", 1, 200, 2},
		{"链路本地", "http://169.254.169.254/latest/meta-data", 1, 502, 1},
		{"IPv4映射", "http://[::ffff:127.0.0.1]/video", 1, 200, 2},
		{"非HTTP", "file:///etc/passwd", 1, 502, 1},
		{"凭据地址", "https://user:secret@8.8.8.8/video", 1, 502, 1},
		{"无地址", "", 1, 502, 1},
		{"错误地址", "http://%zz", 1, 502, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			var bodies []*embyProxyRelayTestBody
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				body := &embyProxyRelayTestBody{Reader: strings.NewReader("video")}
				bodies = append(bodies, body)
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Request: request}
				if len(bodies) <= tc.redirects {
					response.StatusCode = 302
					response.Header.Set("Location", tc.location)
				}
				return response, nil
			})
			response := httptest.NewRecorder()
			service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(response, embyProxyRelayTestRequest("GET", embyProxyTestOrigin))
			assertEmbyProxyCORS(t, response.Header(), embyProxyTestOrigin)
			if response.Code != tc.wantStatus || len(bodies) != tc.wantCalls {
				t.Fatalf("重定向边界失败: %d calls=%d", response.Code, len(bodies))
			}
			for _, body := range bodies {
				if !body.closed.Load() {
					t.Error("失败路径未关闭正文")
				}
			}
			if strings.Contains(output.String(), "signature=") || strings.Contains(output.String(), "user:secret") || strings.Contains(output.String(), "emby-query-secret") {
				t.Error("重定向失败日志泄露地址或凭据")
			}
		})
	}
}

func TestProxyRelayPreHeaderFailures(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				return nil, errors.New("失败地址 " + embyProxyTestMediaURL)
			})
			response := httptest.NewRecorder()
			service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(response, embyProxyRelayTestRequest(method, embyProxyTestOrigin))
			assertEmbyProxyCORS(t, response.Header(), embyProxyTestOrigin)
			if response.Code != 502 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(output.String(), `"stage":"relay"`) {
				t.Fatalf("中继失败未走统一失败机制: %d %s", response.Code, output.String())
			}
			if method == "GET" && !strings.Contains(response.Body.String(), "媒体播放上游不可用") {
				t.Error("头部发送前失败缺少可读正文")
			}
			if method == "HEAD" && response.Body.Len() != 0 {
				t.Error("HEAD 失败不应写正文")
			}
			if strings.Contains(output.String()+response.Body.String(), "media-signature") {
				t.Error("中继失败泄露签名地址")
			}
		})
	}
}

type embyProxyRelayErrorReader struct {
	delivered bool
}

func (reader *embyProxyRelayErrorReader) Read(buffer []byte) (int, error) {
	if !reader.delivered {
		reader.delivered = true
		return copy(buffer, "video"), nil
	}
	return 0, errors.New("读取失败 " + embyProxyTestMediaURL)
}

type embyProxyRelayErrorWriter struct {
	*httptest.ResponseRecorder
}

func (writer embyProxyRelayErrorWriter) Write(buffer []byte) (int, error) {
	return 0, errors.New("播放器断开 " + embyProxyTestMediaURL)
}

func TestProxyRelayStreamingErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		downstream    bool
		contentLength string
	}{
		{"upstream_unknown_length", false, ""},
		{"upstream_known_length", false, "10"},
		{"downstream_unknown_length", true, ""},
		{"downstream_known_length", true, "10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureEmbyProxyLog(t)
			body := &embyProxyRelayTestBody{Reader: &embyProxyRelayErrorReader{}}
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				header := http.Header{"Content-Type": {"video/mp4"}}
				if tc.contentLength != "" {
					header.Set("Content-Length", tc.contentLength)
				}
				return &http.Response{StatusCode: 206, Body: body, Request: request, Header: header}, nil
			})
			response := httptest.NewRecorder()
			var writer http.ResponseWriter = response
			if tc.downstream {
				writer = embyProxyRelayErrorWriter{response}
			}
			service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(writer, embyProxyRelayTestRequest("GET", embyProxyTestOrigin))
			if response.Code != 206 || !body.closed.Load() || !strings.Contains(output.String(), `"reason":"stream_failed"`) {
				t.Fatalf("流错误未正常返回、保留原响应并关闭正文: status=%d closed=%v log=%s", response.Code, body.closed.Load(), output.String())
			}
			if response.Header().Get("Content-Length") != tc.contentLength || response.Header().Get("Content-Type") != "video/mp4" {
				t.Error("流错误改写了媒体响应头")
			}
			wantBody := "video"
			if tc.downstream {
				wantBody = ""
			}
			if response.Body.String() != wantBody || strings.Contains(output.String(), "media-signature") {
				t.Error("流错误追加错误正文或泄露签名地址")
			}
			assertEmbyProxyCORS(t, response.Header(), embyProxyTestOrigin)
		})
	}
}

type embyProxyRelayCancelReader struct {
	ctx       context.Context
	blocked   chan struct{}
	delivered bool
}

func (reader *embyProxyRelayCancelReader) Read(buffer []byte) (int, error) {
	if !reader.delivered {
		reader.delivered = true
		return copy(buffer, "video"), nil
	}
	close(reader.blocked)
	<-reader.ctx.Done()
	return 0, reader.ctx.Err()
}

func TestProxyRelayStreamingCancellationAndClose(t *testing.T) {
	for _, closeService := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%v", closeService), func(t *testing.T) {
			captureEmbyProxyLog(t)
			blocked := make(chan struct{})
			var body *embyProxyRelayTestBody
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				body = &embyProxyRelayTestBody{Reader: &embyProxyRelayCancelReader{ctx: request.Context(), blocked: blocked}}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Request: request}, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := embyProxyRelayTestRequest("GET", embyProxyTestOrigin).WithContext(ctx)
			response := httptest.NewRecorder()
			finished := make(chan struct{})
			go func() {
				service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(response, request)
				close(finished)
			}()
			select {
			case <-blocked:
			case <-time.After(3 * time.Second):
				t.Fatal("中继未开始读取流")
			}
			if response.Body.String() != "video" {
				t.Error("上游未结束时字节未向客户端流式写入")
			}
			if closeService {
				service.Close()
			} else {
				cancel()
			}
			select {
			case <-finished:
				if !body.closed.Load() || response.Code != 200 || response.Body.String() != "video" {
					t.Error("取消未正常返回、保留原响应并关闭正文")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("中继未响应取消")
			}
		})
	}
}

func TestProxyRelayAllowedIPPolicy(t *testing.T) {
	for _, tc := range []struct {
		address string
		allowed bool
	}{
		{"8.8.8.8", true}, {"1.1.1.1", true}, {"2606:4700:4700::1111", true}, {"::ffff:8.8.8.8", true},
		{"127.0.0.1", true}, {"127.255.255.255", true}, {"10.0.0.1", true}, {"172.16.0.1", true}, {"192.168.31.12", true},
		{"169.254.169.254", false}, {"0.0.0.0", false}, {"0.1.2.3", false}, {"224.0.0.1", false}, {"255.255.255.255", false},
		{"100.64.0.1", true}, {"100.127.255.255", true}, {"198.18.0.1", true}, {"192.0.2.1", true}, {"198.51.100.1", true}, {"203.0.113.1", true},
		{"::", false}, {"::1", true}, {"fc00::1", true}, {"fd00::1", true}, {"fe80::1", false}, {"ff02::1", false},
		{"::ffff:127.0.0.1", true}, {"::ffff:10.0.0.1", true}, {"::ffff:192.168.31.12", true}, {"::ffff:100.64.0.1", true},
		{"::ffff:169.254.169.254", false}, {"::ffff:0.0.0.0", false}, {"::ffff:0.1.2.3", false}, {"::ffff:224.0.0.1", false}, {"::ffff:255.255.255.255", false},
		{"2001:db8::1", true}, {"2002:7f00:1::", true}, {"64:ff9b::7f00:1", true}, {"2606:4700:4700::1111%eth0", false},
		{"::ffff:127.0.0.1%eth0", false}, {"::ffff:8.8.8.8%eth0", false}, {"::1%eth0", false}, {"fe80::1%eth0", false}, {"febf:ffff::1", false},
	} {
		t.Run(tc.address, func(t *testing.T) {
			if actual := isEmbyProxyRelayAllowedIP(netip.MustParseAddr(tc.address)); actual != tc.allowed {
				t.Errorf("地址允许策略=%v，期望 %v", actual, tc.allowed)
			}
		})
	}
	if isEmbyProxyRelayAllowedIP(netip.Addr{}) {
		t.Error("不应接受无效IP")
	}
}

func TestProxyRelayDNSPolicyAndPinnedDial(t *testing.T) {
	for _, tc := range []struct {
		name        string
		answers     []netip.Addr
		lookupError error
		allowed     bool
	}{
		{"公网", []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil, true},
		{"公网及私网", []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("192.168.31.12")}, nil, true},
		{"公网及回环", []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil, true},
		{"公网及链路本地", []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("169.254.169.254")}, nil, false},
		{"公网及IPv6链路本地", []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("fe80::1")}, nil, false},
		{"公网及无效地址", []netip.Addr{netip.MustParseAddr("8.8.8.8"), {}}, nil, false},
		{"空答案", nil, nil, false},
		{"内网", []netip.Addr{netip.MustParseAddr("192.168.31.12")}, nil, true},
		{"解析失败", nil, errors.New("lookup failed"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			location, _ := url.Parse("https://cdn.example/video")
			lookups := 0
			lookup := func(ctx context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if host != "cdn.example" || network != "ip" {
					t.Errorf("错误解析参数: %s %s", network, host)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("DNS 解析没有时限")
				}
				if lookups > 1 {
					return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
				}
				return tc.answers, tc.lookupError
			}
			target, err := resolveEmbyProxyRelayTarget(context.Background(), location, lookup)
			if (err == nil) != tc.allowed {
				t.Fatalf("DNS 校验=%v，期望 allowed=%v", err, tc.allowed)
			}
			if !tc.allowed {
				return
			}
			connection, err := target.dial(context.Background(), "tcp", func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != net.JoinHostPort(tc.answers[0].Unmap().String(), "443") || lookups != 1 {
					t.Errorf("拨号必须固定已验证IP，不可二次解析: address=%s lookups=%d", address, lookups)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("拨号没有时限")
				}
				client, peer := net.Pipe()
				_ = peer.Close()
				return client, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
			if _, err := resolveEmbyProxyRelayTarget(context.Background(), location, lookup); err == nil {
				t.Error("后续重绑定答案应拒绝")
			}
		})
	}
}

func TestProxyRelayProductionTransportSafety(t *testing.T) {
	service := NewEmbyProxyService(nil, &http.Client{Timeout: time.Millisecond})
	defer service.Close()
	transport := service.relayClient.Transport.(*http.Transport)
	if service.relayClient.Timeout != 0 || transport.Proxy != nil || transport.DialContext == nil || transport.DialTLSContext != nil || transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 || transport.IdleConnTimeout <= 0 || transport.MaxIdleConns <= 0 || !transport.DisableCompression {
		t.Fatal("生产中继必须独立、有界建连、无整请求超时且不能绕过安全拨号")
	}
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		address string
	}{
		{"未校验", context.Background(), "127.0.0.1:80"},
		{"目标不符", context.WithValue(context.Background(), embyProxyRelayTargetKey{}, embyProxyRelayTarget{address: "cdn.example:443", port: "443", ips: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}), "other.example:443"},
		{"链路本地IP", context.WithValue(context.Background(), embyProxyRelayTargetKey{}, embyProxyRelayTarget{address: "cdn.example:443", port: "443", ips: []netip.Addr{netip.MustParseAddr("169.254.169.254")}}), "cdn.example:443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if connection, err := transport.DialContext(tc.ctx, "tcp", tc.address); !errors.Is(err, errEmbyProxyRelayUnsafe) || connection != nil {
				t.Fatalf("未经安全校验的拨号应被安全策略拒绝，connection=%v, err=%v", connection, err)
			}
		})
	}
	request := httptest.NewRequest("GET", "/video", nil)
	if err := service.relayClient.CheckRedirect(request, nil); err != http.ErrUseLastResponse {
		t.Fatal("重定向必须交给逐跳校验逻辑")
	}
}

type embyProxyRelayCleanupTransport struct {
	closed atomic.Int32
}

func (transport *embyProxyRelayCleanupTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return nil, errors.New("不应发出请求")
}

func (transport *embyProxyRelayCleanupTransport) CloseIdleConnections() {
	transport.closed.Add(1)
}

func TestProxyRelayTransportLifecycleCleanup(t *testing.T) {
	service := NewEmbyProxyService(nil, nil)
	transport := &embyProxyRelayCleanupTransport{}
	service.relayClient.CloseIdleConnections()
	service.relayClient.Transport = transport
	service.Close()
	if transport.closed.Load() != 1 || !errors.Is(service.relayContext.Err(), context.Canceled) {
		t.Fatal("服务关闭必须取消中继生命周期并释放专用传输连接")
	}
}
