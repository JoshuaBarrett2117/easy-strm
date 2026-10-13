package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"gopkg.in/yaml.v2"
)

type embyDirectPlayCapture struct {
	body          []byte
	contentLength int64
	lengthHeader  string
	transfer      []string
	method, path  string
	query, token  string
}

func newEmbyDirectPlayTestProxy(t *testing.T, enabled bool) (*httptest.Server, <-chan embyDirectPlayCapture) {
	t.Helper()
	captured := make(chan embyDirectPlayCapture, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("读取上游请求失败：%v", err)
		}
		captured <- embyDirectPlayCapture{body: body, contentLength: request.ContentLength, lengthHeader: request.Header.Get("Content-Length"), transfer: request.TransferEncoding, method: request.Method, path: request.URL.Path, query: request.URL.RawQuery, token: request.Header.Get("X-Emby-Token")}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	options := domain.DefaultEmbyProxyConfig()
	if err := yaml.Unmarshal([]byte("force_direct_play: "+strconv.FormatBool(enabled)), &options); err != nil {
		t.Fatal(err)
	}
	service, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	proxy := httptest.NewServer(service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}))
	t.Cleanup(proxy.Close)
	return proxy, captured
}

func sendEmbyDirectPlayRequest(t *testing.T, proxy *httptest.Server, captured <-chan embyDirectPlayCapture, method, path, body string, chunked bool, encoding string) embyDirectPlayCapture {
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
	request.Header.Set("X-Emby-Token", "test-private-token")
	if encoding != "" {
		request.Header.Set("Content-Encoding", encoding)
	}
	response, err := proxy.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("代理响应=%d", response.StatusCode)
	}
	return <-captured
}

func assertEmbyDirectPlayProfile(t *testing.T, captured embyDirectPlayCapture, bitrate string) map[string]json.RawMessage {
	t.Helper()
	var root, profile map[string]json.RawMessage
	if err := json.Unmarshal(captured.body, &root); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(root["DeviceProfile"], &profile); err != nil {
		t.Fatal(err)
	}
	for field, expected := range map[string]string{"EnableTranscoding": "false", "TranscodingProfiles": "[]", "EnableDirectPlay": "true", "EnableDirectStream": "true", "MaxStreamingBitrate": bitrate} {
		if string(profile[field]) != expected {
			t.Errorf("%s=%s，期望 %s", field, profile[field], expected)
		}
	}
	if captured.contentLength != int64(len(captured.body)) || captured.lengthHeader != strconv.Itoa(len(captured.body)) || len(captured.transfer) != 0 {
		t.Errorf("重写后的 framing 不正确：长度=%d，头=%q，编码=%v，实际=%d", captured.contentLength, captured.lengthHeader, captured.transfer, len(captured.body))
	}
	return root
}

