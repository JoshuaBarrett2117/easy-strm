package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"easy-strm/internal/domain"
)

func TestEmbyProxyEnhanceCacheSkipsMetadata(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if serveEmbyProxySTRMItemFixture(writer, request) {
			return
		}
		calls.Add(1)
		io.WriteString(writer, `{"MediaSources":[{"Id":"s1","Path":"https://8.8.8.8/movie"}]}`)
	}))
	defer upstream.Close()
	service := NewEmbyProxyService(nil, upstream.Client())
	defer service.Close()
	handler := service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL, APIKey: "test"})
	var workers sync.WaitGroup
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func(session int) {
			defer workers.Done()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/Videos/42/stream?PlaySessionId=%d", session), nil))
			if response.Code != 302 {
				t.Errorf("响应=%d", response.Code)
			}
		}(index)
	}
	workers.Wait()
	if calls.Load() != 1 {
		t.Fatalf("缓存未跳过元数据：%d", calls.Load())
	}
}

func TestEmbyProxyEnhanceTranscodeBypassesCacheAndAuthorization(t *testing.T) {
	var calls, streamCalls atomic.Int32
	var denied atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if serveEmbyProxySTRMItemFixture(writer, request) {
			return
		}
		if !strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
			streamCalls.Add(1)
			return
		}
		calls.Add(1)
		if denied.Load() {
			writer.WriteHeader(401)
			return
		}
		io.WriteString(writer, `{"MediaSources":[{"Id":"s1","Path":"https://8.8.8.8/movie"}]}`)
	}))
	defer upstream.Close()
	service := NewEmbyProxyService(nil, upstream.Client())
	defer service.Close()
	handler := service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL, APIKey: "test"})
	for _, path := range []string{"/Videos/42/stream", "/Videos/42/stream?Static=false", "/Videos/42/master.m3u8", "/Videos/42/hls1/main/0.ts"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if path == "/Videos/42/stream" {
			if response.Code != 302 {
				t.Fatal("直播放未成功")
			}
			denied.Store(true)
		} else if response.Code != 401 {
			t.Fatalf("转码复用了直链缓存或绕过授权：%s %d", path, response.Code)
		}
	}
	if calls.Load() != 4 || streamCalls.Load() != 0 {
		t.Fatalf("元数据=%d 回源=%d", calls.Load(), streamCalls.Load())
	}
}

func TestEmbyProxyEnhanceSingleflightIncludesItems(t *testing.T) {
	var playbackCalls, itemCalls atomic.Int32
	release, entered := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/PlaybackInfo") {
			playbackCalls.Add(1)
			io.WriteString(writer, `{"MediaSources":[{"Id":"s1","Path":"/media/movie.mkv"}]}`)
			return
		}
		itemCalls.Add(1)
		close(entered)
		<-release
		io.WriteString(writer, `{"Items":[{"Id":"42","Path":"/media/movie.strm","IsShortcut":true}]}`)
	}))
	defer upstream.Close()
	service := NewEmbyProxyService(nil, upstream.Client())
	defer service.Close()
	server := &domain.EmbyServer{BaseURL: upstream.URL, APIKey: "test"}
	base, _ := url.Parse(upstream.URL)
	request := httptest.NewRequest("GET", "/Videos/42/stream", nil)
	playback, _ := embyPlaybackRequest(request)
	key := service.embyProxyCacheKey(server, request, playback, 1)
	results := make(chan embyProxyLocationResult, 12)
	go func() { results <- service.resolvePlaybackLocation(server, base, request, playback, 1) }()
	<-entered
	for index := 1; index < 12; index++ {
		go func() {
			results <- service.resolvePlaybackLocation(server, base, request.Clone(context.Background()), playback, 1)
		}()
	}
	waitEmbyProxyFlight(t, service.cache, key, 12)
	close(release)
	for index := 0; index < 12; index++ {
		if (<-results).failure == nil {
			t.Fatal("缺失直链不应缓存")
		}
	}
	if playbackCalls.Load() != 1 || itemCalls.Load() != 1 || len(service.cache.entries) != 0 {
		t.Fatalf("同键未合并完整解析：PlaybackInfo=%d Items=%d", playbackCalls.Load(), itemCalls.Load())
	}
}

func TestEmbyProxyEnhanceHandlerGeneration(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if serveEmbyProxySTRMItemFixture(writer, request) {
			return
		}
		calls.Add(1)
		io.WriteString(writer, `{"MediaSources":[{"Id":"s1","Path":"https://8.8.8.8/movie"}]}`)
	}))
	defer upstream.Close()
	service := NewEmbyProxyService(nil, upstream.Client())
	defer service.Close()
	server := &domain.EmbyServer{BaseURL: upstream.URL, APIKey: "test"}
	for index := 0; index < 2; index++ {
		handler := service.buildHandler(server)
		for repeat := 0; repeat < 2; repeat++ {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/Videos/42/stream", nil))
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("配置更新代次未隔离：%d", calls.Load())
	}
}
