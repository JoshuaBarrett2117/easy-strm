package service

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestEmbyProxyPlayback(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, location string
		upstreamStatus, status             int
		stream                             bool
	}{
		{"STRM", "GET", "/Videos/42/stream?Static=true", `{"MediaSources":[{"Id":"s1","Path":"https://cdn.example/video?sign=123"}]}`, "https://cdn.example/video?sign=123", 200, 302, false},
		{"HEAD前缀", "HEAD", "/emby/Videos/42/stream.mp4?Static=true", `{"MediaSources":[{"Path":"http://strm.example/direct-link?id=1"}]}`, "http://strm.example/direct-link?id=1", 200, 302, false},
		{"多版本选中源", "GET", "/Videos/42/stream?MediaSourceId=s2", `{"MediaSources":[{"Id":"s1","Path":"/media/1.mkv"},{"Id":"s2","Path":"https://cdn.example/2"}]}`, "https://cdn.example/2", 200, 302, false},
		{"本地文件", "GET", "/Videos/42/stream", `{"MediaSources":[{"Path":"/media/movie.mkv","Protocol":"File"}]}`, "", 200, 206, true},
		{"非HTTP远程", "GET", "/Videos/42/stream", `{"MediaSources":[{"Path":"rtsp://example/video","Protocol":"Rtsp","IsRemote":true}]}`, "", 200, 206, true},
		{"转码", "GET", "/Videos/42/stream?Static=false", `{"MediaSources":[{"Path":"/media/movie.mkv","Protocol":"File"}]}`, "", 200, 206, true},
		{"编码转码", "GET", "/Videos/42/stream?VideoCodec=h264", `{"MediaSources":[{"Path":"/media/movie.mkv","Protocol":"File"}]}`, "", 200, 206, true},
		{"HLS", "GET", "/Videos/42/master.m3u8", `{"MediaSources":[{"Path":"/media/movie.mkv","Protocol":"File"}]}`, "", 200, 206, true},
		{"远程地址无效", "GET", "/Videos/42/stream", `{"MediaSources":[{"Path":"http:///invalid","IsRemote":true,"Protocol":"Http"}]}`, "", 200, 502, false},
		{"远程源缺地址", "GET", "/Videos/42/stream", `{"MediaSources":[{"IsRemote":true}]}`, "", 200, 502, false},
		{"远程源解析失败", "GET", "/Videos/42/stream", `{"MediaSources":[{"IsRemote":true,"Path":"bad%path"}]}`, "", 200, 502, false},
		{"错误版本", "GET", "/Videos/42/stream?MediaSourceId=missing", `{"MediaSources":[{"Id":"s1","Path":"/media/1.mkv"}]}`, "", 200, 502, false},
		{"未选版本", "GET", "/Videos/42/stream", `{"MediaSources":[{"Id":"s1"},{"Id":"s2"}]}`, "", 200, 502, false},
		{"解析失败", "GET", "/Videos/42/stream", `{bad`, "", 200, 502, false},
		{"没有媒体源", "GET", "/Videos/42/stream", `{"MediaSources":[]}`, "", 200, 502, false},
		{"上游失败", "GET", "/Videos/42/stream", "", "", 500, 502, false},
		{"拒绝授权", "GET", "/Videos/42/stream", "", "", 403, 403, false},
		{"上游重定向", "GET", "/Videos/42/stream", "", "", 302, 502, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var streamCalls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
					want := "/Items/42/PlaybackInfo"
					if strings.HasPrefix(tc.path, "/emby/") {
						want = "/emby" + want
					}
					if r.URL.Path != want || r.Method != "GET" {
						t.Errorf("PlaybackInfo 请求=%s %s", r.Method, r.URL.Path)
					}
					if r.Header.Get("X-Emby-Token") != "secret-key" {
						t.Error("缺少 API Key 兜底")
					}
					w.WriteHeader(tc.upstreamStatus)
					_, _ = io.WriteString(w, tc.body)
					return
				}
				if r.URL.Path == "/Items" {
					if r.URL.Query().Get("Ids") != "42" || r.URL.Query().Get("Fields") != "Path,MediaSources" {
						t.Errorf("未查询原项目类型: %s", r.URL.Path)
					}
					_, _ = io.WriteString(w, `{"Items":[{"Id":"42","Path":"/media/movie.mkv","IsShortcut":false}]}`)
					return
				}
				streamCalls.Add(1)
				w.WriteHeader(206)
				_, _ = io.WriteString(w, "video")
			}))
			defer remote.Close()
			service := NewEmbyProxyService(nil, remote.Client())
			handler := service.buildHandler(&domain.EmbyServer{BaseURL: remote.URL, APIKey: "secret-key"})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status || response.Header().Get("Location") != tc.location {
				t.Fatalf("响应=%d %s, body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
			}
			if tc.location != "" && (response.Header().Get("Cache-Control") != "no-store" || response.Body.Len() != 0) {
				t.Fatal("302 必须禁止缓存且不传输视频")
			}
			if (streamCalls.Load() > 0) != tc.stream {
				t.Fatalf("意外回源次数=%d", streamCalls.Load())
			}
		})
	}
}

