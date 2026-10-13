package service

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

func newEmbyDirectPlayResponseTestProxy(t *testing.T, enabled bool, body string, header http.Header, chunked bool) (*httptest.Server, <-chan embyDirectPlayCapture) {
	t.Helper()
	captured := make(chan embyDirectPlayCapture, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		input, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("读取上游请求失败：%v", err)
		}
		captured <- embyDirectPlayCapture{body: input, contentLength: request.ContentLength, lengthHeader: request.Header.Get("Content-Length"), transfer: request.TransferEncoding, method: request.Method, path: request.URL.Path, query: request.URL.RawQuery, token: request.Header.Get("X-Emby-Token")}
		for name, values := range header {
			writer.Header()[name] = append([]string(nil), values...)
		}
		if !chunked {
			writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		writer.WriteHeader(http.StatusOK)
		if chunked {
			writer.(http.Flusher).Flush()
		}
		if _, err := io.WriteString(writer, body); err != nil {
			t.Errorf("写入上游响应失败：%v", err)
		}
	}))
	t.Cleanup(upstream.Close)
	options := domain.DefaultEmbyProxyConfig()
	options.ForceDirectPlay = enabled
	service, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	proxy := httptest.NewServer(service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}))
	t.Cleanup(proxy.Close)
	return proxy, captured
}

