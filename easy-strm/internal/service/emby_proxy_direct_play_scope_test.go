package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"easy-strm/internal/domain"
)

// 强制直接播放只应作用于 STRM（远程）媒体源；本地视频必须保留 Emby 的转码能力。

type directPlayScopeUpstream struct {
	kind     string // "local" / "strm" / "broken"
	mu       sync.Mutex
	lookups  int
	playBody []byte
}

func (u *directPlayScopeUpstream) lookupsCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lookups
}

func (u *directPlayScopeUpstream) forwardedBody() []byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]byte(nil), u.playBody...)
}

func newDirectPlayScopeProxy(t *testing.T, kind string) (*httptest.Server, *directPlayScopeUpstream) {
	t.Helper()
	state := &directPlayScopeUpstream{kind: kind}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/Items") {
			state.mu.Lock()
			state.lookups++
			state.mu.Unlock()
			if state.kind == "broken" {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			path := "/media/movie.mkv"
			if state.kind == "strm" {
				path = "/media/movie.strm"
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, `{"Items":[{"Id":"`+request.URL.Query().Get("Ids")+`","Path":"`+path+`","MediaSources":[{"Id":"s1","Path":"`+path+`"}]}]}`)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("读取上游请求失败：%v", err)
		}
		state.mu.Lock()
		state.playBody = body
		state.mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Length", strconv.Itoa(len(`{"MediaSources":[{"Id":"s1","IsRemote":true,"SupportsDirectPlay":true,"SupportsTranscoding":true,"SupportsDirectStream":true,"TranscodingUrl":"https://emby/transcode"}]}`)))
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `{"MediaSources":[{"Id":"s1","IsRemote":true,"SupportsDirectPlay":true,"SupportsTranscoding":true,"SupportsDirectStream":true,"TranscodingUrl":"https://emby/transcode"}]}`)
	}))
	t.Cleanup(upstream.Close)
	options := domain.DefaultEmbyProxyConfig()
	options.ForceDirectPlay = true
	service, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	proxy := httptest.NewServer(service.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}))
	t.Cleanup(proxy.Close)
	return proxy, state
}

const directPlayScopeRequestBody = `{"DeviceProfile":{"EnableTranscoding":true,"TranscodingProfiles":["hls"],"MaxStreamingBitrate":1000000}}`

func sendDirectPlayScopeRequest(t *testing.T, proxy *httptest.Server) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, proxy.URL+"/Items/42/PlaybackInfo", strings.NewReader(directPlayScopeRequestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := proxy.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestDirectPlayScopeSkipsLocalMedia(t *testing.T) {
	proxy, state := newDirectPlayScopeProxy(t, "local")
	sendDirectPlayScopeRequest(t, proxy)
	if body := string(state.forwardedBody()); body != directPlayScopeRequestBody {
		t.Fatalf("本地视频的 PlaybackInfo 不应被改写，实际转发体=%s", body)
	}
}

func TestDirectPlayScopeRewritesRemoteMedia(t *testing.T) {
	proxy, state := newDirectPlayScopeProxy(t, "strm")
	sendDirectPlayScopeRequest(t, proxy)
	body := string(state.forwardedBody())
	for _, want := range []string{`"EnableTranscoding":false`, `"TranscodingProfiles":[]`, `"EnableDirectPlay":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("STRM 的 PlaybackInfo 应被改写，缺少 %s：%s", want, body)
		}
	}
}

func TestDirectPlayScopeKeepsOriginalWhenKindUnknown(t *testing.T) {
	proxy, state := newDirectPlayScopeProxy(t, "broken")
	sendDirectPlayScopeRequest(t, proxy)
	if body := string(state.forwardedBody()); body != directPlayScopeRequestBody {
		t.Fatalf("媒体类型无法判定时应保留原始请求，实际=%s", body)
	}
}

func TestDirectPlayScopeCachesItemKind(t *testing.T) {
	proxy, state := newDirectPlayScopeProxy(t, "local")
	sendDirectPlayScopeRequest(t, proxy)
	sendDirectPlayScopeRequest(t, proxy)
	if got := state.lookupsCount(); got != 1 {
		t.Fatalf("同一 item 的媒体类型应被缓存，实际回查 %d 次", got)
	}
}

func TestEmbyDirectPlayPathPrefix(t *testing.T) {
	cases := map[string]string{
		"/Items/42/PlaybackInfo":            "",
		"/emby/Items/42/PlaybackInfo":        "/emby",
		"/EMBY/Items/42/PlaybackInfo":        "/EMBY",
		"/emby/embyother/Items/42/Playback": "/emby",
		"/Emby/Items":                        "/Emby",
	}
	for path, want := range cases {
		if got := embyDirectPlayPathPrefix(path); got != want {
			t.Fatalf("embyDirectPlayPathPrefix(%q)=%q, want %q", path, got, want)
		}
	}
}