func TestEmbyProxyPreservesRequests(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/gateway/emby/Items?api_key=user&search=a%2Fb" {
			t.Errorf("URL=%s", r.URL.RequestURI())
		}
		for key, value := range map[string]string{"Cookie": "session=client", "Authorization": "Bearer user", "X-Emby-Token": "user-token", "Range": "bytes=1-2"} {
			if r.Header.Get(key) != value {
				t.Errorf("%s=%s", key, r.Header.Get(key))
			}
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != "payload" {
			t.Error("方法或请求体未透传")
		}
		w.Header().Set("Set-Cookie", "session=upstream")
		w.Header().Set("Content-Range", "bytes 1-2/4")
		w.WriteHeader(206)
		_, _ = io.WriteString(w, "ok")
	}))
	defer remote.Close()
	s := NewEmbyProxyService(nil, remote.Client())
	h := s.buildHandler(&domain.EmbyServer{BaseURL: remote.URL + "/gateway/emby"})
	r := httptest.NewRequest("POST", "/emby/Items?api_key=user&search=a%2Fb", strings.NewReader("payload"))
	for key, value := range map[string]string{"Cookie": "session=client", "Authorization": "Bearer user", "X-Emby-Token": "user-token", "Range": "bytes=1-2"} {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "ok" || w.Header().Get("Set-Cookie") != "session=upstream" || w.Header().Get("Content-Range") != "bytes 1-2/4" {
		t.Fatalf("透传失败: %v %s", w.Result(), w.Body.String())
	}
}

func TestEmbyProxyUnavailableUpstream(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	remote.Close()
	s := NewEmbyProxyService(nil, &http.Client{Timeout: time.Second})
	h := s.buildHandler(&domain.EmbyServer{BaseURL: remote.URL, APIKey: "key"})
	for _, path := range []string{"/Items", "/Videos/42/stream"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 502 {
			t.Fatalf("上游断开应返回502: %s %d", path, w.Code)
		}
	}
}

