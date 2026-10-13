package main

import (
	"testing"

	"easy-strm/internal/domain"
	"gopkg.in/yaml.v2"
)

func TestEmbyProxyAppConfigYAMLDefaultsAndOverrides(t *testing.T) {
	for _, testCase := range []struct {
		input                  string
		wantCache, wantBrowser bool
		wantTTL                int
		wantError              bool
		wantForceDirectPlay    bool
	}{
		{input: "{}", wantCache: true, wantTTL: 600},
		{input: "emby_proxy:\n  cache_enabled: false\n  browser_redirect: true\n  cache_ttl_seconds: 30\n  proxy_error_strategy: ' FALLBACK_ORIGIN '", wantBrowser: true, wantTTL: 30},
		{input: "emby_proxy:\n  cache_ttl_seconds: -5", wantError: true},
		{input: "emby_proxy:\n  proxy_error_strategy: unknown", wantError: true},
		{input: "emby_proxy:\n  force_direct_play: true", wantCache: true, wantTTL: 600, wantForceDirectPlay: true},
		{input: "emby_proxy:\n  force_direct_play: false", wantCache: true, wantTTL: 600},
	} {
		config := Config{EmbyProxy: domain.DefaultEmbyProxyConfig()}
		if err := yaml.Unmarshal([]byte(testCase.input), &config); err != nil {
			t.Fatal(err)
		}
		normalized, err := config.EmbyProxy.Normalize()
		if (err != nil) != testCase.wantError {
			t.Fatalf("配置验证结果错误：%v", err)
		}
		if !testCase.wantError && (normalized.CacheEnabled != testCase.wantCache || normalized.BrowserRedirect != testCase.wantBrowser || normalized.CacheTTLSeconds != testCase.wantTTL || !normalized.CacheIsolateIP || !normalized.CacheIsolateUA || normalized.ForceDirectPlay != testCase.wantForceDirectPlay) {
			t.Fatalf("配置加载结果错误：%+v", normalized)
		}
	}
}
