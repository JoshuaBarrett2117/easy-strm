package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	systemConfigProxyURLKey     = "proxy_url"
	systemConfigProxyDomainsKey = "proxy_domains"
	systemConfigTMDBAPIKey      = "tmdb_api_key"
)

var proxyDomainAlias = map[string][]string{
	"tg":       {"telegram.org", "t.me", "api.telegram.org"},
	"telegram": {"telegram.org", "t.me", "api.telegram.org"},
	"github":   {"github.com", "api.github.com", "raw.githubusercontent.com", "gist.github.com"},
	"tmdb":     {"themoviedb.org"},
}

// defaultProxyDomains 是启用代理后始终通过代理访问的内置站点。
var defaultProxyDomains = []string{
	"telegram.org",
	"t.me",
	"github.com",
	"themoviedb.org",
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

func NewProxyAwareHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		proxyURL, domains := loadProxyConfigFromSystem()
		if !shouldUseProxy(proxyURL, req.URL.Hostname(), domains) {
			return nil, nil
		}
		return proxyURL, nil
	}
	// 部分本地代理对 TMDB 的 TLS/HTTP2 连接会返回 EOF；TMDB 请求失败时使用
	// IPv4 直连兜底，其他站点仍严格沿用原有代理策略。
	direct := http.DefaultTransport.(*http.Transport).Clone()
	direct.Proxy = nil
	direct.ForceAttemptHTTP2 = true
	direct.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp4", address)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &tmdbFallbackTransport{primary: transport, direct: direct},
	}
}

// tmdbFallbackTransport 只为 TMDB 的临时代理故障提供直连兜底。
type tmdbFallbackTransport struct {
	primary http.RoundTripper
	direct  http.RoundTripper
}

func (t *tmdbFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.primary.RoundTrip(req)
	if err == nil || !isTMDBHost(req.URL.Hostname()) {
		return resp, err
	}
	return t.direct.RoundTrip(req)
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
	parts := append([]string{}, defaultProxyDomains...)
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