func TestEmbyProxyPreservesEscapedBaseAndClientPaths(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/gateway%2Froute/emby/Items/a%2Fb":
			if r.URL.RawQuery != "filter=a%2Fb" {
				t.Errorf("查询被改写: %s", r.URL.RawQuery)
			}
		case "/gateway%2Froute/emby/Items/item%3F1/PlaybackInfo":
			_, _ = io.WriteString(w, `{"MediaSources":[{"Path":"https://cdn.example/video"}]}`)
		default:
			t.Errorf("转义路径被改写: %s", r.URL.EscapedPath())
		}
	}))
	defer remote.Close()
	s := NewEmbyProxyService(nil, remote.Client())
	h := s.buildHandler(&domain.EmbyServer{BaseURL: remote.URL + "/gateway%2Froute/emby", APIKey: "key"})
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/emby/Items/a%2Fb?filter=a%2Fb", 200},
		{"/emby/Videos/item%3F1/stream", 302},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s 状态码=%d, body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestEmbyProxyPlaybackIdentityAndBasePath(t *testing.T) {
	for _, tc := range []struct {
		name, key, value, query string
		fallback                bool
	}{
		{"Token", "X-Emby-Token", "client-token", "", false},
		{"Authorization", "Authorization", `MediaBrowser Client="test", Token="user"`, "", false},
		{"EmbyAuthorization", "X-Emby-Authorization", `MediaBrowser Token="user"`, "", false},
		{"未知Authorization", "Authorization", "custom-user-auth", "", false},
		{"Cookie", "Cookie", "session=user", "", false},
		{"查询Token", "", "", "?api_key=user&UserId=u1", false},
		{"设备描述", "Authorization", `MediaBrowser Client="test", Device="phone"`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/gateway/emby/Items/42/PlaybackInfo" {
					t.Errorf("路径=%s", r.URL.Path)
				}
				if tc.key != "" && r.Header.Get(tc.key) != tc.value {
					t.Errorf("身份未透传")
				}
				if tc.query != "" && (r.URL.Query().Get("api_key") != "user" || r.URL.Query().Get("UserId") != "u1") {
					t.Error("查询身份未透传")
				}
				if (r.Header.Get("X-Emby-Token") == "admin-secret") != tc.fallback {
					t.Error("错误地覆盖客户端身份")
				}
				_, _ = io.WriteString(w, `{"MediaSources":[{"Path":"https://cdn.example/video"}]}`)
			}))
			defer remote.Close()
			s := NewEmbyProxyService(nil, remote.Client())
			h := s.buildHandler(&domain.EmbyServer{BaseURL: remote.URL + "/gateway/emby", APIKey: "admin-secret"})
			r := httptest.NewRequest("GET", "/emby/Videos/42/stream"+tc.query, nil)
			if tc.key != "" {
				r.Header.Set(tc.key, tc.value)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 302 || strings.Contains(w.Header().Get("Location"), "secret") {
				t.Fatalf("响应=%d %v", w.Code, w.Header())
			}
		})
	}
}

func freeEmbyProxyPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestEmbyProxyListenerLifecycle(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "first") }))
	defer remote.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "second") }))
	defer second.Close()
	s := NewEmbyProxyService(nil, nil, 80, 8082)
	defer s.Close()
	input := domain.EmbyServer{ID: 1, Name: "test", BaseURL: remote.URL, Enabled: true, ProxyPort: freeEmbyProxyPort(t)}
	save := func(failure bool) error {
		_, err := s.save(&input, func() (*domain.EmbyServer, error) {
			if failure {
				return nil, errors.New("DB failed")
			}
			c := input
			return &c, nil
		})
		return err
	}
	get := func(port int) string {
		t.Helper()
		client := &http.Client{Timeout: time.Second}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/Items", port))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	assertClosed := func(port int) {
		t.Helper()
		l, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			t.Fatalf("端口未释放: %d %v", port, err)
		}
		_ = l.Close()
	}
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	firstPort := input.ProxyPort
	if get(firstPort) != "first" {
		t.Fatal("初始代理失败")
	}
	input.BaseURL = second.URL
	if err := save(true); err == nil {
		t.Fatal("应返回保存失败")
	}
	if get(firstPort) != "first" {
		t.Fatal("失败覆盖了原代理")
	}
	if err := save(false); err != nil || get(firstPort) != "second" {
		t.Fatalf("同端口配置更新失败: %v", err)
	}
	input.ProxyPort = freeEmbyProxyPort(t)
	if err := save(true); err == nil {
		t.Fatal("应返回保存失败")
	}
	assertClosed(input.ProxyPort)
	if get(firstPort) != "second" {
		t.Fatal("变更失败关闭了旧端口")
	}
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	assertClosed(firstPort)
	newPort := input.ProxyPort
	if get(newPort) != "second" {
		t.Fatal("新端口代理失败")
	}
	input.Enabled = false
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	assertClosed(newPort)
	input.Enabled = true
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	input.ProxyPort = 0
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	assertClosed(newPort)
	input.ProxyPort = newPort
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	if err := s.delete(1, func() error { return errors.New("delete failed") }); err == nil || get(newPort) != "second" {
		t.Fatal("删除失败应保留代理")
	}
	if err := s.delete(1, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	assertClosed(newPort)
	if err := save(false); err != nil {
		t.Fatal(err)
	}
	s.Close()
	assertClosed(newPort)
}

