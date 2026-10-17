package service

import (
	"bytes"
	"encoding/json"
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

func TestEmbyProxyForceDirectPlayResponseRegression(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 强制直接播放会先回查媒体类型；这里把测试 item 一律标为远程（STRM）。
		if request.Method == http.MethodGet && request.URL.Query().Get("Ids") != "" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"Items":[{"Id":"42","Path":"/media/video.strm","MediaSources":[{"Id":"s1","Path":"/media/video.strm"}]}]}`)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"PlaySessionId":"session","MediaSources":[{"IsRemote":true,"SupportsDirectPlay":false,"SupportsDirectStream":true,"SupportsTranscoding":true,"DirectStreamUrl":"/original.mp4","TranscodingUrl":"/master.m3u8"}]}`)
	}))
	t.Cleanup(upstream.Close)
	options := domain.DefaultEmbyProxyConfig()
	options.ForceDirectPlay = true
	service, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	proxy := httptest.NewServer(service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}))
	t.Cleanup(proxy.Close)
	response, err := proxy.Client().Post(proxy.URL+"/Items/42/PlaybackInfo", "application/json", strings.NewReader(`{"DeviceProfile":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		MediaSources []map[string]json.RawMessage
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.MediaSources) != 1 {
		t.Fatal("MediaSources 丢失")
	}
	source := result.MediaSources[0]
	for field, expected := range map[string]string{"SupportsDirectPlay": "true", "SupportsDirectStream": "false", "SupportsTranscoding": "false", "IsRemote": "true"} {
		if string(source[field]) != expected {
			t.Errorf("%s=%s，期望 %s", field, source[field], expected)
		}
	}
	for _, field := range []string{"DirectStreamUrl", "TranscodingUrl"} {
		if _, exists := source[field]; exists {
			t.Errorf("%s 未删除", field)
		}
	}
}

func newEmbyDirectPlayResponse(request *http.Request, body io.ReadCloser) *http.Response {
	return &http.Response{
		Request: request, Body: body, StatusCode: http.StatusMultiStatus, ContentLength: -1,
		TransferEncoding: []string{"chunked"},
		Header: http.Header{
			"Content-Type": {"application/json; charset=utf-8"}, "Content-Length": {"stale"}, "Transfer-Encoding": {"chunked"},
			"Access-Control-Allow-Origin": {"https://web.example"}, "Access-Control-Allow-Credentials": {"true"},
			"Access-Control-Expose-Headers": {"Content-Length"}, "Vary": {"Origin", "Accept-Encoding"},
			"X-Upstream": {"unchanged", "second"}, "Set-Cookie": {"synthetic=value"},
		},
	}
}

func assertEmbyDirectPlayResponseJSON(t *testing.T, actual []byte, original string) {
	t.Helper()
	var expected, result map[string]json.RawMessage
	if err := json.Unmarshal([]byte(original), &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(actual, &result); err != nil {
		t.Fatal(err)
	}
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(expected["MediaSources"], &sources); err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		source["SupportsDirectStream"] = json.RawMessage("false")
		source["SupportsTranscoding"] = json.RawMessage("false")
		source["SupportsDirectPlay"] = json.RawMessage("true")
		delete(source, "DirectStreamUrl")
		delete(source, "TranscodingUrl")
	}
	expectedSources, err := json.Marshal(sources)
	if err != nil {
		t.Fatal(err)
	}
	expected["MediaSources"] = expectedSources
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(body []byte) interface{} {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value interface{}
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if !reflect.DeepEqual(decode(actual), decode(expectedJSON)) {
		t.Fatal("改写结果不符合规则或改变了无关值")
	}
}

func TestEmbyProxyForceDirectPlayResponseRewrite(t *testing.T) {
	for _, input := range []string{
		` { "PlaySessionId":"session", "Ticks":900719925474099312345678901, "Unknown":{"Decimal":0.123456789012345678901,"Exponent":1.234567890123456789e25,"Text":"媒体<&>","Null":null}, "MediaSources":[{"Id":"first","Name":"媒体","Container":"mp4","Protocol":"Http","Path":"https://synthetic.example/direct-link?path=a&api_key=private","IsRemote":true,"SupportsDirectStream":true,"SupportsTranscoding":true,"SupportsDirectPlay":false,"DirectStreamUrl":"/original.mp4","TranscodingUrl":"/master.m3u8","MediaStreams":[{"Codec":"h264","BitRate":9007199254740993}],"Unknown":[null,true,{"Large":999999999999999999999999999}]},{"Id":"second","IsRemote":false},{"Id":"third","Unknown":false}] } `,
		`{"MediaSources":[{}],"HugeExponent":1e999999999999999999999,"NegativeZero":-0.00000}`,
		`{"MediaSources":[{"IsRemote":false,"SupportsDirectStream":false,"SupportsTranscoding":false,"SupportsDirectPlay":true}]}`,
		`{"MediaSources":[{"IsRemote":null,"SupportsDirectStream":null,"SupportsTranscoding":"old","SupportsDirectPlay":0,"DirectStreamUrl":null,"TranscodingUrl":""}]}`,
	} {
		original := &embyDirectPlayTestBody{reader: strings.NewReader(input)}
		response := newEmbyDirectPlayResponse(httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil), original)
		header := response.Header.Clone()
		forceEmbyDirectPlayResponse(response)
		actual, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		assertEmbyDirectPlayResponseJSON(t, actual, input)
		header.Set("Content-Length", strconv.Itoa(len(actual)))
		header.Del("Transfer-Encoding")
		if !reflect.DeepEqual(response.Header, header) || response.StatusCode != http.StatusMultiStatus || response.ContentLength != int64(len(actual)) || response.TransferEncoding != nil {
			t.Fatal("改写后长度、framing、状态或无关响应头错误")
		}
		if err := response.Body.Close(); err != nil || original.closes != 1 {
			t.Fatal("成功改写必须仅关闭原始 body 一次")
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseAtomicFailOpen(t *testing.T) {
	for _, test := range []struct{ name, input, reason string }{
		{"empty", "", "empty_body"},
		{"whitespace", " \n\t ", "empty_body"},
		{"malformed", `{"MediaSources":[{broken`, "invalid_json"},
		{"trailing", `{"MediaSources":[{}]} {}`, "invalid_json"},
		{"root_array", `[]`, "invalid_json"},
		{"root_null", `null`, "invalid_json"},
		{"root_string", `"text"`, "invalid_json"},
		{"root_bool", `true`, "invalid_json"},
		{"root_number", `42`, "invalid_json"},
		{"missing", ` {"PlaySessionId":"unchanged","Unknown":9007199254740993} `, "missing_media_sources"},
		{"wrong_case", `{"mediaSources":[{}]}`, "missing_media_sources"},
		{"null_sources", `{"MediaSources":null}`, "invalid_media_sources"},
		{"object_sources", `{"MediaSources":{}}`, "invalid_media_sources"},
		{"string_sources", `{"MediaSources":"bad"}`, "invalid_media_sources"},
		{"bool_sources", `{"MediaSources":true}`, "invalid_media_sources"},
		{"number_sources", `{"MediaSources":42}`, "invalid_media_sources"},
		{"empty_sources", ` {"MediaSources":[]} `, "empty_media_sources"},
		{"null_source", `{"MediaSources":[null]}`, "invalid_media_source"},
		{"array_source", `{"MediaSources":[[]]}`, "invalid_media_source"},
		{"bool_source", `{"MediaSources":[true]}`, "invalid_media_source"},
		{"number_source", `{"MediaSources":[1]}`, "invalid_media_source"},
		{"string_source", `{"MediaSources":["bad"]}`, "invalid_media_source"},
		{"late_invalid_source", ` {"MediaSources":[{"IsRemote":false,"SupportsDirectStream":true,"DirectStreamUrl":"private"},null,{"SupportsTranscoding":true}]} `, "invalid_media_source"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rewritten, changed, localSources, reason := rewriteEmbyDirectPlayResponseJSON([]byte(test.input))
			if reason != test.reason || rewritten != nil || changed != nil || localSources != nil {
				t.Fatalf("原子校验返回错误：reason=%s changed=%v local=%v", reason, changed, localSources)
			}
			original := &embyDirectPlayTestBody{reader: strings.NewReader(test.input)}
			response := newEmbyDirectPlayResponse(httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil), original)
			header := response.Header.Clone()
			forceEmbyDirectPlayResponse(response)
			if original.closes != 0 || response.ContentLength != -1 || !reflect.DeepEqual(response.TransferEncoding, []string{"chunked"}) || !reflect.DeepEqual(response.Header, header) {
				t.Fatal("fail-open 改变了 framing、响应头或提前关闭原始流")
			}
			actual, err := io.ReadAll(response.Body)
			if err != nil || string(actual) != test.input {
				t.Fatal("fail-open 未按字节保留原始响应")
			}
			if err := response.Body.Close(); err != nil || original.closes != 1 {
				t.Fatal("fail-open 未保留原始 closer")
			}
		})
	}
}

func TestEmbyProxyForceDirectPlayResponseUntouchedBodies(t *testing.T) {
	for _, test := range []struct {
		name, method, path, contentType string
		encodings                       []string
		length                          int64
		uncompressed, nilRequest        bool
		nilURL                          bool
	}{
		{name: "GET", method: "GET"}, {name: "PUT", method: "PUT"}, {name: "HEAD", method: "HEAD"}, {name: "DELETE", method: "DELETE"},
		{name: "other", path: "/Items/42/Other"}, {name: "empty_id", path: "/Items//PlaybackInfo"},
		{name: "wrong_group", path: "/Videos/42/PlaybackInfo"}, {name: "extra_path", path: "/Items/42/PlaybackInfo/Other"},
		{name: "trailing_slash", path: "/Items/42/PlaybackInfo/"}, {name: "short_path", path: "/PlaybackInfo"},
		{name: "nil_request", nilRequest: true}, {name: "nil_url", nilURL: true},
		{name: "text", contentType: "text/plain"}, {name: "html", contentType: "text/html"},
		{name: "missing_type", contentType: "missing"}, {name: "invalid_type", contentType: "application/json; broken"},
		{name: "not_json_suffix", contentType: "text/custom+json"},
		{name: "gzip", encodings: []string{"gzip"}}, {name: "br", encodings: []string{"br"}},
		{name: "multiple_encoding", encodings: []string{"identity", "gzip"}}, {name: "combined_encoding", encodings: []string{"identity, gzip"}},
		{name: "transport_decoded", uncompressed: true}, {name: "declared_oversize", length: embyDirectPlayBodyLimit + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			method, path := test.method, test.path
			if method == "" {
				method = http.MethodPost
			}
			if path == "" {
				path = "/Items/42/PlaybackInfo"
			}
			original := &embyDirectPlayTestBody{reader: strings.NewReader(`{"MediaSources":[{}]}`)}
			response := newEmbyDirectPlayResponse(httptest.NewRequest(method, path, nil), original)
			if test.nilRequest {
				response.Request = nil
			} else if test.nilURL {
				response.Request.URL = nil
			}
			if test.contentType == "missing" {
				response.Header.Del("Content-Type")
			} else if test.contentType != "" {
				response.Header.Set("Content-Type", test.contentType)
			}
			if test.encodings != nil {
				response.Header["Content-Encoding"] = test.encodings
			}
			response.Uncompressed = test.uncompressed
			response.ContentLength = test.length
			header := response.Header.Clone()
			forceEmbyDirectPlayResponse(response)
			if response.Body != original || original.reads != 0 || original.closes != 0 || response.ContentLength != test.length || !reflect.DeepEqual(response.Header, header) || !reflect.DeepEqual(response.TransferEncoding, []string{"chunked"}) {
				t.Fatal("跳过响应必须零读取、零替换并保留 framing 和头")
			}
			_ = response.Body.Close()
		})
	}
	for _, body := range []io.ReadCloser{nil, http.NoBody} {
		response := newEmbyDirectPlayResponse(httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil), body)
		forceEmbyDirectPlayResponse(response)
		if response.Body != body || response.ContentLength != -1 {
			t.Fatal("nil 或 NoBody 被替换")
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseBodyLifecycle(t *testing.T) {
	for _, test := range []struct {
		name, input string
		readError   error
		rewritten   bool
	}{
		{name: "success", input: `{"MediaSources":[{}]}`, rewritten: true},
		{name: "invalid", input: `{"MediaSources":null}`},
		{name: "read_failure", input: `{"MediaSources":[{}]}` + strings.Repeat(" ", 64<<10), readError: errors.New("private-read-token")},
		{name: "unexpected_eof", input: `{"MediaSources":[{}]}`, readError: io.ErrUnexpectedEOF},
		{name: "oversize", input: strings.Repeat("x", embyDirectPlayBodyLimit+100)},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := &embyDirectPlayTestBody{reader: strings.NewReader(test.input), readError: test.readError, closeErr: errors.New("private-close-token")}
			response := newEmbyDirectPlayResponse(httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil), original)
			header := response.Header.Clone()
			forceEmbyDirectPlayResponse(response)
			if original.consumed > embyDirectPlayBodyLimit+1 {
				t.Fatal("有界读取超过上限加 sentinel")
			}
			if !test.rewritten && (original.closes != 0 || !reflect.DeepEqual(response.Header, header) || response.ContentLength != -1 || !reflect.DeepEqual(response.TransferEncoding, []string{"chunked"})) {
				t.Fatal("读取失败或超限改变了 framing 或提前关闭")
			}
			actual, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if test.rewritten {
				assertEmbyDirectPlayResponseJSON(t, actual, test.input)
				if original.closes != 1 {
					t.Fatal("成功改写未关闭旧流")
				}
			} else if string(actual) != test.input {
				t.Fatal("读取错误时未回放全部已消费字节和剩余流")
			}
			err = response.Body.Close()
			if original.closes != 1 || (test.rewritten && err != nil) || (!test.rewritten && !errors.Is(err, original.closeErr)) {
				t.Fatal("原始 closer 未保留或被重复关闭")
			}
		})
	}
}

type embyDirectPlayStalledBody struct{ reads, closes int }

func (body *embyDirectPlayStalledBody) Read(buffer []byte) (int, error) {
	body.reads++
	return 0, nil
}

func (body *embyDirectPlayStalledBody) Close() error {
	body.closes++
	return nil
}

func TestEmbyProxyForceDirectPlayResponseNoProgress(t *testing.T) {
	body := &embyDirectPlayStalledBody{}
	response := newEmbyDirectPlayResponse(httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil), body)
	header := response.Header.Clone()
	forceEmbyDirectPlayResponse(response)
	if body.reads != 100 || body.closes != 0 || !reflect.DeepEqual(response.Header, header) || response.ContentLength != -1 {
		t.Fatal("无进展读取必须有界退出且保留原始流")
	}
	if err := response.Body.Close(); err != nil || body.closes != 1 {
		t.Fatal("无进展读取未保留 closer")
	}
}

func TestEmbyProxyForceDirectPlayResponseSizeBoundary(t *testing.T) {
	for _, size := range []int{embyDirectPlayBodyLimit - 1, embyDirectPlayBodyLimit, embyDirectPlayBodyLimit + 1, embyDirectPlayBodyLimit + 1024} {
		for _, known := range []bool{false, true} {
			t.Run(strconv.Itoa(size)+"/known="+strconv.FormatBool(known), func(t *testing.T) {
				prefix, suffix := `{"MediaSources":[{}],"Padding":"`, `"}`
				input := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
				original := &embyDirectPlayTestBody{reader: strings.NewReader(input)}
				response := newEmbyDirectPlayResponse(httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil), original)
				if known {
					response.ContentLength = int64(size)
				}
				header := response.Header.Clone()
				forceEmbyDirectPlayResponse(response)
				if original.consumed > embyDirectPlayBodyLimit+1 || (size > embyDirectPlayBodyLimit && !known && original.consumed != embyDirectPlayBodyLimit+1) || (size > embyDirectPlayBodyLimit && known && original.reads != 0) {
					t.Fatal("未遵守上限加 sentinel 或已知超限零读取")
				}
				actual, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if size <= embyDirectPlayBodyLimit {
					assertEmbyDirectPlayResponseJSON(t, actual, input)
					if response.ContentLength != int64(len(actual)) || response.Header.Get("Content-Length") != strconv.Itoa(len(actual)) || len(response.TransferEncoding) != 0 || response.Header.Get("Transfer-Encoding") != "" {
						t.Fatal("边界成功改写未同步 framing")
					}
				} else if string(actual) != input || !reflect.DeepEqual(response.Header, header) || !reflect.DeepEqual(response.TransferEncoding, []string{"chunked"}) || (known && response.ContentLength != int64(size)) || (!known && response.ContentLength != -1) {
					t.Fatal("超限响应或 framing 未完整透传")
				}
				_ = response.Body.Close()
				if original.closes != 1 {
					t.Fatal("边界响应原始流关闭次数错误")
				}
			})
		}
	}
}

func TestEmbyProxyForceDirectPlayResponseLogging(t *testing.T) {
	logs := captureEmbyProxyLog(t)
	request := httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo?api_key=private-query-token", nil)
	request.Header.Set("X-Emby-Token", "private-header-token")
	response := newEmbyDirectPlayResponse(request, io.NopCloser(strings.NewReader(`{"Private":"private-body-token","MediaSources":[{"IsRemote":false,"SupportsDirectStream":true,"SupportsTranscoding":true,"SupportsDirectPlay":false,"DirectStreamUrl":"https://private-url.example/?api_key=private-url-token","TranscodingUrl":"private-transcode-token"},{}]}`)))
	forceEmbyDirectPlayResponse(response)
	_ = response.Body.Close()
	var success, remoteWarnings int
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record struct {
			Level, Module, Message string
			Fields                 struct {
				ItemID        string   `json:"item_id"`
				Reason        string   `json:"reason"`
				SourceIndex   int      `json:"source_index"`
				ChangedFields []string `json:"changed_fields"`
			}
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Level == "INFO" {
			success++
			expected := []string{"MediaSources[0].SupportsDirectStream", "MediaSources[0].SupportsTranscoding", "MediaSources[0].SupportsDirectPlay", "MediaSources[0].DirectStreamUrl", "MediaSources[0].TranscodingUrl", "MediaSources[1].SupportsDirectStream", "MediaSources[1].SupportsTranscoding", "MediaSources[1].SupportsDirectPlay"}
			if record.Fields.ItemID != "42" || record.Module != "emby_proxy" || record.Message != "强制直接播放已改写播放信息响应" || !reflect.DeepEqual(record.Fields.ChangedFields, expected) {
				t.Fatalf("成功日志内容错误：%s", line)
			}
		} else if record.Level == "WARN" && record.Fields.Reason == "is_remote_false" && record.Fields.ItemID == "42" && record.Fields.SourceIndex == 0 {
			remoteWarnings++
		} else {
			t.Fatalf("意外日志：%s", line)
		}
	}
	if success != 1 || remoteWarnings != 1 {
		t.Fatal("应只记录一次成功及一次 IsRemote=false 警告；缺失 IsRemote 不应警告")
	}
	for _, test := range []struct{ input, reason string }{
		{`{"MediaSources":null}`, "invalid_media_sources"}, {`{broken`, "invalid_json"},
		{`{"Private":"private-body-token"}`, "missing_media_sources"}, {"", "empty_body"},
		{`{"MediaSources":[{"IsRemote":false},null]}`, "invalid_media_source"},
	} {
		response := newEmbyDirectPlayResponse(request, io.NopCloser(strings.NewReader(test.input)))
		before := len(logs.String())
		forceEmbyDirectPlayResponse(response)
		_ = response.Body.Close()
		output := logs.String()[before:]
		if !strings.Contains(output, `"reason":"`+test.reason+`"`) || !strings.Contains(output, `"level":"WARN"`) || strings.Contains(output, `"level":"INFO"`) || strings.Contains(output, "is_remote_false") {
			t.Fatalf("fail-open 日志错误：%s", output)
		}
	}
	for _, test := range []struct{ readError, closeError error }{
		{readError: errors.New("private-read-token")}, {closeError: errors.New("private-close-token")},
	} {
		body := &embyDirectPlayTestBody{reader: strings.NewReader(`{"MediaSources":[{}]}`), readError: test.readError, closeErr: test.closeError}
		response := newEmbyDirectPlayResponse(request, body)
		forceEmbyDirectPlayResponse(response)
		_ = response.Body.Close()
	}
	for _, reason := range []string{"body_read_failed", "body_close_failed"} {
		if !strings.Contains(logs.String(), `"reason":"`+reason+`"`) {
			t.Errorf("缺少 %s 日志", reason)
		}
	}
	for _, secret := range []string{"private-query-token", "private-header-token", "private-body-token", "private-url.example", "private-url-token", "private-transcode-token", "private-read-token", "private-close-token"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("日志泄漏 %s", secret)
		}
	}
}
