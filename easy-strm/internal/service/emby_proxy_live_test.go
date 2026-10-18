package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

const liveSourceJSON = `{"Id":"s1","Path":"http://tuner.invalid/channel","Protocol":"Http","IsRemote":true,"IsInfiniteStream":true,"RequiresOpening":true,"RequiresClosing":true,"LiveStreamId":"upstream-session","SupportsTranscoding":true,"TranscodingUrl":"/Videos/42/master.m3u8"}`

func TestLiveSourceClassification(t *testing.T) {
	for _, marker := range []string{`"IsInfiniteStream":true`, `"RequiresOpening":true`, `"RequiresClosing":true`, `"LiveStreamId":"session"`} {
		var source embyProxyMediaSource
		if err := json.Unmarshal([]byte(`{"Path":"http://tuner.invalid/live","Protocol":"Http",`+marker+`}`), &source); err != nil {
			t.Fatal(err)
		}
		if embyProxySourceIsSTRM(source) {
			t.Errorf("live source classified as STRM: %s", marker)
		}
	}
}

func TestLivePlaybackInfoNegotiation(t *testing.T) {
	for _, metadata := range []string{
		`{"Items":[{"Id":"42","Type":"LiveTvChannel","Path":"http://tuner.invalid/live"}]}`,
		`{"Items":[{"Id":"42","MediaSources":[` + liveSourceJSON + `]}]}`,
	} {
		t.Run(metadata, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/Items") {
					calls.Add(1)
					io.WriteString(w, metadata)
					return
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != directPlayScopeRequestBody {
					t.Errorf("live negotiation rewritten: %s", body)
				}
				io.WriteString(w, `{"MediaSources":[`+liveSourceJSON+`]}`)
			}))
			defer upstream.Close()
			options := domain.DefaultEmbyProxyConfig()
			options.ForceDirectPlay = true
			s, err := NewEmbyProxyServiceWithConfig(nil, upstream.Client(), options)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			h := s.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL})
			for i := 0; i < 2; i++ {
				r := httptest.NewRequest("POST", "/emby/Items/42/PlaybackInfo", strings.NewReader(directPlayScopeRequestBody))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 200 || w.Body.String() != `{"MediaSources":[`+liveSourceJSON+`]}` {
					t.Errorf("live response rewritten: %d %s", w.Code, w.Body.String())
				}
			}
			if calls.Load() != 2 || len(s.directPlayCache) != 0 || len(s.cache.entries) != 0 {
				t.Errorf("live state cached: lookups=%d verdicts=%d locations=%d", calls.Load(), len(s.directPlayCache), len(s.cache.entries))
			}
		})
	}
}

func TestLiveVideoRoutesAndVODPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, source, item string
		want               int
	}{
		{"lifecycle", liveSourceJSON, `{"Id":"42","Type":"LiveTvChannel"}`, 200},
		{"channel type", `{"Id":"s1","Path":"http://tuner.invalid/live","Protocol":"Http"}`, `{"Id":"42","Type":"LiveTvChannel"}`, 200},
		{"unopened channel", `{"Id":"s1","RequiresOpening":true}`, `{"Id":"42","Type":"LiveTvChannel"}`, 200},
		{"VOD", `{"Id":"s1","Path":"https://cdn.example/movie"}`, `{"Id":"42","Type":"Movie","Path":"/media/movie.strm"}`, 502},
		{"VOD mixed sources", `{"Id":"s1","Path":"https://cdn.example/movie"}`, `{"Id":"42","Path":"/media/movie.strm","MediaSources":[{"Id":"other","RequiresOpening":true}]}`, 502},
		{"unknown", `{"Id":"s1"}`, `{"Id":"42"}`, 502},
	} {
		for _, path := range []string{"/emby/videos/42/stream.ts", "/Videos/42/master.m3u8", "/emby/Videos/42/hls1/main/0.ts", "/Videos/42/hls/main/0.ts", "/emby/videos/42/live"} {
			t.Run(tc.name+path, func(t *testing.T) {
				contentType, content := "video/mp2t", "live bytes"
				if strings.HasSuffix(path, "m3u8") {
					contentType, content = "application/vnd.apple.mpegurl", "#EXTM3U\n#EXT-X-TARGETDURATION:6\n"
				}
				var streams atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
						io.WriteString(w, `{"MediaSources":[`+tc.source+`]}`)
						return
					}
					if strings.HasSuffix(r.URL.Path, "/Items") {
						io.WriteString(w, `{"Items":[`+tc.item+`]}`)
						return
					}
					streams.Add(1)
					if r.URL.RequestURI() != path+"?LiveStreamId=client-forged&api_key=client&DeviceId=device" {
						t.Errorf("URI changed: %s", r.URL.RequestURI())
					}
					if r.Header.Get("Range") != "bytes=0-" || r.Header.Get("Accept") != "video/mp2t" || r.Header.Get("X-Emby-Token") != "user-token" {
						t.Errorf("headers changed: %v", r.Header)
					}
					w.Header().Set("Content-Type", contentType)
					io.WriteString(w, content)
				}))
				defer upstream.Close()
				s := NewEmbyProxyService(nil, upstream.Client())
				defer s.Close()
				if tc.want == 200 {
					s.options.ProbeHEAD, s.options.ResolveFinalURL = true, true
				}
				s.relayClient.Transport = embyProxyRelayTestTransport(func(*http.Request) (*http.Response, error) {
					t.Error("live tuner must not be probed or relayed directly")
					return nil, errors.New("unexpected media request")
				})
				h := s.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL})
				want := tc.want
				if strings.HasPrefix(tc.name, "VOD") && strings.HasSuffix(path, "stream.ts") {
					want = 302
				}
				for i := 0; i < 2; i++ {
					r := httptest.NewRequest("GET", path+"?LiveStreamId=client-forged&api_key=client&DeviceId=device", nil)
					r.Header.Set("Range", "bytes=0-")
					r.Header.Set("Accept", "video/mp2t")
					r.Header.Set("X-Emby-Token", "user-token")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					if w.Code != want {
						t.Errorf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
					}
					if want == 200 && (w.Body.String() != content || w.Header().Get("Content-Type") != contentType || w.Header().Get("Location") != "") {
						t.Errorf("live not passed through: %v %s", w.Header(), w.Body.String())
					}
				}
				if want == 200 && (streams.Load() != 2 || len(s.cache.entries) != 0) {
					t.Errorf("live cached: streams=%d locations=%d", streams.Load(), len(s.cache.entries))
				}
				if want != 200 && streams.Load() != 0 {
					t.Error("VOD/unknown bypassed policy via client LiveStreamId")
				}
			})
		}
	}
}

