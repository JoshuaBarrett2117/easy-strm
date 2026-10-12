package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

func TestEmbyProxyCacheTTLAndLRUBound(t *testing.T) {
	options := domain.DefaultEmbyProxyConfig()
	options.CacheCapacity = 2
	cache := newEmbyProxyCache(options)
	now := time.Unix(1000, 0)
	cache.now = func() time.Time { return now }
	var calls atomic.Int32
	resolve := func(context.Context) embyProxyLocationResult {
		calls.Add(1)
		return embyProxyLocationResult{location: embyProxyTestMediaURL}
	}
	for _, key := range []string{"first", "second", "first", "third", "first"} {
		cache.resolve(context.Background(), context.Background(), key, resolve)
	}
	if calls.Load() != 3 || cache.lru.Len() != 2 || cache.entries["second"] != nil {
		t.Fatalf("LRU 错误：调用=%d 容量=%d", calls.Load(), cache.lru.Len())
	}
	cache.mu.Lock()
	now = now.Add(cache.ttl)
	cache.mu.Unlock()
	cache.resolve(context.Background(), context.Background(), "first", resolve)
	if calls.Load() != 4 {
		t.Fatal("TTL 边界未重新解析")
	}
	options.CacheEnabled = false
	disabled := newEmbyProxyCache(options)
	for index := 0; index < 2; index++ {
		disabled.resolve(context.Background(), context.Background(), "key", resolve)
	}
	if calls.Load() != 6 || disabled.lru.Len() != 0 {
		t.Fatal("禁用缓存仍复用结果")
	}
}

func TestEmbyProxyCacheIsolation(t *testing.T) {
	service := NewEmbyProxyService(nil, nil)
	defer service.Close()
	server := &domain.EmbyServer{ID: 7, BaseURL: "http://emby.example", APIKey: "test"}
	request := httptest.NewRequest("GET", "/Videos/42/stream?MediaSourceId=s1&api_key=user", nil)
	request.Header.Set("User-Agent", "player")
	request.RemoteAddr = "192.0.2.1:1000"
	playback, _ := embyPlaybackRequest(request)
	original := service.embyProxyCacheKey(server, request, playback, 1)
	changes := map[string]func(*http.Request){
		"media_source":      func(request *http.Request) { request.URL.RawQuery = "MediaSourceId=s2&api_key=user" },
		"query_token":       func(request *http.Request) { request.URL.RawQuery = "MediaSourceId=s1&api_key=other" },
		"absent_token":      func(request *http.Request) { request.URL.RawQuery = "MediaSourceId=s1" },
		"authorization":     func(request *http.Request) { request.Header.Set("Authorization", "Bearer other") },
		"cookie":            func(request *http.Request) { request.Header.Set("Cookie", "identity=other") },
		"header_token":      func(request *http.Request) { request.Header.Set("X-Emby-Token", "other") },
		"ua":                func(request *http.Request) { request.Header.Set("User-Agent", "other") },
		"ip":                func(request *http.Request) { request.RemoteAddr = "192.0.2.2:1000" },
		"user_id":           func(request *http.Request) { request.URL.RawQuery += "&UserId=other" },
		"unknown_dimension": func(request *http.Request) { request.URL.RawQuery += "&policy=other" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			modified := request.Clone(context.Background())
			change(modified)
			if service.embyProxyCacheKey(server, modified, playback, 1) == original {
				t.Fatal("缓存身份未隔离")
			}
		})
	}
	if service.embyProxyCacheKey(server, request, playback, 2) == original {
		t.Fatal("配置代次未隔离")
	}
	other := *server
	other.APIKey = "changed"
	if service.embyProxyCacheKey(&other, request, playback, 1) == original {
		t.Fatal("实例凭据未隔离")
	}
	other = *server
	other.BaseURL = "http://another.example"
	if service.embyProxyCacheKey(&other, request, playback, 1) == original {
		t.Fatal("实例地址未隔离")
	}
	modified := request.Clone(context.Background())
	modified.RemoteAddr = "192.0.2.1:9999"
	modified.URL.RawQuery += "&PlaySessionId=volatile&StartTimeTicks=123&X-Playback-Session-Id=random"
	if service.embyProxyCacheKey(server, modified, playback, 1) != original {
		t.Fatal("易变播放参数或连接端口参与了缓存键")
	}
	service.options.CacheIsolateUA, service.options.CacheIsolateIP = false, false
	original = service.embyProxyCacheKey(server, request, playback, 1)
	modified.Header.Set("User-Agent", "different")
	modified.RemoteAddr = "192.0.2.5:1"
	if service.embyProxyCacheKey(server, modified, playback, 1) != original {
		t.Fatal("关闭 UA/IP 隔离未生效")
	}
}

