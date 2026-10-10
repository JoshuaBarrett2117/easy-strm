package service

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// EmbyProxyService 管理实例独立反代端口，保存前绑定端口，保存失败不改变现有代理。
type EmbyProxyService struct {
	servers   *dao.EmbyServerDAO
	client    *http.Client
	reserved  map[int]bool
	mu        sync.Mutex
	listeners map[int]*embyProxyListener
	closed    bool
}

type embyProxyHandler struct{ http.Handler }

type embyProxyListener struct {
	port        int
	socket      net.Listener
	server      *http.Server
	handler     atomic.Pointer[embyProxyHandler]
	connections sync.Map
}

type embyProxyTrackedListener struct {
	net.Listener
	connections *sync.Map
}

type embyProxyConnection struct {
	net.Conn
	connections *sync.Map
}

// Accept 跟踪新连接，使升级后的 WebSocket 也能随实例停止。
func (l *embyProxyTrackedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tracked := &embyProxyConnection{Conn: conn, connections: l.connections}
	l.connections.Store(tracked, true)
	return tracked, nil
}

// Close 关闭连接并移除跟踪项，避免保留已结束的 WebSocket。
func (c *embyProxyConnection) Close() error {
	c.connections.Delete(c)
	return c.Conn.Close()
}

func (l *embyProxyListener) close() {
	_ = l.socket.Close()
	_ = l.server.Close()
	// http.Server.Close 不关闭已升级的 WebSocket，实例停止时一并断开。
	l.connections.Range(func(key, _ any) bool { _ = key.(net.Conn).Close(); return true })
}

// NewEmbyProxyService 创建反代服务；reservedPorts 为应用及 Nginx 等必须保留的端口。
func NewEmbyProxyService(servers *dao.EmbyServerDAO, client *http.Client, reservedPorts ...int) *EmbyProxyService {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	reserved := make(map[int]bool)
	for _, port := range reservedPorts {
		reserved[port] = true
	}
	return &EmbyProxyService{servers: servers, client: &copyClient, reserved: reserved, listeners: make(map[int]*embyProxyListener)}
}

// Start 加载已启用实例；逐项报告启动失败，其他可用实例仍启动。
func (s *EmbyProxyService) Start() error {
	servers, err := s.servers.List()
	if err != nil {
		return err
	}
	var failures []error
	for _, server := range servers {
		if !server.Enabled || server.ProxyPort == 0 {
			continue
		}
		_, err := s.save(server, func() (*domain.EmbyServer, error) { return server, nil })
		if err != nil {
			failures = append(failures, fmt.Errorf("Emby 实例 %d 反代启动失败: %w", server.ID, err))
		}
	}
	return errors.Join(failures...)
}

// Close 关闭全部监听器、正在传输的请求和 WebSocket。
func (s *EmbyProxyService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id, listener := range s.listeners {
		listener.close()
		delete(s.listeners, id)
	}
}

// save 在互斥区内保留新端口至数据库写入完成，避免端口检查与启动之间的竞争。
func (s *EmbyProxyService) save(input *domain.EmbyServer, persist func() (*domain.EmbyServer, error)) (*domain.EmbyServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("Emby 反代服务已关闭")
	}
	if err := validateEmbyServerInput(input.Name, input.BaseURL, input.APIKey, true); err != nil {
		return nil, err
	}
	if input.ProxyPort < 0 || input.ProxyPort > 65535 {
		return nil, fmt.Errorf("反代端口必须为 0（关闭）或 1-65535")
	}
	if input.ProxyPort != 0 && s.reserved[input.ProxyPort] {
		return nil, fmt.Errorf("反代端口 %d 已被 easy-strm 或 Nginx 使用", input.ProxyPort)
	}
	for id, listener := range s.listeners {
		if id != input.ID && input.ProxyPort != 0 && listener.port == input.ProxyPort {
			return nil, fmt.Errorf("反代端口 %d 已被其他 Emby 实例使用", input.ProxyPort)
		}
	}
	old := s.listeners[input.ID]
	var bound net.Listener
	if input.ProxyPort != 0 && (old == nil || old.port != input.ProxyPort) {
		var err error
		bound, err = net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(input.ProxyPort)))
		if err != nil {
			return nil, fmt.Errorf("反代端口 %d 无法监听，请检查端口占用及权限: %w", input.ProxyPort, err)
		}
		defer func() {
			if bound != nil {
				_ = bound.Close()
			}
		}()
	}
	server, err := persist()
	if err != nil {
		return nil, err
	}
	if !server.Enabled || server.ProxyPort == 0 {
		if old != nil {
			old.close()
			delete(s.listeners, input.ID)
		}
		return server, nil
	}
	handler := s.buildHandler(server)
	if old != nil && old.port == server.ProxyPort {
		old.handler.Store(&embyProxyHandler{handler})
		return server, nil
	}
	listener := &embyProxyListener{port: server.ProxyPort, socket: bound}
	listener.handler.Store(&embyProxyHandler{handler})
	listener.server = &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { listener.handler.Load().ServeHTTP(w, r) }),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	s.listeners[server.ID] = listener
	serving := &embyProxyTrackedListener{Listener: bound, connections: &listener.connections}
	bound = nil
	go func() {
		if err := listener.server.Serve(serving); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			logger.Error("Emby 实例 %d 反代端口 %d 监听失败: %v", server.ID, server.ProxyPort, err)
		}
	}()
	if old != nil {
		old.close()
	}
	return server, nil
}