func TestLiveStreamFlushAndCancel(t *testing.T) {
	for _, path := range []string{"/emby/Videos/42/stream.ts", "/emby/videos/42/live", "/emby/LiveTv/LiveStreamFiles/session/stream.ts"} {
		t.Run(path, func(t *testing.T) {
			canceled := make(chan struct{})
			release := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
					io.WriteString(w, `{"MediaSources":[`+liveSourceJSON+`]}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/Items") {
					io.WriteString(w, `{"Items":[{"Id":"42","Type":"LiveTvChannel"}]}`)
					return
				}
				w.Header().Set("Content-Type", "video/mp2t")
				io.WriteString(w, "first")
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					close(canceled)
				case <-release:
				}
			}))
			defer upstream.Close()
			defer close(release)
			metadataClient := upstream.Client()
			metadataClient.Timeout = time.Second
			s := NewEmbyProxyService(nil, metadataClient)
			defer s.Close()
			proxy := httptest.NewServer(s.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}))
			defer proxy.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r, _ := http.NewRequestWithContext(ctx, "GET", proxy.URL+path, nil)
			client := proxy.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			resp, err := client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("status=%d", resp.StatusCode)
			}
			data := make([]byte, 5)
			if _, err := io.ReadFull(resp.Body, data); err != nil || string(data) != "first" {
				t.Fatalf("first bytes before EOF: %q %v", data, err)
			}
			if resp.ContentLength != -1 || len(resp.TransferEncoding) != 1 || resp.TransferEncoding[0] != "chunked" {
				t.Fatalf("stream framing: %+v", resp)
			}
			// 流传输不得继承元数据客户端的总超时，取消仍须传播至上游。
			select {
			case <-canceled:
				t.Fatal("live stream timed out with metadata client")
			case <-time.After(1100 * time.Millisecond):
			}
			cancel()
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("upstream not canceled")
			}
		})
	}
}

func TestLiveMetadataIdentityAndFailures(t *testing.T) {
	for _, auth := range []struct{ query, header, value string }{
		{"ApiKey=client", "", ""}, {"token=client", "", ""}, {"api_key=client", "", ""},
		{"AccessToken=client", "", ""}, {"", "Authorization", `MediaBrowser Token="client", DeviceId="device"`},
		{"", "X-Emby-Authorization", `MediaBrowser Token="client"`}, {"", "X-Emby-Token", "client"},
		{"", "X-MediaBrowser-Token", "client"},
	} {
		t.Run(auth.query+auth.header, func(t *testing.T) {
			var denied atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if auth.header != "" && r.Header.Get(auth.header) != auth.value {
					t.Error("auth header lost")
				}
				if r.Header.Get("X-Emby-Token") == "admin" {
					t.Error("client identity replaced by admin")
				}
				if r.Header.Get("X-Emby-Device-Id") != "device" || r.URL.Query().Get("DeviceId") != "device" {
					t.Error("device identity lost")
				}
				if auth.query != "" && !strings.Contains(r.URL.RawQuery, auth.query) {
					t.Error("auth query lost")
				}
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") || strings.HasSuffix(r.URL.Path, "/Items") {
					if r.Header.Get("Accept") != "application/json" || r.Header.Get("Range") != "" {
						t.Error("metadata received media headers")
					}
					if denied.Load() {
						w.WriteHeader(401)
						return
					}
					if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
						io.WriteString(w, `{"MediaSources":[`+liveSourceJSON+`]}`)
					} else {
						io.WriteString(w, `{"Items":[{"Id":"42","Type":"LiveTvChannel"}]}`)
					}
					return
				}
				io.WriteString(w, "live")
			}))
			defer upstream.Close()
			s := NewEmbyProxyService(nil, upstream.Client())
			defer s.Close()
			h := s.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL, APIKey: "admin"})
			for _, want := range []int{200, 401} {
				r := httptest.NewRequest("GET", "/Videos/42/stream.ts?DeviceId=device&"+auth.query, nil)
				if auth.header != "" {
					r.Header.Set(auth.header, auth.value)
				}
				r.Header.Set("X-Emby-Device-Id", "device")
				r.Header.Set("Range", "bytes=0-")
				r.Header.Set("Accept", "video/mp2t")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != want {
					t.Errorf("status=%d want=%d", w.Code, want)
				}
				denied.Store(true)
			}
		})
	}
}

func TestLiveResponseDefenseAfterSTRMVerdict(t *testing.T) {
	body := `{"MediaSources":[` + liveSourceJSON + `]}`
	r := httptest.NewRequest("POST", "/Items/42/PlaybackInfo", nil)
	response := &http.Response{Request: r, StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
	forceEmbyDirectPlayResponse(response)
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil || string(got) != body {
		t.Fatalf("live response changed after stale verdict: %s %v", got, err)
	}
}

func TestLiveControlRoutesAndHEAD(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/emby/LiveTv/Channels", ""},
		{"POST", "/emby/LiveTv/LiveStreams/Open", `{"OpenToken":"session"}`},
		{"POST", "/LiveTv/LiveStreams/Close", `{"LiveStreamId":"session"}`},
		{"HEAD", "/emby/Videos/42/stream.ts", ""},
		{"HEAD", "/emby/Videos/42/live", ""},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			var forwarded atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
					io.WriteString(w, `{"MediaSources":[`+liveSourceJSON+`]}`)
					return
				}
				if r.Method != tc.method || r.URL.Path != tc.path || r.URL.Query().Get("ApiKey") != "client" {
					t.Errorf("request changed: %s %s", r.Method, r.URL)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != tc.body {
					t.Errorf("body changed: %s %v", body, err)
				}
				forwarded.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				io.WriteString(w, `{"ok":true}`)
			}))
			defer upstream.Close()
			s := NewEmbyProxyService(nil, upstream.Client())
			defer s.Close()
			s.options.ForceDirectPlay = true
			h := s.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL})
			r := httptest.NewRequest(tc.method, tc.path+"?ApiKey=client", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 || forwarded.Load() != 1 || w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("passthrough failed: %d %v", w.Code, w.Header())
			}
			if tc.method == "HEAD" && w.Body.Len() != 0 {
				t.Error("HEAD body must be empty")
			}
			if tc.method != "HEAD" && w.Body.String() != `{"ok":true}` {
				t.Errorf("body=%s", w.Body.String())
			}
		})
	}
}

func TestLiveQueryCannotBypassUnknownHTTPMetadata(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   int
	}{
		{200, `{"Items":[]}`, 502},
		{200, `{"Items":[{"Id":"other","Type":"LiveTvChannel"}]}`, 502},
		{200, `{`, 502},
		{401, ``, 401}, {403, ``, 403}, {503, ``, 502},
	} {
		t.Run(http.StatusText(tc.status)+tc.body, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/PlaybackInfo") {
					io.WriteString(w, `{"MediaSources":[{"Id":"s1","Path":"http://tuner.invalid/live"}]}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/Items") {
					w.WriteHeader(tc.status)
					io.WriteString(w, tc.body)
					return
				}
				t.Error("unknown type forwarded to stream")
			}))
			defer upstream.Close()
			s := NewEmbyProxyService(nil, upstream.Client())
			defer s.Close()
			s.options.ProxyErrorStrategy = "fallback_origin"
			w := httptest.NewRecorder()
			s.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL}).ServeHTTP(w, httptest.NewRequest("GET", "/Videos/42/stream.ts?LiveStreamId=forged", nil))
			if w.Code != tc.want || w.Header().Get("Location") != "" || len(s.cache.entries) != 0 {
				t.Errorf("unknown type bypass: %d %v", w.Code, w.Header())
			}
		})
	}
}
