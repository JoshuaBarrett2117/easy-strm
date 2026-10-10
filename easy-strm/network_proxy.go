package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	systemConfigProxyURLKey     = "proxy_url"
	systemConfigProxyDomainsKey = "proxy_domains"
	systemConfigTMDBAPIKey      = "tmdb_api_key"
	proxyDialTimeout            = 10 * time.Second
	proxyResponseHeaderTimeout  = 15 * time.Second
)

var proxyDomainAlias = map[string][]string{
	"tg":       {"telegram.org", "t.me", "api.telegram.org"},
	"telegram": {"telegram.org", "t.me", "api.telegram.org"},
	"github":   {"github.com", "api.github.com", "raw.githubusercontent.com", "gist.github.com"},
	"tmdb":     {"themoviedb.org", "image.tmdb.org"},
}

type NetworkProbeSite struct {
	Name string
	URL  string
}

type NetworkProbeResult struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	DurationMS int64  `json:"duration_ms"`
	ViaProxy   bool   `json:"via_proxy"`
	Error      string `json:"error,omitempty"`
}

// NewProxyAwareHTTPClient 按系统白名单选路，代理传输失败时直连一次，保留调用方总超时。
func NewProxyAwareHTTPClient(timeout time.Duration) *http.Client {
	return newProxyAwareHTTPClient(timeout, loadProxyConfigFromSystem, http.DefaultTransport.(*http.Transport))
}

func newProxyAwareHTTPClient(timeout time.Duration, loader func() (*url.URL, []string), base *http.Transport) *http.Client {
	direct := base.Clone()
	direct.Proxy = nil
	tmdbDirect := direct.Clone()
	tmdbDirect.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return direct.DialContext(ctx, "tcp4", address)
	}
	proxy := direct.Clone()
	configureProxyTransport(proxy, direct.DialContext, proxyDialTimeout, proxyResponseHeaderTimeout)
	proxy.Proxy = func(req *http.Request) (*url.URL, error) {
		proxyURL, _ := req.Context().Value(proxyRouteKey{}).(*url.URL)
		return proxyURL, nil
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &proxyFallbackTransport{proxy: proxy, direct: direct, tmdbDirect: tmdbDirect, loadProxyConfig: loader},
	}
}

// configureProxyTransport 限制代理拨号与连接建立（包括 CONNECT），GotConn 后解除连接期限以保留流式响应。
func configureProxyTransport(transport *http.Transport, dial func(context.Context, string, string) (net.Conn, error), dialTimeout, headerTimeout time.Duration) {
	transport.ResponseHeaderTimeout = headerTimeout
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, dialTimeout)
		defer cancel()
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if headerTimeout > 0 {
			if err := conn.SetDeadline(time.Now().Add(headerTimeout)); err != nil {
				conn.Close()
				return nil, err
			}
		}
		return conn, nil
	}
}

type proxyRouteKey struct{}

// proxyFallbackTransport 仅对实际走代理的传输错误直连一次；请求可能已被服务端执行，重试仍有重复副作用风险。
type proxyFallbackTransport struct {
	proxy           http.RoundTripper
	direct          http.RoundTripper
	tmdbDirect      http.RoundTripper
	loadProxyConfig func() (*url.URL, []string)
}

// RoundTrip 每次请求只读取一次选路配置，保留原始上下文，仅通过 GetBody 重放请求体。
func (fallback *proxyFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	proxyURL, domains := fallback.loadProxyConfig()
	if !shouldUseProxy(proxyURL, req.URL.Hostname(), domains) {
		return fallback.direct.RoundTrip(req)
	}
	ctx := context.WithValue(req.Context(), proxyRouteKey{}, proxyURL)
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if err := info.Conn.SetDeadline(time.Time{}); err != nil {
				info.Conn.Close()
			}
		},
	})
	resp, err := fallback.proxy.RoundTrip(req.Clone(ctx))
	if err == nil {
		return resp, err
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if contextErr := req.Context().Err(); contextErr != nil {
		return nil, contextErr
	}
	retry := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil, err
		}
		body, replayErr := req.GetBody()
		if replayErr != nil {
			if body != nil {
				body.Close()
			}
			return nil, fmt.Errorf("代理请求失败 (%v)，无法重放请求体: %w", err, replayErr)
		}
		if body == nil {
			return nil, fmt.Errorf("无法重放请求体: %w", err)
		}
		retry.Body = body
	}
	direct := fallback.direct
	if isTMDBHost(req.URL.Hostname()) {
		direct = fallback.tmdbDirect
	}
	resp, err = direct.RoundTrip(retry)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	return resp, nil
}

// CloseIdleConnections 释放代理与两类直连传输的空闲连接，不中断正在读取的响应体。
func (fallback *proxyFallbackTransport) CloseIdleConnections() {
	for _, transport := range []http.RoundTripper{fallback.proxy, fallback.direct, fallback.tmdbDirect} {
		if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}

func isTMDBHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "api.themoviedb.org" || strings.HasSuffix(host, ".themoviedb.org")
}