func TestEmbyProxyRejectsPortsBeforeSave(t *testing.T) {
	occupied, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	s := NewEmbyProxyService(nil, nil, 80, 8082)
	defer s.Close()
	for _, port := range []int{-1, 65536, 80, 8082, occupied.Addr().(*net.TCPAddr).Port} {
		called := false
		_, err := s.save(&domain.EmbyServer{Name: "test", BaseURL: "http://localhost", ProxyPort: port}, func() (*domain.EmbyServer, error) { called = true; return nil, nil })
		if err == nil || called {
			t.Fatalf("无效端口 %d 触发了保存", port)
		}
	}
	for _, port := range []int{0, freeEmbyProxyPort(t)} {
		input := &domain.EmbyServer{ID: 1, Name: "test", BaseURL: "http://localhost", ProxyPort: port, Enabled: false}
		if _, err := s.save(input, func() (*domain.EmbyServer, error) { return input, nil }); err != nil {
			t.Fatal(err)
		}
	}
	input := &domain.EmbyServer{ID: 1, Name: "test", BaseURL: "http://localhost", ProxyPort: freeEmbyProxyPort(t), Enabled: true}
	if _, err := s.save(input, func() (*domain.EmbyServer, error) { return input, nil }); err != nil {
		t.Fatal(err)
	}
	other := *input
	other.ID = 2
	if _, err := s.save(&other, func() (*domain.EmbyServer, error) { t.Fatal("重复端口不能保存"); return nil, nil }); err == nil {
		t.Fatal("实例端口冲突未拒绝")
	}
}

func TestEmbyProxyStartLoadsEnabledInstances(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	port := freeEmbyProxyPort(t)
	rows := sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time", "proxy_port"})
	now := time.Now()
	rows.AddRow(1, "enabled", "http://localhost", "key", true, true, now, now, port)
	rows.AddRow(2, "disabled", "http://localhost", "key", false, false, now, now, freeEmbyProxyPort(t))
	rows.AddRow(3, "closed", "http://localhost", "key", true, false, now, now, 0)
	rows.AddRow(4, "conflict", "http://localhost", "key", true, false, now, now, 8082)
	mock.ExpectQuery("SELECT id, name, base_url").WillReturnRows(rows)
	s := NewEmbyProxyService(dao.NewEmbyServerDAO(db), nil, 8082)
	defer s.Close()
	if err := s.Start(); err == nil {
		t.Fatal("应报告冲突")
	}
	if len(s.listeners) != 1 || s.listeners[1].port != port {
		t.Fatal("未按启用状态恢复监听")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyProxyWebSocket(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" || r.Header.Get("X-Emby-Token") != "user" {
			t.Error("升级头未透传")
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\nhello")
		_, _ = io.Copy(io.Discard, conn)
	}))
	defer remote.Close()
	s := NewEmbyProxyService(nil, remote.Client())
	defer s.Close()
	input := &domain.EmbyServer{ID: 1, Name: "test", BaseURL: remote.URL, Enabled: true, ProxyPort: freeEmbyProxyPort(t)}
	if _, err := s.save(input, func() (*domain.EmbyServer, error) { return input, nil }); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(input.ProxyPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = io.WriteString(conn, "GET /embywebsocket HTTP/1.1\r\nHost: localhost\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nX-Emby-Token: user\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 101 {
		t.Fatalf("未升级: %d", response.StatusCode)
	}
	data := make([]byte, 5)
	if _, err := io.ReadFull(reader, data); err != nil || string(data) != "hello" {
		t.Fatalf("升级流失败: %s %v", data, err)
	}
	s.Close()
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("停止后 WebSocket 未关闭")
	}
}
