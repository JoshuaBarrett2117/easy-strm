package domain

import (
	"fmt"
	"strings"
)

// EmbyProxyConfig 定义进程级反代策略；启动时注入，不保存到实例数据库。
type EmbyProxyConfig struct {
	CacheEnabled       bool   `yaml:"cache_enabled"`
	CacheTTLSeconds    int    `yaml:"cache_ttl_seconds"`
	CacheCapacity      int    `yaml:"cache_capacity"`
	CacheIsolateUA     bool   `yaml:"cache_isolate_ua"`
	CacheIsolateIP     bool   `yaml:"cache_isolate_ip"`
	ResolveFinalURL    bool   `yaml:"resolve_final_url"`
	ProbeHEAD          bool   `yaml:"probe_head"`
	BrowserRedirect    bool   `yaml:"browser_redirect"`
	ForceDirectPlay    bool   `yaml:"force_direct_play"`
	ProxyErrorStrategy string `yaml:"proxy_error_strategy"`
}

// DefaultEmbyProxyConfig 返回兼容旧播放链路的默认值，仅启用有界直链缓存。
func DefaultEmbyProxyConfig() EmbyProxyConfig {
	return EmbyProxyConfig{CacheEnabled: true, CacheTTLSeconds: 600, CacheCapacity: 1024, CacheIsolateUA: true, CacheIsolateIP: true, ProxyErrorStrategy: "reject"}
}

// Normalize 校验启动配置并补齐零值时长、容量和策略；非法边界返回错误，不静默放宽安全策略。
func (config EmbyProxyConfig) Normalize() (EmbyProxyConfig, error) {
	if config.CacheTTLSeconds == 0 {
		config.CacheTTLSeconds = 600
	}
	if config.CacheCapacity == 0 {
		config.CacheCapacity = 1024
	}
	config.ProxyErrorStrategy = strings.ToLower(strings.TrimSpace(config.ProxyErrorStrategy))
	if config.ProxyErrorStrategy == "" {
		config.ProxyErrorStrategy = "reject"
	}
	if config.CacheTTLSeconds < 1 || config.CacheTTLSeconds > 86400 {
		return config, fmt.Errorf("Emby 反代缓存 TTL 必须为 1-86400 秒")
	}
	if config.CacheCapacity < 1 || config.CacheCapacity > 100000 {
		return config, fmt.Errorf("Emby 反代缓存容量必须为 1-100000")
	}
	if config.ProxyErrorStrategy != "reject" && config.ProxyErrorStrategy != "fallback_origin" {
		return config, fmt.Errorf("Emby 反代错误策略必须为 reject 或 fallback_origin")
	}
	return config, nil
}
