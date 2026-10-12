package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"easy-strm/internal/domain"
)

const embyProxyRelayMaxRedirects = 5

var errEmbyProxyRelayUnsafe = errors.New("媒体地址不是允许访问的 HTTP(S) 地址")

type embyProxyRelayTargetKey struct{}

type embyProxyRelayTarget struct {
	address string
	port    string
	ips     []netip.Addr
}

func newEmbyProxyRelayClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			// 不使用环境代理；连接必须经过已校验 IP 的拨号路径，TLS 仍验证原域名。
			DialContext:           dialEmbyProxyRelay,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   4,
			ForceAttemptHTTP2:     true,
			DisableCompression:    true,
		},
	}
}

func (s *EmbyProxyService) relayMedia(server *domain.EmbyServer, w http.ResponseWriter, incoming *http.Request, location string) {
	ctx, cancel := context.WithCancel(incoming.Context())
	stop := context.AfterFunc(s.relayContext, cancel)
	defer stop()
	defer cancel()
	if s.relayContext.Err() != nil {
		cancel()
	}
	response, failure := s.fetchRelayMedia(ctx, incoming, location)
	if failure != nil {
		writeEmbyProxyFailure(server, w, incoming, failure)
		return
	}
	defer response.Body.Close()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "Vary"} {
		for _, value := range response.Header.Values(name) {
			w.Header().Add(name, value)
		}
	}
	applyProxyCORS(w.Header(), incoming)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	if incoming.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, response.Body); err != nil {
		failure := newEmbyProxyFailure("relay", "stream_failed", "媒体传输中断", response.StatusCode, err)
		failure.status = response.StatusCode
		logEmbyProxyFailure(server, incoming, failure)
		return
	}
}

func (s *EmbyProxyService) fetchRelayMedia(ctx context.Context, incoming *http.Request, location string) (*http.Response, *embyProxyFailure) {
	current, err := url.Parse(location)
	if err != nil {
		return nil, newEmbyProxyFailure("relay", "unsafe_location", "媒体播放地址无效，已禁止回源", 0, err)
	}
	for redirects := 0; ; redirects++ {
		target, err := resolveEmbyProxyRelayTarget(ctx, current, s.relayLookup)
		if err != nil {
			return nil, newEmbyProxyFailure("relay", "unsafe_location", "媒体播放地址不可安全访问，已禁止回源", 0, err)
		}
		request, err := http.NewRequestWithContext(context.WithValue(ctx, embyProxyRelayTargetKey{}, target), incoming.Method, current.String(), nil)
		if err != nil {
			return nil, newEmbyProxyFailure("relay", "invalid_request", "无法创建媒体播放请求", 0, err)
		}
		// 每跳新建请求，只允许播放与缓存校验头，绝不复制 Emby 凭据或客户端查询参数。
		for _, name := range []string{"Range", "If-Range", "Accept", "User-Agent", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
			for _, value := range incoming.Header.Values(name) {
				request.Header.Add(name, value)
			}
		}
		response, err := s.relayClient.Do(request)
		if err != nil {
			return nil, newEmbyProxyFailure("relay", "connection_failed", "媒体播放上游不可用", 0, err)
		}
		switch response.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			_ = response.Body.Close()
			if redirects >= embyProxyRelayMaxRedirects {
				return nil, newEmbyProxyFailure("relay", "redirect_limit", "媒体播放重定向超过限制", response.StatusCode, nil)
			}
			next, err := response.Location()
			if err != nil {
				return nil, newEmbyProxyFailure("relay", "invalid_redirect", "媒体播放重定向地址无效", response.StatusCode, err)
			}
			current = next
		default:
			response.Request = request
			return response, nil
		}
	}
}

func resolveEmbyProxyRelayTarget(ctx context.Context, location *url.URL, lookup func(context.Context, string, string) ([]netip.Addr, error)) (embyProxyRelayTarget, error) {
	if location == nil || (location.Scheme != "http" && location.Scheme != "https") || location.Hostname() == "" || location.User != nil || location.Opaque != "" {
		return embyProxyRelayTarget{}, errEmbyProxyRelayUnsafe
	}
	port := location.Port()
	if port == "" {
		port = "80"
		if location.Scheme == "https" {
			port = "443"
		}
	}
	host := location.Hostname()
	var ips []netip.Addr
	if address, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{address}
	} else {
		// DNS 有界且每跳校验全部结果；混合公网/内网答案允许，但任一禁用地址都会拒绝整跳。
		resolveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var err error
		ips, err = lookup(resolveCtx, "ip", host)
		if err != nil {
			return embyProxyRelayTarget{}, err
		}
	}
	if len(ips) == 0 {
		return embyProxyRelayTarget{}, errEmbyProxyRelayUnsafe
	}
	for _, address := range ips {
		if !isEmbyProxyRelayAllowedIP(address) {
			return embyProxyRelayTarget{}, errEmbyProxyRelayUnsafe
		}
	}
	return embyProxyRelayTarget{address: net.JoinHostPort(host, port), port: port, ips: ips}, nil
}

// 允许内网/回环地址是刻意设计——内网分域 DNS 会把本服务的公网域名解析到局域网地址，用于节省 CDN 流量，中继必须照走。
func isEmbyProxyRelayAllowedIP(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if address.IsUnspecified() || address.IsMulticast() || address.IsLinkLocalUnicast() {
		return false
	}
	if address.Is4() {
		ipv4 := address.As4()
		if ipv4[0] == 0 || ipv4 == [4]byte{255, 255, 255, 255} {
			return false
		}
	}
	return true
}

func dialEmbyProxyRelay(ctx context.Context, network, address string) (net.Conn, error) {
	target, ok := ctx.Value(embyProxyRelayTargetKey{}).(embyProxyRelayTarget)
	if !ok || !strings.EqualFold(address, target.address) {
		return nil, errEmbyProxyRelayUnsafe
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return target.dial(ctx, network, dialer.DialContext)
}

func (target embyProxyRelayTarget) dial(ctx context.Context, network string, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var failures []error
	for _, address := range target.ips {
		if !isEmbyProxyRelayAllowedIP(address) {
			return nil, errEmbyProxyRelayUnsafe
		}
		connection, err := dial(dialCtx, network, net.JoinHostPort(address.Unmap().String(), target.port))
		if err == nil {
			return connection, nil
		}
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		return nil, errEmbyProxyRelayUnsafe
	}
	return nil, errors.Join(failures...)
}
