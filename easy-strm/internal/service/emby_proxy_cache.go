package service

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/domain"
)

type embyProxyLocationResult struct {
	location string
	original string
	failure  *embyProxyFailure
}

func (result embyProxyLocationResult) clone() embyProxyLocationResult {
	if result.failure != nil {
		failure := *result.failure
		result.failure = &failure
	}
	return result
}

type embyProxyCacheEntry struct {
	key     string
	result  embyProxyLocationResult
	expires time.Time
}

type embyProxyFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	result  embyProxyLocationResult
}

type embyProxyCache struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	flights  map[string]*embyProxyFlight
	capacity int
	ttl      time.Duration
	enabled  bool
	now      func() time.Time
}

func newEmbyProxyCache(config domain.EmbyProxyConfig) *embyProxyCache {
	return &embyProxyCache{entries: make(map[string]*list.Element), lru: list.New(), flights: make(map[string]*embyProxyFlight), capacity: config.CacheCapacity, ttl: time.Duration(config.CacheTTLSeconds) * time.Second, enabled: config.CacheEnabled, now: time.Now}
}

func (cache *embyProxyCache) clear() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.entries = make(map[string]*list.Element)
	cache.lru.Init()
}

// resolve 用互斥保护 TTL/LRU 与同键任务；输入键和只读请求协作者，输出独立结果副本。
// 等待者可独立取消，最后一个等待者退出会取消任务；任务受服务关闭和 30 秒上限约束，满容量拒绝新增任务。
func (cache *embyProxyCache) resolve(ctx, lifetime context.Context, key string, resolve func(context.Context) embyProxyLocationResult) embyProxyLocationResult {
	if ctx.Err() != nil || lifetime.Err() != nil {
		return embyProxyCanceledResult()
	}
	cache.mu.Lock()
	if entry := cache.entries[key]; entry != nil {
		value := entry.Value.(embyProxyCacheEntry)
		if cache.now().Before(value.expires) {
			cache.lru.MoveToFront(entry)
			cache.mu.Unlock()
			return value.result.clone()
		}
		cache.lru.Remove(entry)
		delete(cache.entries, key)
	}
	flight := cache.flights[key]
	if flight == nil {
		if len(cache.flights) >= cache.capacity {
			cache.mu.Unlock()
			return embyProxyLocationResult{failure: newEmbyProxyFailure("cache", "resolution_busy", "媒体信息解析并发已达上限，请重试", 0, nil)}
		}
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		stop := context.AfterFunc(lifetime, cancel)
		flight = &embyProxyFlight{done: make(chan struct{}), cancel: cancel}
		cache.flights[key] = flight
		go func() {
			defer stop()
			defer cancel()
			result := resolve(workCtx)
			cache.mu.Lock()
			defer cache.mu.Unlock()
			flight.result = result.clone()
			if cache.flights[key] == flight {
				delete(cache.flights, key)
				if cache.enabled && workCtx.Err() == nil && lifetime.Err() == nil && result.failure == nil && result.location != "" {
					cache.entries[key] = cache.lru.PushFront(embyProxyCacheEntry{key: key, result: result.clone(), expires: cache.now().Add(cache.ttl)})
					for cache.lru.Len() > cache.capacity {
						old := cache.lru.Back()
						delete(cache.entries, old.Value.(embyProxyCacheEntry).key)
						cache.lru.Remove(old)
					}
				}
			}
			close(flight.done)
		}()
	}
	flight.waiters++
	cache.mu.Unlock()
	select {
	case <-flight.done:
		if ctx.Err() != nil || lifetime.Err() != nil {
			return embyProxyCanceledResult()
		}
		return flight.result.clone()
	case <-ctx.Done():
	case <-lifetime.Done():
	}
	cache.mu.Lock()
	flight.waiters--
	if flight.waiters == 0 && cache.flights[key] == flight {
		delete(cache.flights, key)
		flight.cancel()
	}
	cache.mu.Unlock()
	return embyProxyCanceledResult()
}

func embyProxyCanceledResult() embyProxyLocationResult {
	return embyProxyLocationResult{failure: newEmbyProxyFailure("playback_info", "connection_failed", "媒体信息解析已取消，已禁止回源", 0, context.Canceled)}
}

// embyProxyCacheKey 对实例配置代次、客户端身份及非易变参数整体哈希；令牌缺失与存在互不复用。
// IP 只取连接对端，不信任可伪造转发头；仅忽略播放会话、起播位置，未知参数保留以免遗漏安全维度。
func (s *EmbyProxyService) embyProxyCacheKey(server *domain.EmbyServer, incoming *http.Request, playback embyProxyPlaybackRequest, generation uint64) string {
	query := incoming.URL.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "starttimeticks", "playsessionid", "x-playback-session-id":
			delete(query, key)
		}
	}
	identity := make(http.Header)
	for _, key := range []string{"Authorization", "X-Emby-Authorization", "X-Emby-Token", "X-MediaBrowser-Token", "Cookie"} {
		identity[key] = append([]string(nil), incoming.Header.Values(key)...)
	}
	dimensions := []any{server.ID, server.BaseURL, server.APIKey, server.ProxyPort, generation, s.options, playback.itemID, playback.prefix, embyQuery(query, "MediaSourceId"), query, identity}
	if s.options.CacheIsolateUA {
		dimensions = append(dimensions, incoming.Header.Values("User-Agent"))
	}
	if s.options.CacheIsolateIP {
		host, _, err := net.SplitHostPort(incoming.RemoteAddr)
		if err != nil {
			host = incoming.RemoteAddr
		}
		dimensions = append(dimensions, host)
	}
	encoded, _ := json.Marshal(dimensions)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// resolvePlaybackLocation 将实例、请求和代次转换为隔离键，合并识别及可选探测，返回保留原地址的独立结果。
// 转码入口始终绕过缓存并重新授权识别；两条路径均受取消、服务关闭和 30 秒解析上限约束。
func (s *EmbyProxyService) resolvePlaybackLocation(server *domain.EmbyServer, base *url.URL, incoming *http.Request, playback embyProxyPlaybackRequest, generation uint64) embyProxyLocationResult {
	resolve := func(ctx context.Context) embyProxyLocationResult {
		request := incoming.Clone(ctx)
		location, failure := s.playbackLocation(server, base, request, playback)
		result := embyProxyLocationResult{location: location, original: location, failure: failure}
		if failure == nil && location != "" {
			return s.prepareMediaLocation(request, location)
		}
		return result
	}
	if playback.transcoding {
		ctx, cancel := context.WithTimeout(incoming.Context(), 30*time.Second)
		defer cancel()
		stop := context.AfterFunc(s.relayContext, cancel)
		defer stop()
		return resolve(ctx)
	}
	key := s.embyProxyCacheKey(server, incoming, playback, generation)
	return s.cache.resolve(incoming.Context(), s.relayContext, key, resolve)
}
