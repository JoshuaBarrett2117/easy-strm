package domain

import "testing"

func TestEmbyProxyConfigDefaultsAndNormalization(t *testing.T) {
	defaults := DefaultEmbyProxyConfig()
	if !defaults.CacheEnabled || defaults.CacheTTLSeconds != 600 || defaults.CacheCapacity != 1024 || !defaults.CacheIsolateUA || !defaults.CacheIsolateIP || defaults.ResolveFinalURL || defaults.ProbeHEAD || defaults.BrowserRedirect || defaults.ForceDirectPlay || defaults.ProxyErrorStrategy != "reject" {
		t.Fatalf("默认配置不符合兼容基线：%+v", defaults)
	}
	for _, enabled := range []bool{false, true} {
		normalized, err := (EmbyProxyConfig{ForceDirectPlay: enabled}).Normalize()
		if err != nil || normalized.ForceDirectPlay != enabled {
			t.Fatalf("强制直接播放开关归一化改变：%+v %v", normalized, err)
		}
	}
	for _, strategy := range []string{"", " reject ", " FALLBACK_ORIGIN "} {
		config := EmbyProxyConfig{ProxyErrorStrategy: strategy}
		normalized, err := config.Normalize()
		if err != nil || normalized.CacheTTLSeconds != 600 || normalized.CacheCapacity != 1024 || normalized.CacheEnabled {
			t.Fatalf("归一化错误：%+v %v", normalized, err)
		}
	}
	for _, config := range []EmbyProxyConfig{{CacheTTLSeconds: -1}, {CacheTTLSeconds: 86401}, {CacheCapacity: -1}, {CacheCapacity: 100001}, {ProxyErrorStrategy: "allow"}} {
		if _, err := config.Normalize(); err == nil {
			t.Fatalf("非法配置被接受：%+v", config)
		}
	}
}