func sendEmbyDirectPlayResponseRequest(t *testing.T, proxy *httptest.Server, captured <-chan embyDirectPlayCapture, method, path, body string, chunked bool, encoding string) (*http.Response, []byte, embyDirectPlayCapture) {
	t.Helper()
	var reader io.Reader = strings.NewReader(body)
	if chunked {
		reader = io.NopCloser(reader)
	}
	request, err := http.NewRequest(method, proxy.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Emby-Token", "private-header-token")
	if encoding != "" {
		request.Header.Set("Accept-Encoding", encoding)
	}
	response, err := proxy.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("读取或关闭代理响应失败：%v / %v", readErr, closeErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("代理状态=%d", response.StatusCode)
	}
	return response, actual, <-captured
}

func assertEmbyDirectPlayResponseFraming(t *testing.T, response *http.Response, actual []byte, chunked bool) {
	t.Helper()
	if chunked && len(actual) == 0 && response.ContentLength == 0 && response.Header.Get("Content-Length") == "0" && len(response.TransferEncoding) == 0 {
		return
	}
	if chunked {
		if response.ContentLength != -1 || response.Header.Get("Content-Length") != "" || !reflect.DeepEqual(response.TransferEncoding, []string{"chunked"}) {
			t.Fatal("透传 chunked framing 被改变")
		}
	} else if response.ContentLength != int64(len(actual)) || response.Header.Get("Content-Length") != strconv.Itoa(len(actual)) || len(response.TransferEncoding) != 0 || response.Header.Get("Transfer-Encoding") != "" {
		t.Fatal("Content-Length 或 Transfer-Encoding 未同步")
	}
}

func TestEmbyProxyForceDirectPlayResponseIntegration(t *testing.T) {
	const input = ` { "PlaySessionId":"session", "Ticks":900719925474099312345, "MediaSources":[{"Id":"one","Path":"https://synthetic.example/direct-link?api_key=private","Container":"mp4","Protocol":"Http","IsRemote":true,"SupportsDirectStream":true,"SupportsTranscoding":true,"SupportsDirectPlay":false,"DirectStreamUrl":"/original.mp4","TranscodingUrl":"/master.m3u8","MediaStreams":[{"Codec":"h264","BitRate":9007199254740993}]},{"Id":"two","IsRemote":false},{"Id":"three","Unknown":[null,0.123456789012345678901]}]} `
	const requestBody = ` { "DeviceProfile": { "EnableTranscoding": true }, "Ticks":9007199254740993 } `
	for _, test := range []struct {
		name, method, path, contentType string
		disabled, skipped               bool
	}{
		{name: "enabled", path: "/Items/42/PlaybackInfo?api_key=private-query-token"},
		{name: "emby_prefix", path: "/emby/Items/42/PlaybackInfo"},
		{name: "nested_case", path: "/gateway/EMBY/iTeMs/42/pLaYbAcKiNfO"},
		{name: "vendor_json", path: "/Items/42/PlaybackInfo", contentType: "application/vnd.emby+json; charset=UTF-8"},
		{name: "disabled", path: "/Items/42/PlaybackInfo", disabled: true},
		{name: "GET", method: "GET", path: "/Items/42/PlaybackInfo", skipped: true},
		{name: "PUT", method: "PUT", path: "/Items/42/PlaybackInfo", skipped: true},
		{name: "DELETE", method: "DELETE", path: "/Items/42/PlaybackInfo", skipped: true},
		{name: "other_api", path: "/Users", skipped: true},
		{name: "empty_id", path: "/Items//PlaybackInfo", skipped: true},
		{name: "wrong_group", path: "/Other/42/PlaybackInfo", skipped: true},
		{name: "suffix", path: "/Items/42/PlaybackInfo/Other", skipped: true},
		{name: "trailing_slash", path: "/Items/42/PlaybackInfo/", skipped: true},
	} {
		for _, chunked := range []bool{false, true} {
			t.Run(test.name+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				logs := captureEmbyProxyLog(t)
				contentType := test.contentType
				if contentType == "" {
					contentType = "application/json; charset=utf-8"
				}
				header := http.Header{
					"Content-Type": {contentType}, "Content-Encoding": {"identity"},
					"Access-Control-Allow-Origin": {"https://web.example"}, "Access-Control-Allow-Credentials": {"true"},
					"Access-Control-Expose-Headers": {"Content-Length"}, "Vary": {"Origin", "Accept-Encoding"},
					"X-Upstream": {"unchanged", "second"}, "Cache-Control": {"private, no-cache"}, "Set-Cookie": {"synthetic=value"},
				}
				proxy, captured := newEmbyDirectPlayResponseTestProxy(t, !test.disabled, input, header, chunked)
				method := test.method
				if method == "" {
					method = http.MethodPost
				}
				response, actual, upstream := sendEmbyDirectPlayResponseRequest(t, proxy, captured, method, test.path, requestBody, chunked, "identity")
				changed := !test.disabled && !test.skipped
				if changed {
					assertEmbyDirectPlayResponseJSON(t, actual, input)
					assertEmbyDirectPlayProfile(t, upstream, "200000000")
					if !strings.Contains(logs.String(), "强制直接播放已改写播放信息响应") || !strings.Contains(logs.String(), `"reason":"is_remote_false"`) {
						t.Fatal("实际反代未记录改写成功及非远程源警告")
					}
				} else {
					if string(actual) != input || string(upstream.body) != requestBody || strings.Contains(logs.String(), "强制直接播放") {
						t.Fatal("关闭或未命中时请求、响应必须零改动且不输出改写日志")
					}
					if chunked && (upstream.contentLength != -1 || upstream.lengthHeader != "" || !reflect.DeepEqual(upstream.transfer, []string{"chunked"})) {
						t.Fatal("未命中时请求 chunked framing 被改变")
					}
					if !chunked && (upstream.contentLength != int64(len(requestBody)) || upstream.lengthHeader != strconv.Itoa(len(requestBody)) || len(upstream.transfer) != 0) {
						t.Fatal("未命中时请求长度被改变")
					}
				}
				assertEmbyDirectPlayResponseFraming(t, response, actual, chunked && !changed)
				for name, values := range header {
					if !reflect.DeepEqual(response.Header.Values(name), values) {
						t.Errorf("响应头 %s 未保留", name)
					}
				}
				path, query, _ := strings.Cut(test.path, "?")
				if upstream.path != path || upstream.query != query || upstream.method != method || upstream.token != "private-header-token" {
					t.Fatal("既有 URL、方法或身份头行为被改变")
				}
			})
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseFailOpenIntegration(t *testing.T) {
	for _, test := range []struct{ name, input, contentType, encoding, reason string }{
		{"empty", "", "application/json", "", "empty_body"},
		{"invalid", ` {"MediaSources":[{broken`, "application/json", "", "invalid_json"},
		{"missing", ` {"PlaySessionId":"private-body-token"} `, "application/json", "", "missing_media_sources"},
		{"empty_sources", ` {"MediaSources":[]} `, "application/json", "", "empty_media_sources"},
		{"invalid_shape", `{"MediaSources":{}}`, "application/json", "", "invalid_media_sources"},
		{"late_invalid_source", ` {"MediaSources":[{"IsRemote":false,"SupportsDirectStream":true,"DirectStreamUrl":"private-url-token"},null]} `, "application/json", "", "invalid_media_source"},
		{"non_json", `{"MediaSources":[{}]}`, "text/plain", "", "non_json_response"},
		{"encoded_gzip", `{"MediaSources":[{}]}`, "application/json", "gzip", "encoded_body"},
		{"encoded_br", `{"MediaSources":[{}]}`, "application/json", "br", "encoded_body"},
	} {
		for _, chunked := range []bool{false, true} {
			t.Run(test.name+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				logs := captureEmbyProxyLog(t)
				header := http.Header{"Content-Type": {test.contentType}, "Access-Control-Allow-Origin": {"https://web.example"}, "X-Upstream": {"unchanged"}}
				if test.encoding != "" {
					header.Set("Content-Encoding", test.encoding)
				}
				proxy, captured := newEmbyDirectPlayResponseTestProxy(t, true, test.input, header, chunked)
				response, actual, _ := sendEmbyDirectPlayResponseRequest(t, proxy, captured, http.MethodPost, "/Items/42/PlaybackInfo?api_key=private-query-token", `{"DeviceProfile":{}}`, false, "identity")
				if string(actual) != test.input {
					t.Fatal("实际反代 fail-open 改变了原始响应字节")
				}
				assertEmbyDirectPlayResponseFraming(t, response, actual, chunked)
				for name, values := range header {
					if !reflect.DeepEqual(response.Header.Values(name), values) {
						t.Errorf("fail-open 响应头 %s 改变", name)
					}
				}
				if !strings.Contains(logs.String(), `"reason":"`+test.reason+`"`) || strings.Contains(logs.String(), "已改写播放信息响应") || strings.Contains(logs.String(), "is_remote_false") {
					t.Fatal("实际反代 fail-open 缺少原因或误报成功/部分改写")
				}
				for _, secret := range []string{"private-header-token", "private-query-token", "private-body-token", "private-url-token"} {
					if strings.Contains(logs.String(), secret) {
						t.Errorf("实际反代日志泄漏 %s", secret)
					}
				}
			})
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseSizeIntegration(t *testing.T) {
	for _, size := range []int{embyDirectPlayBodyLimit, embyDirectPlayBodyLimit + 1, embyDirectPlayBodyLimit + 1024} {
		for _, chunked := range []bool{false, true} {
			t.Run(strconv.Itoa(size)+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				prefix, suffix := `{"MediaSources":[{}],"Padding":"`, `"}`
				input := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
				proxy, captured := newEmbyDirectPlayResponseTestProxy(t, true, input, http.Header{"Content-Type": {"application/json"}}, chunked)
				response, actual, _ := sendEmbyDirectPlayResponseRequest(t, proxy, captured, http.MethodPost, "/Items/42/PlaybackInfo", `{"DeviceProfile":{}}`, false, "identity")
				if size == embyDirectPlayBodyLimit {
					assertEmbyDirectPlayResponseJSON(t, actual, input)
				} else if string(actual) != input {
					t.Fatal("实际反代截断或改写超限响应")
				}
				assertEmbyDirectPlayResponseFraming(t, response, actual, chunked && size > embyDirectPlayBodyLimit)
			})
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseGzipIntegration(t *testing.T) {
	const input = ` {"MediaSources":[{"SupportsDirectStream":true,"SupportsTranscoding":true,"DirectStreamUrl":"/original.mp4"}]} `
	var encoded bytes.Buffer
	compressor := gzip.NewWriter(&encoded)
	if _, err := io.WriteString(compressor, input); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	for _, explicit := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run("explicit="+strconv.FormatBool(explicit)+"/enabled="+strconv.FormatBool(enabled), func(t *testing.T) {
				logs := captureEmbyProxyLog(t)
				proxy, captured := newEmbyDirectPlayResponseTestProxy(t, enabled, encoded.String(), http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}}, false)
				encoding := ""
				if explicit {
					encoding = "gzip"
				} else {
					transport := &http.Transport{DisableCompression: true}
					proxy.Client().Transport = transport
					t.Cleanup(transport.CloseIdleConnections)
				}
				response, actual, _ := sendEmbyDirectPlayResponseRequest(t, proxy, captured, http.MethodPost, "/Items/42/PlaybackInfo", `{"DeviceProfile":{}}`, false, encoding)
				if explicit {
					if !bytes.Equal(actual, encoded.Bytes()) || response.Header.Get("Content-Encoding") != "gzip" {
						t.Fatal("显式编码响应必须保留压缩字节和编码头")
					}
					assertEmbyDirectPlayResponseFraming(t, response, actual, false)
				} else if string(actual) != input || response.Header.Get("Content-Encoding") != "" {
					t.Fatal("Transport 自动解压行为被改变或解压后被误改写")
				}
				if strings.Contains(logs.String(), "已改写播放信息响应") || (enabled && !strings.Contains(logs.String(), `"reason":"encoded_body"`)) {
					t.Fatal("编码响应日志错误")
				}
			})
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseLifecycleIntegration(t *testing.T) {
	for _, test := range []struct {
		name, input string
		readError   error
		rewritten   bool
	}{
		{name: "success", input: `{"MediaSources":[{}]}`, rewritten: true},
		{name: "invalid", input: ` {"MediaSources":[{},null]} `},
		{name: "read_failure", input: `{"MediaSources":[{}]}` + strings.Repeat(" ", 64<<10), readError: errors.New("private-read-token")},
		{name: "unexpected_eof", input: `{"MediaSources":[{}]}`, readError: io.ErrUnexpectedEOF},
	} {
		for _, chunked := range []bool{false, true} {
			t.Run(test.name+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				logs := captureEmbyProxyLog(t)
				body := &embyDirectPlayTestBody{reader: strings.NewReader(test.input), readError: test.readError, closeErr: errors.New("private-close-token")}
				transport := embyProxyRelayTestTransport(func(request *http.Request) (*http.Response, error) {
					response := &http.Response{StatusCode: http.StatusOK, Request: request, Header: http.Header{"Content-Type": {"application/json"}, "Access-Control-Allow-Origin": {"https://web.example"}}, Body: body, ContentLength: int64(len(test.input))}
					if chunked {
						response.ContentLength = -1
						response.TransferEncoding = []string{"chunked"}
					} else {
						response.Header.Set("Content-Length", strconv.Itoa(len(test.input)))
					}
					return response, nil
				})
				options := domain.DefaultEmbyProxyConfig()
				options.ForceDirectPlay = true
				service, err := NewEmbyProxyServiceWithConfig(nil, &http.Client{Transport: transport}, options)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(service.Close)
				proxy := httptest.NewServer(service.buildHandler(&domain.EmbyServer{BaseURL: "http://synthetic.example"}))
				t.Cleanup(proxy.Close)
				response, err := proxy.Client().Post(proxy.URL+"/Items/42/PlaybackInfo", "application/json", strings.NewReader(`{"DeviceProfile":{}}`))
				if err != nil {
					t.Fatal(err)
				}
				actual, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				proxy.Close()
				if err != nil || body.closes != 1 {
					t.Fatalf("实际反代读取或原始流关闭错误：err=%v closes=%d", err, body.closes)
				}
				if test.rewritten {
					assertEmbyDirectPlayResponseJSON(t, actual, test.input)
					if !strings.Contains(logs.String(), `"reason":"body_close_failed"`) {
						t.Fatal("实际反代缺少关闭失败日志")
					}
				} else if string(actual) != test.input {
					t.Fatal("实际反代读取错误后未完整回放")
				}
				assertEmbyDirectPlayResponseFraming(t, response, actual, chunked && !test.rewritten)
				if response.Header.Get("Access-Control-Allow-Origin") != "https://web.example" || strings.Contains(logs.String(), "private-read-token") || strings.Contains(logs.String(), "private-close-token") {
					t.Fatal("实际反代生命周期处理丢失 CORS 或泄漏日志")
				}
			})
		}
	}
}
