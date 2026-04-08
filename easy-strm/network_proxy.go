package main

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	systemConfigProxyURLKey     = "proxy_url"
	systemConfigProxyDomainsKey = "proxy_domains"
)

var proxyDomainAlias = map[string][]string{
	"tg":       {"telegram.org", "t.me", "api.telegram.org"},
	"telegram": {"telegram.org", "t.me", "api.telegram.org"},
	"github":   {"github.com", "api.github.com", "raw.githubusercontent.com", "gist.github.com"},
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
		if proxyURL == nil {
			return nil, nil
		}
		if !shouldProxyHost(req.URL.Hostname(), domains) {
			return nil, nil
		}
		return proxyURL, nil
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func RunNetworkProbe(sites []NetworkProbeSite, timeout time.Duration) []NetworkProbeResult {
	results := make([]NetworkProbeResult, 0, len(sites))
	httpClient := NewProxyAwareHTTPClient(timeout)
	_, proxyDomains := loadProxyConfigFromSystem()

	for _, site := range sites {
		result := NetworkProbeResult{
			Name:     site.Name,
			URL:      site.URL,
			ViaProxy: shouldProxySiteURL(site.URL, proxyDomains),
		}

		start := time.Now()
		req, err := http.NewRequest(http.MethodGet, site.URL, nil)
		if err != nil {
			result.Error = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			results = append(results, result)
			continue
		}
		req.Header.Set("User-Agent", "easy-strm-network-probe/1.0")

		resp, err := httpClient.Do(req)
		result.DurationMS = time.Since(start).Milliseconds()
		if err != nil {
			result.Error = err.Error()
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
		domains = normalizeProxyDomains(config.ConfigVal)
	}

	return proxyURL, domains
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

func shouldProxySiteURL(rawURL string, domains []string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return shouldProxyHost(parsed.Hostname(), domains)
}
