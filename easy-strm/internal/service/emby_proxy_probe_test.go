package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

func TestEmbyProxyProbeRedirectCredentialsAndBrowserHeaders(t *testing.T) {
	var heads atomic.Int32
	var bodies []*embyProxyRelayTestBody
	service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
		heads.Add(1)
		if request.Method != "HEAD" || request.Header.Get("User-Agent") != "test-player" {
			t.Errorf("探测方法或 UA 不正确：%s %v", request.Method, request.Header)
		}
		for _, header := range []string{"Cookie", "Authorization", "X-Emby-Token", "X-MediaBrowser-Token", "X-Emby-Authorization", "Origin", "Referer", "Range", "If-Range"} {
			if request.Header.Get(header) != "" {
				t.Errorf("探测泄露请求头：%s", header)
			}
		}
		if strings.Contains(request.URL.RawQuery, "emby-query-secret") {
			t.Error("泄露客户端查询凭据")
		}
		body := &embyProxyRelayTestBody{Reader: strings.NewReader("")}
		bodies = append(bodies, body)
		header := make(http.Header)
		status := 200
		if request.URL.Path == "/video" {
			status = 302
			header.Set("Location", "https://1.1.1.1/final?signature=cdn")
		}
		return &http.Response{StatusCode: status, Header: header, Body: body}, nil
	})
	service.options.ResolveFinalURL, service.options.ProbeHEAD, service.options.BrowserRedirect = true, true, true
	handler := service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example", APIKey: "test"})
	for index := 0; index < 2; index++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, embyProxyRelayTestRequest("GET", "https://browser.example"))
		if response.Code != 302 || response.Header().Get("Location") != "https://1.1.1.1/final?signature=cdn" {
			t.Fatalf("最终 URL 错误：%d %v", response.Code, response.Header())
		}
		for header, value := range map[string]string{"Accept-Ranges": "bytes", "Access-Control-Allow-Origin": "https://browser.example", "Access-Control-Allow-Credentials": "true", "Cache-Control": "no-cache", "Referrer-Policy": "no-referrer", "Vary": "Origin"} {
			if response.Header().Get(header) != value {
				t.Errorf("302 缺少 %s：%v", header, response.Header())
			}
		}
	}
	if heads.Load() != 3 {
		t.Fatalf("独立预解析/探测或成功缓存错误：%d", heads.Load())
	}
	for _, body := range bodies {
		if !body.closed.Load() {
			t.Fatal("HEAD 响应体未关闭")
		}
	}
}

func TestEmbyProxyPreResolutionRetryAndOriginalFallback(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "retry_success", false: "original_fallback"}[success], func(t *testing.T) {
			var calls atomic.Int32
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				count := calls.Add(1)
				if !success || count < 3 {
					return nil, errors.New("临时连接错误")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			service.options.ResolveFinalURL = true
			started := time.Now()
			result := service.prepareMediaLocation(embyProxyRelayTestRequest("GET", ""), embyProxyTestMediaURL)
			if result.failure != nil || result.location != embyProxyTestMediaURL || result.original != embyProxyTestMediaURL || calls.Load() != 3 {
				t.Fatalf("重试/保留原地址失败：%+v calls=%d", result, calls.Load())
			}
			if time.Since(started) < 4*time.Second {
				t.Fatal("重试未保留两秒间隔")
			}
		})
	}
}

func TestEmbyProxyProbeJSONFailureBrowserRelayOriginalAndNoCache(t *testing.T) {
	var heads, gets atomic.Int32
	service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
		header := make(http.Header)
		if request.Method == "HEAD" {
			heads.Add(1)
			header.Set("Content-Type", "Application/JSON; Charset=UTF-8")
		} else {
			gets.Add(1)
			if request.URL.String() != embyProxyTestMediaURL {
				t.Errorf("未中继原始地址：%s", request.URL)
			}
		}
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader("video"))}, nil
	})
	service.options.ProbeHEAD, service.options.BrowserRedirect = true, true
	handler := service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example", APIKey: "test"})
	for index := 0; index < 2; index++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, embyProxyRelayTestRequest("GET", "https://browser.example"))
		if response.Code != 200 || response.Body.String() != "video" || response.Header().Get("Location") != "" {
			t.Fatalf("浏览器失败未降级中继：%d %s", response.Code, response.Body.String())
		}
	}
	if heads.Load() != 6 || gets.Load() != 2 || len(service.cache.entries) != 0 {
		t.Fatalf("失败探测被缓存：HEAD=%d GET=%d", heads.Load(), gets.Load())
	}
}

func TestEmbyProxyProbeNon2xxAndCancellation(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var body *embyProxyRelayTestBody
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				body = &embyProxyRelayTestBody{Reader: strings.NewReader("")}
				cancel()
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}, nil
			})
			service.options.ProbeHEAD = true
			result := service.prepareMediaLocation(embyProxyRelayTestRequest("GET", "").WithContext(ctx), embyProxyTestMediaURL)
			if result.failure == nil || result.original != embyProxyTestMediaURL || !body.closed.Load() {
				t.Fatal("非 2xx 未拒绝或未关闭响应体")
			}
		})
	}
	service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	service.options.ProbeHEAD = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := service.prepareMediaLocation(embyProxyRelayTestRequest("GET", "").WithContext(ctx), embyProxyTestMediaURL)
	if result.failure == nil || time.Since(started) > time.Second {
		t.Fatal("探测未及时响应 context 取消")
	}
}

func TestEmbyProxyProbeUnsafeRedirectNeverFallsBack(t *testing.T) {
	for _, resolve := range []bool{false, true} {
		var calls atomic.Int32
		service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"http://169.254.169.254/latest?api_key=unsafe"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		service.options.ResolveFinalURL, service.options.ProbeHEAD, service.options.BrowserRedirect = resolve, !resolve, true
		service.options.ProxyErrorStrategy = "fallback_origin"
		response := httptest.NewRecorder()
		service.buildHandler(&domain.EmbyServer{BaseURL: "http://emby.example"}).ServeHTTP(response, embyProxyRelayTestRequest("GET", "https://browser.example"))
		if response.Code != 502 || calls.Load() != 1 || len(service.cache.entries) != 0 {
			t.Fatalf("不安全重定向发生降级：%d calls=%d", response.Code, calls.Load())
		}
	}
}

func TestEmbyProxyProbeRedirectLimit(t *testing.T) {
	var calls atomic.Int32
	service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {embyProxyTestMediaURL}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, failure := service.fetchRelayMedia(ctx, embyProxyRelayTestRequest("HEAD", ""), embyProxyTestMediaURL)
	if failure == nil || failure.reason != "redirect_limit" || calls.Load() != embyProxyRelayMaxRedirects+1 {
		t.Fatalf("重定向无上限：%v %d", failure, calls.Load())
	}
}
