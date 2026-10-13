package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type embyDirectPlayTestBody struct {
	reader    *strings.Reader
	readError error
	closeErr  error
	reads     int
	consumed  int
	closes    int
}

func (body *embyDirectPlayTestBody) Read(buffer []byte) (int, error) {
	body.reads++
	count, err := body.reader.Read(buffer)
	body.consumed += count
	if body.readError != nil {
		failure := body.readError
		body.readError = nil
		return count, failure
	}
	return count, err
}

func (body *embyDirectPlayTestBody) Close() error {
	body.closes++
	return body.closeErr
}

func TestEmbyProxyForceDirectPlayBodyLifecycle(t *testing.T) {
	for _, testCase := range []struct {
		name, input string
		readError   error
		rewritten   bool
	}{
		{name: "成功", input: `{"DeviceProfile":{}}`, rewritten: true},
		{name: "失败", input: `{"DeviceProfile":null}`},
		{name: "读取失败", input: `{"DeviceProfile":{}}` + strings.Repeat(" ", 64<<10), readError: errors.New("private-read-error-token")},
		{name: "意外EOF", input: `{"DeviceProfile":{}}`, readError: io.ErrUnexpectedEOF},
		{name: "超限", input: strings.Repeat("x", (8<<20)+100)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			original := &embyDirectPlayTestBody{reader: strings.NewReader(testCase.input), readError: testCase.readError, closeErr: errors.New("test close failure")}
			request := httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", original)
			request.ContentLength = -1
			request.TransferEncoding = []string{"chunked"}
			request.Header.Set("Transfer-Encoding", "chunked")
			request.Header.Set("Content-Length", "stale")
			getBodyCalls := 0
			request.GetBody = func() (io.ReadCloser, error) {
				getBodyCalls++
				return io.NopCloser(strings.NewReader(testCase.input)), nil
			}
			forceEmbyDirectPlay(request)
			if original.consumed > (8<<20)+1 {
				t.Fatal("读取超过检测字节边界")
			}
			if !testCase.rewritten {
				if original.closes != 0 || request.ContentLength != -1 || request.Header.Get("Content-Length") != "stale" || request.Header.Get("Transfer-Encoding") != "chunked" || !reflect.DeepEqual(request.TransferEncoding, []string{"chunked"}) {
					t.Fatal("失败回放改变原始 framing 或提前关闭")
				}
			} else if original.closes != 1 || len(request.TransferEncoding) != 0 || request.Header.Get("Transfer-Encoding") != "" {
				t.Fatal("成功重写未关闭旧流或清除 transfer encoding")
			}
			result, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !testCase.rewritten && string(result) != testCase.input {
				t.Fatalf("读取失败或超限后未完整回放：%d / %d", len(result), len(testCase.input))
			}
			if testCase.rewritten && (request.ContentLength != int64(len(result)) || request.Header.Get("Content-Length") != strconv.Itoa(len(result))) {
				t.Fatal("成功重写长度未同步")
			}
			replay, err := request.GetBody()
			if err != nil {
				t.Fatal(err)
			}
			replayBytes, err := io.ReadAll(replay)
			_ = replay.Close()
			if err != nil || !bytes.Equal(result, replayBytes) || (testCase.rewritten && getBodyCalls != 0) || (!testCase.rewritten && getBodyCalls != 1) {
				t.Fatal("GetBody 未保持正确重放内容")
			}
			err = request.Body.Close()
			if original.closes != 1 || (!testCase.rewritten && !errors.Is(err, original.closeErr)) {
				t.Fatal("原始 closer 未被保留或重复关闭")
			}
		})
	}
}

func TestEmbyProxyForceDirectPlayUntouchedBodies(t *testing.T) {
	for _, testCase := range []struct{ method, path, encoding string }{
		{"GET", "/Items/42/PlaybackInfo", ""},
		{"POST", "/Users", ""},
		{"POST", "/Items//PlaybackInfo", ""},
		{"POST", "/Items/42/PlaybackInfo", "gzip"},
	} {
		original := &embyDirectPlayTestBody{reader: strings.NewReader(`{"DeviceProfile":{}}`)}
		request := httptest.NewRequest(testCase.method, testCase.path, original)
		request.Header.Set("Content-Encoding", testCase.encoding)
		forceEmbyDirectPlay(request)
		if request.Body != original || original.reads != 0 || original.closes != 0 {
			t.Fatalf("跳过请求被读取或替换：%+v", testCase)
		}
	}
}

func TestEmbyProxyForceDirectPlayLogging(t *testing.T) {
	logs := captureEmbyProxyLog(t)
	proxy, captured := newEmbyDirectPlayTestProxy(t, true)
	sendEmbyDirectPlayRequest(t, proxy, captured, http.MethodPost, "/emby/Items/42/PlaybackInfo?api_key=test-query-token", `{"DeviceProfile":{},"Private":"test-private-body"}`, false, "")
	output := logs.String()
	for _, expected := range []string{`"level":"INFO"`, `"module":"emby_proxy"`, `"item_id":"42"`, `"changed_fields":["EnableTranscoding","TranscodingProfiles","EnableDirectPlay","EnableDirectStream","MaxStreamingBitrate"]`} {
		if !strings.Contains(output, expected) {
			t.Errorf("成功日志缺少 %s", expected)
		}
	}
	sendEmbyDirectPlayRequest(t, proxy, captured, http.MethodPost, "/Items/invalid/PlaybackInfo", `{"Private":"test-private-body"}`, false, "")
	output = logs.String()
	for _, expected := range []string{`"level":"WARN"`, `"item_id":"invalid"`, `"reason":"missing_device_profile"`} {
		if !strings.Contains(output, expected) {
			t.Errorf("失败日志缺少 %s", expected)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/Items/read-error/PlaybackInfo", &embyDirectPlayTestBody{reader: strings.NewReader("prefix"), readError: errors.New("private-read-error-token")})
	forceEmbyDirectPlay(request)
	_ = request.Body.Close()
	output = logs.String()
	if !strings.Contains(output, `"reason":"body_read_failed"`) {
		t.Error("读取失败缺少原因日志")
	}
	for _, secret := range []string{"test-private-token", "test-query-token", "test-private-body", "private-read-error-token"} {
		if strings.Contains(output, secret) {
			t.Errorf("日志泄漏 %s", secret)
		}
	}
}

func TestEmbyProxyForceDirectPlayMultipleContentEncodings(t *testing.T) {
	original := &embyDirectPlayTestBody{reader: strings.NewReader(`{"DeviceProfile":{}}`)}
	request := httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", original)
	request.Header.Add("Content-Encoding", "identity")
	request.Header.Add("Content-Encoding", "gzip")
	forceEmbyDirectPlay(request)
	if request.Body != original || original.reads != 0 || original.closes != 0 {
		t.Fatal("多值 Content-Encoding 必须跳过且不读取原始流")
	}
}

func TestEmbyProxyForceDirectPlayNoGetBody(t *testing.T) {
	for _, body := range []io.ReadCloser{nil, http.NoBody, io.NopCloser(strings.NewReader(`{"DeviceProfile":{}}`))} {
		request := httptest.NewRequest(http.MethodPost, "/Items/42/PlaybackInfo", nil)
		request.Body = body
		forceEmbyDirectPlay(request)
		if request.GetBody != nil {
			t.Fatal("未提供 GetBody 的请求不应改变重试能力")
		}
		if body == nil || body == http.NoBody {
			if request.Body != body {
				t.Fatal("空 Body 被替换")
			}
			continue
		}
		_ = request.Body.Close()
	}
}