func waitEmbyProxyFlight(t *testing.T, cache *embyProxyCache, key string, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		flight := cache.flights[key]
		ready := flight != nil && flight.waiters == count
		cache.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("等待者未及时加入同键任务")
}

func TestEmbyProxyCacheCancellableWaitAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiter", true: "shutdown"}[shutdown], func(t *testing.T) {
			cache := newEmbyProxyCache(domain.DefaultEmbyProxyConfig())
			lifetime, closeService := context.WithCancel(context.Background())
			defer closeService()
			caller, cancelCaller := context.WithCancel(context.Background())
			defer cancelCaller()
			started, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			resolve := func(ctx context.Context) embyProxyLocationResult {
				calls.Add(1)
				close(started)
				select {
				case <-release:
					return embyProxyLocationResult{location: embyProxyTestMediaURL}
				case <-ctx.Done():
					close(stopped)
					return embyProxyCanceledResult()
				}
			}
			first, second := make(chan embyProxyLocationResult, 1), make(chan embyProxyLocationResult, 1)
			go func() { first <- cache.resolve(caller, lifetime, "key", resolve) }()
			<-started
			go func() { second <- cache.resolve(context.Background(), lifetime, "key", resolve) }()
			waitEmbyProxyFlight(t, cache, "key", 2)
			cancelCaller()
			if (<-first).failure == nil {
				t.Fatal("取消等待者未退出")
			}
			if shutdown {
				closeService()
				if (<-second).failure == nil {
					t.Fatal("关闭服务未取消请求")
				}
				select {
				case <-stopped:
				case <-time.After(time.Second):
					t.Fatal("关闭服务未停止解析任务")
				}
			} else {
				close(release)
				if (<-second).failure != nil || calls.Load() != 1 {
					t.Fatal("单个等待者取消影响了其他等待者")
				}
			}
		})
	}
}

func TestEmbyProxyCacheLastWaiterAndCapacity(t *testing.T) {
	options := domain.DefaultEmbyProxyConfig()
	options.CacheCapacity = 1
	cache := newEmbyProxyCache(options)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, stopped, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		cache.resolve(ctx, context.Background(), "first", func(ctx context.Context) embyProxyLocationResult {
			close(started)
			<-ctx.Done()
			close(stopped)
			return embyProxyCanceledResult()
		})
	}()
	<-started
	result := cache.resolve(context.Background(), context.Background(), "second", func(context.Context) embyProxyLocationResult {
		t.Error("任务容量限制未生效")
		return embyProxyLocationResult{}
	})
	if result.failure == nil || result.failure.reason != "resolution_busy" {
		t.Fatal("并发满容量未拒绝")
	}
	cancel()
	<-done
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("最后一个等待者取消未停止任务")
	}
}

func TestEmbyProxyCacheDoesNotStoreFailureOrLocalMedia(t *testing.T) {
	cache := newEmbyProxyCache(domain.DefaultEmbyProxyConfig())
	for _, result := range []embyProxyLocationResult{{}, {location: embyProxyTestMediaURL, failure: newEmbyProxyFailure("probe", "invalid_media_type", "失败", 200, nil)}} {
		for index := 0; index < 2; index++ {
			resolved := cache.resolve(context.Background(), context.Background(), "key", func(context.Context) embyProxyLocationResult { return result })
			if resolved.failure != nil {
				resolved.failure.reason = "changed"
				if result.failure.reason == "changed" {
					t.Fatal("失败结果共享可变指针")
				}
			}
		}
	}
	if len(cache.entries) != 0 {
		t.Fatal("失败或非 STRM 结果被缓存")
	}
}