func TestEmbyProxyForceDirectPlayRewrite(t *testing.T) {
	for _, testCase := range []struct {
		name, path, bitrate, expected string
		chunked                       bool
	}{
		{name: "全部字段及低码率", path: "/Items/42/PlaybackInfo", bitrate: "10000000", expected: "200000000"},
		{name: "缺少码率", path: "/emby/Items/42/PlaybackInfo", expected: "200000000"},
		{name: "高码率保留精度", path: "/gateway/api/Items/42/PlaybackInfo", bitrate: "9007199254740993", expected: "9007199254740993"},
		{name: "大小写及等于下限", path: "/EMBY/iTeMs/42/pLaYbAcKiNfO?UserId=test&api_key=test-query-token", bitrate: "200000000", expected: "200000000"},
		{name: "chunked", path: "/Items/42/PlaybackInfo", bitrate: "1e8", expected: "200000000", chunked: true},
		{name: "下限附近小数", path: "/Items/42/PlaybackInfo", bitrate: "199999999.99999999999999", expected: "200000000"},
		{name: "高精度小数", path: "/Items/42/PlaybackInfo", bitrate: "200000000.00000000000001", expected: "200000000.00000000000001"},
		{name: "高码率指数", path: "/Items/42/PlaybackInfo", bitrate: "2.01e8", expected: "2.01e8"},
		{name: "超大指数", path: "/Items/42/PlaybackInfo", bitrate: "1e999999999999999999999999", expected: "1e999999999999999999999999"},
		{name: "超小指数", path: "/Items/42/PlaybackInfo", bitrate: "1e-999999999999999999999999", expected: "200000000"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			proxy, captured := newEmbyDirectPlayTestProxy(t, true)
			bitrateField := ""
			if testCase.bitrate != "" {
				bitrateField = `,"MaxStreamingBitrate":` + testCase.bitrate
			}
			body := `{"DeviceProfile":{"EnableTranscoding":true,"TranscodingProfiles":[{"Type":"Video","Codec":"h264"}],"EnableDirectPlay":false,"EnableDirectStream":false,"Unrelated":{"Large":900719925474099312345,"Decimal":0.123456789012345678901,"Text":"设备<&>"}` + bitrateField + `},"UserId":"test-private-user","Ticks":9007199254740993,"Extra":[null,true,{"Value":1.234567890123456789e25}]}`
			result := sendEmbyDirectPlayRequest(t, proxy, captured, http.MethodPost, testCase.path, body, testCase.chunked, "")
			root := assertEmbyDirectPlayProfile(t, result, testCase.expected)
			var original, originalProfile, profile map[string]json.RawMessage
			_ = json.Unmarshal([]byte(body), &original)
			_ = json.Unmarshal(original["DeviceProfile"], &originalProfile)
			_ = json.Unmarshal(root["DeviceProfile"], &profile)
			for _, field := range []string{"UserId", "Ticks", "Extra"} {
				if !bytes.Equal(root[field], original[field]) {
					t.Errorf("无关字段 %s 改变：%s", field, root[field])
				}
			}
			var expectedUnrelated, actualUnrelated interface{}
			decode := func(raw json.RawMessage, value *interface{}) {
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				if err := decoder.Decode(value); err != nil {
					t.Fatal(err)
				}
			}
			decode(originalProfile["Unrelated"], &expectedUnrelated)
			decode(profile["Unrelated"], &actualUnrelated)
			expectedJSON, _ := json.Marshal(expectedUnrelated)
			actualJSON, _ := json.Marshal(actualUnrelated)
			if !bytes.Equal(expectedJSON, actualJSON) {
				t.Errorf("DeviceProfile 无关字段改变：%s", actualJSON)
			}
			if result.method != http.MethodPost || result.token != "test-private-token" {
				t.Error("方法或身份头改变")
			}
			path, query, _ := strings.Cut(testCase.path, "?")
			if result.path != path || result.query != query {
				t.Errorf("URL 改变：%s?%s", result.path, result.query)
			}
		})
	}
}

