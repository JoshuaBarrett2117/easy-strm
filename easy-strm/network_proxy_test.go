package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildProxyDomainsIncludesBuiltInSites(t *testing.T) {
	domains := buildProxyDomains("")

	for _, host := range []string{
		"api.telegram.org",
		"t.me",
		"github.com",
		"api.github.com",
		"api.themoviedb.org",
	} {
		if !shouldProxyHost(host, domains) {
			t.Fatalf("内置站点 %s 应默认走代理，domains=%v", host, domains)
		}
	}

	if shouldProxyHost("example.com", domains) {
		t.Fatalf("非内置站点不应默认走代理，domains=%v", domains)
	}
}

func TestBuildProxyDomainsAppendsCustomDomains(t *testing.T) {
	domains := buildProxyDomains("custom.example.com")

	if !shouldProxyHost("custom.example.com", domains) {
		t.Fatalf("自定义站点应追加到代理规则，domains=%v", domains)
	}
	if !shouldProxyHost("api.themoviedb.org", domains) {
		t.Fatalf("自定义站点不应覆盖内置代理规则，domains=%v", domains)
	}
}

func TestShouldUseProxyRequiresProxyURL(t *testing.T) {
	domains := buildProxyDomains("")

	if shouldUseProxy(nil, "api.telegram.org", domains) {
		t.Fatal("未配置代理服务器地址时应保持直连")
	}

	proxyURL, err := url.Parse("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("解析测试代理地址失败: %v", err)
	}
	if !shouldUseProxy(proxyURL, "api.telegram.org", domains) {
		t.Fatal("代理地址有效且命中内置域名时应走代理")
	}
}

func TestBuildNetworkProbeRequestURLAddsTMDBAPIKey(t *testing.T) {
	const apiKey = "tmdb-secret-key"
	requestURL := buildNetworkProbeRequestURL("https://api.themoviedb.org/3/configuration?language=zh-CN", apiKey)

	parsed, err := url.Parse(requestURL)
	if err != nil {
		t.Fatalf("解析探测请求地址失败: %v", err)
	}
	if parsed.Query().Get("api_key") != apiKey {
		t.Fatalf("TMDB 探测请求应携带 API Key，url=%s", redactNetworkProbeSecret(requestURL, apiKey))
	}
	if parsed.Query().Get("language") != "zh-CN" {
		t.Fatalf("原有查询参数应保留，url=%s", redactNetworkProbeSecret(requestURL, apiKey))
	}
}

func TestBuildNetworkProbeRequestURLDoesNotModifyOtherSites(t *testing.T) {
	const rawURL = "https://api.github.com?api_key=public-value"
	if got := buildNetworkProbeRequestURL(rawURL, "tmdb-secret-key"); got != rawURL {
		t.Fatalf("非 TMDB 站点不应被修改，got=%s", got)
	}
}

func TestRedactNetworkProbeSecret(t *testing.T) {
	const apiKey = "tmdb-secret-key"
	message := redactNetworkProbeSecret("request failed: https://api.themoviedb.org/3/configuration?api_key="+apiKey, apiKey)
	if strings.Contains(message, apiKey) {
		t.Fatalf("错误信息不应泄漏 TMDB API Key: %s", message)
	}
	if !strings.Contains(message, "******") {
		t.Fatalf("错误信息应保留脱敏占位符: %s", message)
	}
}