func RunNetworkProbe(sites []NetworkProbeSite, timeout time.Duration) []NetworkProbeResult {
	results := make([]NetworkProbeResult, 0, len(sites))
	httpClient := NewProxyAwareHTTPClient(timeout)
	proxyURL, proxyDomains := loadProxyConfigFromSystem()
	tmdbAPIKey := loadTMDBAPIKeyFromSystem()

	for _, site := range sites {
		result := NetworkProbeResult{
			Name:     site.Name,
			URL:      site.URL,
			ViaProxy: shouldProxySiteURL(proxyURL, site.URL, proxyDomains),
		}

		start := time.Now()
		requestURL := buildNetworkProbeRequestURL(site.URL, tmdbAPIKey)
		req, err := http.NewRequest(http.MethodGet, requestURL, nil)
		if err != nil {
			result.Error = redactNetworkProbeSecret(err.Error(), tmdbAPIKey)
			result.DurationMS = time.Since(start).Milliseconds()
			results = append(results, result)
			continue
		}
		req.Header.Set("User-Agent", "easy-strm-network-probe/1.0")

		resp, err := httpClient.Do(req)
		result.DurationMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Error = redactNetworkProbeSecret(err.Error(), tmdbAPIKey)
			results = append(results, result)
			continue
		}

		result.StatusCode = resp.StatusCode
		result.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
		resp.Body.Close()
		results = append(results, result)
	}

	return results
}

func loadTMDBAPIKeyFromSystem() string {
	config, err := GetSystemConfigByKey(systemConfigTMDBAPIKey)
	if err != nil || config == nil {
		return ""
	}

	apiKey := strings.TrimSpace(config.ConfigVal)
	if strings.Contains(apiKey, "****") {
		return ""
	}
	return apiKey
}

// buildNetworkProbeRequestURL 为 TMDB 探测请求附加鉴权参数，其他站点保持原样。
func buildNetworkProbeRequestURL(rawURL, tmdbAPIKey string) string {
	apiKey := strings.TrimSpace(tmdbAPIKey)
	if apiKey == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "api.themoviedb.org") {
		return rawURL
	}

	query := parsed.Query()
	query.Set("api_key", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// redactNetworkProbeSecret 避免底层 HTTP 错误将 TMDB API Key 回传到页面。
func redactNetworkProbeSecret(message, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return message
	}
	return strings.ReplaceAll(message, secret, "******")
}

func loadProxyConfigFromSystem() (*url.URL, []string) {
	var proxyURL *url.URL
	var domains []string

	if config, err := GetSystemConfigByKey(systemConfigProxyURLKey); err == nil && config != nil {
		raw := strings.TrimSpace(config.ConfigVal)
		if raw != "" {
			if parsed, parseErr := url.Parse(raw); parseErr == nil && parsed.Scheme != "" && parsed.Host != "" {
				proxyURL = parsed
			}
		}
	}

	if config, err := GetSystemConfigByKey(systemConfigProxyDomainsKey); err == nil && config != nil {
		domains = buildProxyDomains(config.ConfigVal)
	} else {
		domains = buildProxyDomains("")
	}

	return proxyURL, domains
}

// buildProxyDomains 合并内置代理站点与用户追加的自定义站点。
func buildProxyDomains(raw string) []string {
	parts := make([]string, 0, len(proxyDomainAlias)+1)
	for alias := range proxyDomainAlias {
		parts = append(parts, alias)
	}
	sort.Strings(parts)
	if custom := strings.TrimSpace(raw); custom != "" {
		parts = append(parts, custom)
	}
	return normalizeProxyDomains(strings.Join(parts, "\n"))
}

func normalizeProxyDomains(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})

	seen := make(map[string]struct{})
	result := make([]string, 0, len(parts))

	appendDomain := func(domain string) {
		domain = strings.ToLower(strings.TrimSpace(domain))
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "*.")
		domain = strings.Trim(domain, "/")
		domain = strings.TrimSuffix(domain, ".")
		if domain == "" {
			return
		}
		if _, exists := seen[domain]; exists {
			return
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}

	for _, part := range parts {
		key := strings.ToLower(strings.TrimSpace(part))
		if aliases, ok := proxyDomainAlias[key]; ok {
			for _, alias := range aliases {
				appendDomain(alias)
			}
			continue
		}
		appendDomain(key)
	}

	return result
}

func shouldProxyHost(host string, domains []string) bool {
	if host == "" || len(domains) == 0 {
		return false
	}

	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")

	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func shouldUseProxy(proxyURL *url.URL, host string, domains []string) bool {
	return proxyURL != nil && shouldProxyHost(host, domains)
}

func shouldProxySiteURL(proxyURL *url.URL, rawURL string, domains []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return shouldUseProxy(proxyURL, parsed.Hostname(), domains)
}