func TestEmbyProxyForceDirectPlayPassthrough(t *testing.T) {
	valid := ` { "DeviceProfile": { "EnableTranscoding": true, "MaxStreamingBitrate": 1 }, "Ticks": 9007199254740993 } `
	for _, testCase := range []struct {
		name, method, path, body, encoding string
		disabled, chunked                  bool
	}{
		{name: "GET Users", method: "GET", path: "/Users", body: valid},
		{name: "POST 非目标", method: "POST", path: "/Items/42/Other", body: valid},
		{name: "GET PlaybackInfo", method: "GET", path: "/Items/42/PlaybackInfo", body: valid},
		{name: "PUT PlaybackInfo", method: "PUT", path: "/Items/42/PlaybackInfo", body: valid},
		{name: "无ID", method: "POST", path: "/Items//PlaybackInfo", body: valid},
		{name: "错误分组", method: "POST", path: "/Videos/42/PlaybackInfo", body: valid},
		{name: "后缀多余路径", method: "POST", path: "/Items/42/PlaybackInfo/Other", body: valid},
		{name: "尾斜杠", method: "POST", path: "/Items/42/PlaybackInfo/", body: valid},
		{name: "非法JSON", body: `{"DeviceProfile":{broken`},
		{name: "空请求"},
		{name: "空白", body: " \n\t "},
		{name: "多JSON", body: `{"DeviceProfile":{}} {}`},
		{name: "根数组", body: `[]`},
		{name: "根null", body: `null`},
		{name: "无DeviceProfile", body: ` { "UserId": "test-private-user" } `},
		{name: "null DeviceProfile", body: `{"DeviceProfile":null}`},
		{name: "数组DeviceProfile", body: `{"DeviceProfile":[]}`},
		{name: "字符串DeviceProfile", body: `{"DeviceProfile":"bad"}`},
		{name: "bool DeviceProfile", body: `{"DeviceProfile":true}`},
		{name: "非法转码bool", body: `{"DeviceProfile":{"EnableTranscoding":"true"}}`},
		{name: "null bool", body: `{"DeviceProfile":{"EnableDirectPlay":null}}`},
		{name: "非法直播bool", body: `{"DeviceProfile":{"EnableDirectStream":1}}`},
		{name: "非法转码profile", body: `{"DeviceProfile":{"TranscodingProfiles":{}}}`},
		{name: "null转码profile", body: `{"DeviceProfile":{"TranscodingProfiles":null}}`},
		{name: "码率字符串", body: `{"DeviceProfile":{"MaxStreamingBitrate":"200000000"}}`},
		{name: "码率null", body: `{"DeviceProfile":{"MaxStreamingBitrate":null}}`},
		{name: "码率bool", body: `{"DeviceProfile":{"MaxStreamingBitrate":true}}`},
		{name: "关闭", body: valid, disabled: true},
		{name: "关闭chunked", body: valid, disabled: true, chunked: true},
		{name: "压缩跳过", body: valid, encoding: "gzip"},
		{name: "失败chunked", body: `{"DeviceProfile":null}`, chunked: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			proxy, captured := newEmbyDirectPlayTestProxy(t, !testCase.disabled)
			method, path := testCase.method, testCase.path
			if method == "" {
				method = http.MethodPost
			}
			if path == "" {
				path = "/Items/42/PlaybackInfo"
			}
			result := sendEmbyDirectPlayRequest(t, proxy, captured, method, path, testCase.body, testCase.chunked, testCase.encoding)
			if string(result.body) != testCase.body {
				t.Fatal("fail-open 或跳过请求字节改变")
			}
			if testCase.chunked {
				if result.contentLength != -1 || result.lengthHeader != "" || len(result.transfer) != 1 || result.transfer[0] != "chunked" {
					t.Errorf("chunked framing 改变：%+v", result)
				}
			} else if result.contentLength != int64(len(testCase.body)) || len(result.transfer) != 0 || (len(testCase.body) > 0 && result.lengthHeader != strconv.Itoa(len(testCase.body))) {
				t.Error("原请求长度或 framing 改变")
			}
		})
	}
}

func TestEmbyProxyForceDirectPlaySizeBoundary(t *testing.T) {
	const limit = 8 << 20
	for _, size := range []int{limit, limit + 1, limit + 1024} {
		for _, chunked := range []bool{false, true} {
			t.Run(strconv.Itoa(size)+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				proxy, captured := newEmbyDirectPlayTestProxy(t, true)
				prefix, suffix := `{"DeviceProfile":{},"Padding":"`, `"}`
				body := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
				result := sendEmbyDirectPlayRequest(t, proxy, captured, http.MethodPost, "/Items/42/PlaybackInfo", body, chunked, "")
				if size == limit {
					root := assertEmbyDirectPlayProfile(t, result, "200000000")
					if len(root["Padding"]) != size-len(prefix)-len(suffix)+2 {
						t.Fatal("边界请求丢失数据")
					}
					return
				}
				if string(result.body) != body {
					t.Fatalf("超限请求被截断：收到 %d，原始 %d", len(result.body), len(body))
				}
				if chunked && (result.contentLength != -1 || result.lengthHeader != "" || len(result.transfer) != 1) {
					t.Fatal("超限 chunked 请求 framing 改变")
				}
				if !chunked && (result.contentLength != int64(size) || result.lengthHeader != strconv.Itoa(size)) {
					t.Fatal("超限请求原长度改变")
				}
			})
		}
	}
}