func (s *EmbyProxyService) delete(id int, persist func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := persist(); err != nil {
		return err
	}
	if listener := s.listeners[id]; listener != nil {
		listener.close()
		delete(s.listeners, id)
	}
	return nil
}

func (s *EmbyProxyService) buildHandler(server *domain.EmbyServer) http.Handler {
	// 捕获配置副本，后续同端口更新不会影响进行中的请求。
	config := *server
	target, _ := url.Parse(config.BaseURL)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(embyProxyTarget(target, r.In.URL.Path))
			r.SetXForwarded()
			r.Out.Header.Set("X-Request-ID", logger.RequestID(r.In.Context()))
		},
		Transport: s.client.Transport,
		ModifyResponse: func(response *http.Response) error {
			// 请求入口已设置标识，移除上游同名头，避免 ReverseProxy 追加重复值。
			response.Header.Del("X-Request-ID")
			if response.Request != nil {
				if response.StatusCode >= 500 {
					failure := newEmbyProxyFailure("proxy", "upstream_http_error", "Emby 反代上游返回异常状态", response.StatusCode, nil)
					failure.status, failure.blocked = response.StatusCode, false
					logEmbyProxyFailure(&config, response.Request, failure)
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			failure := newEmbyProxyFailure("proxy", "connection_failed", "Emby 反代上游不可用", 0, err)
			failure.blocked = false
			writeEmbyProxyFailure(&config, w, r, failure)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		playback, matched := embyPlaybackRequest(r)
		contextual, err := beginEmbyProxyRequest(r, w, playback)
		if err != nil {
			http.Error(w, "请求标识随机源不可用", http.StatusServiceUnavailable)
			return
		}
		r = contextual
		if !matched {
			proxy.ServeHTTP(w, r)
			return
		}
		location, failure := s.playbackLocation(&config, target, r, playback)
		if failure != nil {
			writeEmbyProxyFailure(&config, w, r, failure)
			return
		}
		if location == "" {
			proxy.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusFound)
	})
}

// base_url 已包含 /emby 时去掉重复前缀，其他子路径沿用标准库的路径拼接。
func embyProxyTarget(base *url.URL, path string) *url.URL {
	target := *base
	if strings.HasSuffix(strings.ToLower(strings.TrimRight(target.Path, "/")), "/emby") && strings.HasPrefix(strings.ToLower(path), "/emby/") {
		escaped := strings.TrimRight(target.EscapedPath(), "/")
		target.Path = strings.TrimRight(target.Path, "/")[:len(strings.TrimRight(target.Path, "/"))-5]
		if index := strings.LastIndex(escaped, "/"); index >= 0 {
			target.RawPath = escaped[:index]
		}
	}
	return &target
}

func embyQuery(query url.Values, key string) string {
	for name, values := range query {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func embyClientIdentity(r *http.Request) bool {
	for _, key := range []string{"X-Emby-Token", "X-MediaBrowser-Token", "Cookie"} {
		if r.Header.Get(key) != "" {
			return true
		}
	}
	for _, key := range []string{"Authorization", "X-Emby-Authorization"} {
		value := strings.ToLower(r.Header.Get(key))
		if strings.Contains(value, "token=") || strings.HasPrefix(value, "bearer ") || strings.HasPrefix(value, "basic ") {
			return true
		}
		if value != "" && !strings.HasPrefix(value, "mediabrowser ") && !strings.HasPrefix(value, "emby ") {
			return true
		}
	}
	return embyQuery(r.URL.Query(), "api_key") != "" || embyQuery(r.URL.Query(), "AccessToken") != "" || embyQuery(r.URL.Query(), "X-Emby-Token") != ""
}
