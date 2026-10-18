package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

func TestEmbyProxyFallbackPolicyOriginalPathAndQuery(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		path            string
		body            string
		metadataStatus  int
		browser         bool
		browserRedirect bool
		strategy        string
		want            int
		forward         bool
	}{
		{name: "default_transcode_reject", path: "/emby/Videos/42/stream?Static=false", want: 502},
		{name: "default_hls_reject", path: "/emby/Videos/42/hls1/main/0.ts?api_key=client", want: 502},
		{name: "transcode_origin", path: "/emby/Videos/42/stream?Static=false&api_key=client&opaque=a%2Fb", strategy: "fallback_origin", want: 206, forward: true},
		{name: "hls_origin", path: "/emby/Videos/42/hls1/main/0.ts?api_key=client&opaque=a%2Fb", strategy: "fallback_origin", want: 206, forward: true},
		{name: "browser_transcode_still_reject", path: "/emby/Videos/42/stream?Static=false", browser: true, browserRedirect: true, want: 502},
		{name: "missing_link_default", body: `{"MediaSources":[{"Path":"/media/movie.strm"}]}`, want: 502},
		{name: "missing_link_browser_disabled", body: `{"MediaSources":[{"Path":"/media/movie.strm"}]}`, browser: true, want: 502},
		{name: "missing_link_browser_enabled", body: `{"MediaSources":[{"Path":"/media/movie.strm"}]}`, browser: true, browserRedirect: true, want: 206, forward: true},
		{name: "missing_link_native_origin", body: `{"MediaSources":[{"Path":"/media/movie.strm"}]}`, strategy: "fallback_origin", want: 206, forward: true},
		{name: "missing_source_browser_enabled", body: `{"MediaSources":[]}`, browser: true, browserRedirect: true, want: 206, forward: true},
		{name: "missing_transcode_browser_reject", body: `{"MediaSources":[]}`, path: "/emby/Videos/42/stream?Static=false", browser: true, browserRedirect: true, want: 502},
		{name: "emby_error_code", body: `{"ErrorCode":"NotAllowed","MediaSources":[]}`, browser: true, browserRedirect: true, strategy: "fallback_origin", want: 502},
		{name: "authorization401", metadataStatus: 401, browser: true, browserRedirect: true, strategy: "fallback_origin", want: 401},
		{name: "authorization403", metadataStatus: 403, browser: true, browserRedirect: true, strategy: "fallback_origin", want: 403},
		{name: "invalid_json", body: `{`, browser: true, browserRedirect: true, strategy: "fallback_origin", want: 502},
		{name: "ambiguous_source", body: `{"MediaSources":[{"Path":"https://8.8.8.8/a"},{"Path":"https://8.8.8.8/b"}]}`, browser: true, browserRedirect: true, strategy: "fallback_origin", want: 502},
		{name: "unsafe_source", body: `{"MediaSources":[{"Path":"http://169.254.169.254/latest"}]}`, browser: true, browserRedirect: true, strategy: "fallback_origin", want: 502},
		{name: "unsafe_transcode", body: `{"MediaSources":[{"Path":"http://169.254.169.254/latest"}]}`, path: "/emby/Videos/42/stream?Static=false", strategy: "fallback_origin", want: 502},
		{name: "userinfo", body: `{"MediaSources":[{"Path":"http://user:secret@8.8.8.8/a"}]}`, strategy: "fallback_origin", want: 502},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.path == "" {
				testCase.path = "/emby/Videos/42/stream?api_key=client&opaque=a%2Fb"
			}
			if testCase.body == "" {
				testCase.body = `{"MediaSources":[{"Path":"https://8.8.8.8/movie"}]}`
			}
			forwarded := make(chan string, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if serveEmbyProxySTRMItemFixture(writer, request) {
					return
				}
				if strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
					if testCase.metadataStatus != 0 {
						writer.WriteHeader(testCase.metadataStatus)
						return
					}
					io.WriteString(writer, testCase.body)
					return
				}
				forwarded <- request.URL.RequestURI()
				if request.URL.Query().Get("api_key") != "client" || request.Header.Get("X-Emby-Token") != "client-token" {
					t.Error("原始回源请求身份被替换")
				}
				writer.WriteHeader(206)
				io.WriteString(writer, "origin-video")
			}))
			defer upstream.Close()
			options := domain.DefaultEmbyProxyConfig()
			options.BrowserRedirect = testCase.browserRedirect
			if testCase.strategy != "" {
				options.ProxyErrorStrategy = testCase.strategy
			}
			service, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			service.relayClient.Transport = embyProxyRelayTestTransport(func(*http.Request) (*http.Response, error) {
				t.Error("不应调用媒体地址")
				return nil, errEmbyProxyRelayUnsafe
			})
			request := httptest.NewRequest("GET", testCase.path, nil)
			request.Header.Set("X-Emby-Token", "client-token")
			if testCase.browser {
				request.Header.Set("Origin", "https://browser.example")
			}
			response := httptest.NewRecorder()
			service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL + "/gateway/emby", APIKey: "admin"}).ServeHTTP(response, request)
			if response.Code != testCase.want {
				t.Fatalf("响应=%d，期望=%d %s", response.Code, testCase.want, response.Body.String())
			}
			if testCase.forward {
				select {
				case uri := <-forwarded:
					if uri != "/gateway"+testCase.path {
						t.Fatalf("回源路径/查询被改写：%s", uri)
					}
				default:
					t.Fatal("未回源可信 Emby")
				}
			} else if len(forwarded) != 0 {
				t.Fatal("禁止降级的错误发生了回源")
			}
		})
	}
}

func TestEmbyProxyProbeNativeRejectVersusOrigin(t *testing.T) {
	for _, strategy := range []string{"reject", "fallback_origin"} {
		t.Run(strategy, func(t *testing.T) {
			forwarded := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if serveEmbyProxySTRMItemFixture(writer, request) {
					return
				}
				if strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
					io.WriteString(writer, `{"MediaSources":[{"Path":"https://8.8.8.8/video"}]}`)
					return
				}
				forwarded <- request.URL.RequestURI()
				io.WriteString(writer, "origin-video")
			}))
			defer upstream.Close()
			options := domain.DefaultEmbyProxyConfig()
			options.ProbeHEAD, options.ProxyErrorStrategy = true, strategy
			service, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			service.relayClient.Transport = embyProxyRelayTestTransport(func(request *http.Request) (*http.Response, error) {
				if request.Method != "HEAD" {
					t.Error("原生客户端探测失败不应中继媒体 GET")
				}
				return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			response := httptest.NewRecorder()
			path := "/Videos/42/stream?api_key=client&opaque=a%2Fb"
			service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}).ServeHTTP(response, httptest.NewRequest("GET", path, nil))
			if strategy == "reject" {
				if response.Code != 502 || len(forwarded) != 0 {
					t.Fatal("默认探测失败未拒绝")
				}
			} else if response.Code != 200 || response.Body.String() != "origin-video" || <-forwarded != path {
				t.Fatal("探测失败未按原路径回源 Emby")
			}
			if len(service.cache.entries) != 0 {
				t.Fatal("失败探测被缓存")
			}
		})
	}
}
