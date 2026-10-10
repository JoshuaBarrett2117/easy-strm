package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type proxyRoundTripFunc func(*http.Request) (*http.Response, error)

func (f proxyRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testProxyFallback(primary, direct http.RoundTripper) *proxyFallbackTransport {
	return &proxyFallbackTransport{
		proxy: primary, direct: direct, tmdbDirect: direct,
		loadProxyConfig: func() (*url.URL, []string) {
			return &url.URL{Scheme: "http", Host: "127.0.0.1:7890"}, buildProxyDomains("custom.example.com")
		},
	}
}

func TestProxyFallbackForAllProxiedHosts(test *testing.T) {
	fallback := testProxyFallback(
		proxyRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("proxy EOF") }),
		proxyRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
		}),
	)
	for _, host := range []string{"api.themoviedb.org", "api.themoviedb.org.", "api.telegram.org", "t.me", "github.com", "api.github.com", "raw.githubusercontent.com", "gist.github.com", "image.tmdb.org", "custom.example.com"} {
		req, _ := http.NewRequest(http.MethodGet, "https://"+host+"/3/search/tv", nil)
		resp, err := fallback.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			test.Errorf("proxied host should fallback: host=%s resp=%v err=%v", host, resp, err)
			continue
		}
		resp.Body.Close()
	}
}

func TestBuildProxyDomainsIncludesBuiltInSites(test *testing.T) {
	domains := buildProxyDomains("")

	for _, host := range []string{
		"api.telegram.org",
		"t.me",
		"github.com",
		"api.github.com",
		"api.themoviedb.org",
		"image.tmdb.org",
		"raw.githubusercontent.com",
		"gist.github.com",
	} {
		if !shouldProxyHost(host, domains) {
			test.Errorf("内置站点 %s 应默认走代理，domains=%v", host, domains)
		}
	}

	if shouldProxyHost("example.com", domains) {
		test.Fatalf("非内置站点不应默认走代理，domains=%v", domains)
	}
}

func TestProxyFallbackReplaysBody(test *testing.T) {
	fallback := testProxyFallback(
		proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			io.ReadAll(req.Body)
			req.Body.Close()
			return nil, errors.New("proxy EOF after sending body")
		}),
		proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil || string(body) != "payload" {
				test.Errorf("fallback body=%q err=%v, want complete payload", body, err)
			}
			req.Body.Close()
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}),
	)
	req, err := http.NewRequest(http.MethodPost, "https://api.themoviedb.org", strings.NewReader("payload"))
	if err != nil {
		test.Fatal(err)
	}
	if _, err := fallback.RoundTrip(req); err != nil {
		test.Fatal(err)
	}
}

func TestProxyAliasScope(test *testing.T) {
	domains := buildProxyDomains("tg,telegram,github,tmdb, CUSTOM.EXAMPLE.COM,custom.example.com")
	defaults := make(map[string]bool)
	for _, domain := range buildProxyDomains("") {
		if defaults[domain] {
			test.Errorf("duplicate default domain: %s", domain)
		}
		defaults[domain] = true
	}
	seen := make(map[string]bool)
	for _, domain := range domains {
		if seen[domain] {
			test.Errorf("duplicate domain: %s", domain)
		}
		seen[domain] = true
	}
	for name, aliases := range proxyDomainAlias {
		for _, alias := range aliases {
			if !seen[alias] || !defaults[alias] {
				test.Errorf("missing %s alias %s", name, alias)
			}
		}
	}
	proxyURL := &url.URL{Scheme: "http", Host: "127.0.0.1:7890"}
	for _, host := range []string{
		"115.com", "webapi.115.com", "115cdn.net", "cdnfhnfile.115cdn.net", "192.168.31.12",
		"localhost", "emby.local", "emby.lan", "::1", "tmdb.org", "www.tmdb.org", "other.tmdb.org",
		"eviltelegram.org", "telegram.org.evil.com", "evilt.me", "t.me.evil.com",
		"evilgithub.com", "github.com.evil.com", "raw.githubusercontent.com.evil.com",
		"evilthemoviedb.org", "api.themoviedb.org.evil.com", "evilimage.tmdb.org", "image.tmdb.org.evil.com",
	} {
		if shouldUseProxy(proxyURL, host, domains) {
			test.Errorf("%s must stay direct", host)
		}
	}
	for _, host := range []string{"api.themoviedb.org", "image.tmdb.org", "api.telegram.org", "t.me", "github.com", "api.github.com", "raw.githubusercontent.com", "gist.github.com", "custom.example.com", "API.GITHUB.COM."} {
		if !shouldUseProxy(proxyURL, host, domains) || shouldUseProxy(nil, host, domains) {
			test.Errorf("incorrect proxy selection for %s", host)
		}
	}
}

func TestIsTMDBHostSemantics(test *testing.T) {
	for host, want := range map[string]bool{
		"api.themoviedb.org": true, "API.THEMOVIEDB.ORG.": true, "other.themoviedb.org": true,
		"themoviedb.org": false, "image.tmdb.org": false, "tmdb.org": false,
		"evilthemoviedb.org": false, "api.themoviedb.org.evil.com": false,
	} {
		if got := isTMDBHost(host); got != want {
			test.Errorf("isTMDBHost(%q)=%v, want %v", host, got, want)
		}
	}
}

func TestProxyFallbackDirectOnlyAndDisabled(test *testing.T) {
	for _, host := range []string{"webapi.115.com", "cdnfhnfile.115cdn.net", "192.168.31.12", "emby.local", "api.themoviedb.org"} {
		test.Run(host, func(test *testing.T) {
			var directCalls int
			failure := errors.New("direct failed")
			transport := testProxyFallback(
				proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
					test.Fatal("direct-only request reached proxy")
					return nil, nil
				}),
				proxyRoundTripFunc(func(*http.Request) (*http.Response, error) { directCalls++; return nil, failure }),
			)
			if isTMDBHost(host) {
				transport.loadProxyConfig = func() (*url.URL, []string) { return nil, buildProxyDomains("") }
			}
			req, _ := http.NewRequest(http.MethodGet, "http://"+host, nil)
			if _, err := transport.RoundTrip(req); !errors.Is(err, failure) || directCalls != 1 {
				test.Fatalf("err=%v direct calls=%d, want original failure once", err, directCalls)
			}
		})
	}
}

type proxyTestBody struct {
	io.Reader
	closed bool
}

func (body *proxyTestBody) Close() error { body.closed = true; return nil }

func TestProxyFallbackFailureAndBodyCleanup(test *testing.T) {
	var proxyCalls, directCalls int
	proxyBody := &proxyTestBody{Reader: strings.NewReader("bad proxy")}
	directBody := &proxyTestBody{Reader: strings.NewReader("bad direct")}
	directError := errors.New("direct failed")
	transport := testProxyFallback(
		proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
			proxyCalls++
			return &http.Response{Body: proxyBody}, errors.New("proxy failed")
		}),
		proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
			directCalls++
			return &http.Response{Body: directBody}, directError
		}),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://github.com", nil)
	resp, err := transport.RoundTrip(req)
	if resp != nil || !errors.Is(err, directError) || proxyCalls != 1 || directCalls != 1 || !proxyBody.closed || !directBody.closed {
		test.Fatalf("resp=%v err=%v calls=%d/%d bodies closed=%v/%v", resp, err, proxyCalls, directCalls, proxyBody.closed, directBody.closed)
	}
}

func TestProxyFallbackUnreplayableBody(test *testing.T) {
	for _, replay := range []string{"absent", "error"} {
		test.Run(replay, func(test *testing.T) {
			failure := errors.New("proxy failed after partial write")
			transport := testProxyFallback(
				proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					buffer := make([]byte, 2)
					req.Body.Read(buffer)
					req.Body.Close()
					return nil, failure
				}),
				proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
					test.Fatal("must not replay partial or empty body")
					return nil, nil
				}),
			)
			req, _ := http.NewRequest(http.MethodPost, "https://github.com", io.NopCloser(strings.NewReader("payload")))
			if replay == "error" {
				req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("replay failed") }
			}
			if resp, err := transport.RoundTrip(req); resp != nil || err == nil {
				test.Fatalf("resp=%v err=%v, want failure", resp, err)
			}
		})
	}
}

func TestProxyFallbackTMDBSelectionAndEmptyBody(test *testing.T) {
	for _, host := range []string{"api.themoviedb.org", "other.themoviedb.org", "image.tmdb.org", "api.telegram.org", "github.com"} {
		test.Run(host, func(test *testing.T) {
			var normalCalls, ipv4Calls int
			transport := testProxyFallback(
				proxyRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("proxy failed") }),
				proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					normalCalls++
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
				}),
			)
			transport.tmdbDirect = proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				ipv4Calls++
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
			})
			for _, body := range []io.Reader{nil, http.NoBody} {
				req, _ := http.NewRequest(http.MethodPost, "https://"+host, body)
				if _, err := transport.RoundTrip(req); err != nil {
					test.Fatal(err)
				}
			}
			if isTMDBHost(host) {
				if ipv4Calls != 2 || normalCalls != 0 {
					test.Fatalf("IPv4=%d normal=%d", ipv4Calls, normalCalls)
				}
			} else if ipv4Calls != 0 || normalCalls != 2 {
				test.Fatalf("IPv4=%d normal=%d", ipv4Calls, normalCalls)
			}
		})
	}
}

func testProxyClient(test *testing.T, origin, proxy *httptest.Server, loader func() (*url.URL, []string), headerTimeout time.Duration) *http.Client {
	test.Helper()
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		target := origin.Listener.Addr().String()
		if address == proxy.Listener.Addr().String() {
			target = address
		}
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}
	if loader == nil {
		proxyURL, err := url.Parse(proxy.URL)
		if err != nil {
			test.Fatal(err)
		}
		loader = func() (*url.URL, []string) { return proxyURL, buildProxyDomains("") }
	}
	client := newProxyAwareHTTPClient(3*time.Second, loader, base)
	transport := client.Transport.(*proxyFallbackTransport)
	configureProxyTransport(transport.proxy.(*http.Transport), base.DialContext, time.Second, headerTimeout)
	test.Cleanup(client.CloseIdleConnections)
	return client
}

func testProxyServer(test *testing.T, handler http.HandlerFunc) *httptest.Server {
	test.Helper()
	server := httptest.NewServer(handler)
	test.Cleanup(server.Close)
	return server
}

func waitProxySignal(test *testing.T, signal <-chan struct{}) {
	test.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		test.Fatal("timed out waiting for local server")
	}
}

func TestProxyTransportStallsFallback(test *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		test.Run(scheme, func(test *testing.T) {
			var originCalls, proxyCalls atomic.Int32
			originHandler := http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				originCalls.Add(1)
				io.WriteString(writer, "direct")
			})
			origin := httptest.NewServer(originHandler)
			if scheme == "https" {
				origin.Close()
				origin = httptest.NewTLSServer(originHandler)
			}
			test.Cleanup(origin.Close)
			release, stopped := make(chan struct{}), make(chan struct{})
			defer close(release)
			proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
				proxyCalls.Add(1)
				if (scheme == "https") != (req.Method == http.MethodConnect) {
					test.Errorf("unexpected proxy method %s for %s", req.Method, scheme)
				}
				select {
				case <-req.Context().Done():
				case <-release:
				}
				close(stopped)
			})
			client := testProxyClient(test, origin, proxy, nil, 30*time.Millisecond)
			resp, err := client.Get(scheme + "://github.com/stall")
			if err != nil {
				test.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(body) != "direct" || proxyCalls.Load() != 1 || originCalls.Load() != 1 {
				test.Fatalf("body=%q err=%v proxy=%d direct=%d", body, err, proxyCalls.Load(), originCalls.Load())
			}
			waitProxySignal(test, stopped)
		})
	}
}

func TestProxyTransportConnectErrorAndRouteSnapshot(test *testing.T) {
	var originCalls, proxyCalls, loads atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		originCalls.Add(1)
		io.WriteString(writer, "direct")
	}))
	test.Cleanup(origin.Close)
	proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
		proxyCalls.Add(1)
		writer.WriteHeader(http.StatusBadGateway)
	})
	proxyURL, _ := url.Parse(proxy.URL)
	client := testProxyClient(test, origin, proxy, func() (*url.URL, []string) {
		if loads.Add(1) != 1 {
			return nil, nil
		}
		return proxyURL, buildProxyDomains("")
	}, time.Second)
	resp, err := client.Get("https://api.telegram.org/test")
	if err != nil {
		test.Fatal(err)
	}
	resp.Body.Close()
	if loads.Load() != 1 || proxyCalls.Load() != 1 || originCalls.Load() != 1 {
		test.Fatalf("loads=%d proxy=%d direct=%d, want 1/1/1", loads.Load(), proxyCalls.Load(), originCalls.Load())
	}
}

func TestProxyTransportDialErrorFallbackOnce(test *testing.T) {
	var originCalls, proxyDials atomic.Int32
	origin := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
		originCalls.Add(1)
		io.WriteString(writer, "direct")
	})
	proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) { test.Error("failed dial reached proxy") })
	client := testProxyClient(test, origin, proxy, nil, time.Second)
	transport := client.Transport.(*proxyFallbackTransport)
	configureProxyTransport(transport.proxy.(*http.Transport), func(context.Context, string, string) (net.Conn, error) {
		proxyDials.Add(1)
		return nil, errors.New("offline proxy connect error")
	}, time.Second, time.Second)
	resp, err := client.Get("http://raw.githubusercontent.com/test")
	if err != nil {
		test.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(body) != "direct" || originCalls.Load() != 1 || proxyDials.Load() != 1 {
		test.Fatalf("body=%q err=%v direct=%d proxy dials=%d", body, err, originCalls.Load(), proxyDials.Load())
	}
}

func TestProxyTransportDirectPreservesHeaderWait(test *testing.T) {
	var originCalls, proxyCalls atomic.Int32
	const headerTimeout = 10 * time.Millisecond
	origin := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
		originCalls.Add(1)
		<-time.After(3 * headerTimeout)
		io.WriteString(writer, "115 direct")
	})
	proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) { proxyCalls.Add(1) })
	client := testProxyClient(test, origin, proxy, nil, headerTimeout)
	resp, err := client.Get("http://webapi.115.com/slow")
	if err != nil {
		test.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(body) != "115 direct" || originCalls.Load() != 1 || proxyCalls.Load() != 0 {
		test.Fatalf("body=%q err=%v direct=%d proxy=%d", body, err, originCalls.Load(), proxyCalls.Load())
	}
}

func TestProxyTransportBodyReplay(test *testing.T) {
	for _, replayable := range []bool{true, false} {
		name := "unreplayable"
		if replayable {
			name = "replayable"
		}
		test.Run(name, func(test *testing.T) {
			var originCalls, proxyCalls atomic.Int32
			checkBody := func(req *http.Request) {
				body, err := io.ReadAll(req.Body)
				if err != nil || string(body) != "complete payload" || req.Method != http.MethodPost || req.Header.Get("X-Test") != "preserved" {
					test.Errorf("method=%s body=%q header=%s err=%v", req.Method, body, req.Header.Get("X-Test"), err)
				}
			}
			origin := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
				originCalls.Add(1)
				checkBody(req)
				io.WriteString(writer, "direct")
			})
			proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
				proxyCalls.Add(1)
				checkBody(req)
				conn, _, err := writer.(http.Hijacker).Hijack()
				if err != nil {
					test.Error(err)
					return
				}
				conn.Close()
			})
			client := testProxyClient(test, origin, proxy, nil, time.Second)
			var reader io.Reader = strings.NewReader("complete payload")
			if !replayable {
				reader = io.NopCloser(reader)
			}
			req, _ := http.NewRequest(http.MethodPost, "http://github.com/submit", reader)
			req.Header.Set("X-Test", "preserved")
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			wantDirect := int32(0)
			if replayable {
				wantDirect = 1
			}
			if (err == nil) != replayable || proxyCalls.Load() != 1 || originCalls.Load() != wantDirect {
				test.Fatalf("err=%v proxy=%d direct=%d replayable=%v", err, proxyCalls.Load(), originCalls.Load(), replayable)
			}
		})
	}
}

func TestProxyTransportSuccessAndHTTPStatus(test *testing.T) {
	var originCalls, proxyCalls, loads atomic.Int32
	origin := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) { originCalls.Add(1) })
	proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
		proxyCalls.Add(1)
		if req.URL.Hostname() != "api.github.com" {
			test.Errorf("proxy URL=%s", req.URL)
		}
		status := http.StatusServiceUnavailable
		if req.URL.Query().Get("result") == "ok" {
			status = http.StatusOK
		}
		writer.WriteHeader(status)
		io.WriteString(writer, "proxy response")
	})
	proxyURL, _ := url.Parse(proxy.URL)
	client := testProxyClient(test, origin, proxy, func() (*url.URL, []string) { loads.Add(1); return proxyURL, buildProxyDomains("") }, time.Second)
	var group sync.WaitGroup
	const requests = 12
	for requestIndex := 0; requestIndex < requests; requestIndex++ {
		wantStatus := http.StatusServiceUnavailable
		requestURL := "http://api.github.com/test"
		if requestIndex%2 == 0 {
			wantStatus = http.StatusOK
			requestURL += "?result=ok"
		}
		group.Add(1)
		go func() {
			defer group.Done()
			resp, err := client.Get(requestURL)
			if err != nil {
				test.Error(err)
				return
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != wantStatus || string(body) != "proxy response" {
				test.Errorf("status=%d body=%q err=%v", resp.StatusCode, body, err)
			}
		}()
	}
	group.Wait()
	if loads.Load() != requests || proxyCalls.Load() != requests || originCalls.Load() != 0 {
		test.Fatalf("loads=%d proxy=%d direct=%d", loads.Load(), proxyCalls.Load(), originCalls.Load())
	}
}

func TestProxyTransportCancellationAndClientTimeout(test *testing.T) {
	for _, variant := range []struct{ scheme, mode string }{{"http", "cancel"}, {"http", "timeout"}, {"https", "cancel"}, {"https", "timeout"}} {
		test.Run(variant.scheme+"/"+variant.mode, func(test *testing.T) {
			var directCalls atomic.Int32
			origin := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) { directCalls.Add(1) })
			reached, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
			defer close(release)
			proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
				defer close(stopped)
				close(reached)
				select {
				case <-req.Context().Done():
				case <-release:
				}
			})
			client := testProxyClient(test, origin, proxy, nil, time.Second)
			if variant.mode == "timeout" {
				client.Timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, variant.scheme+"://github.com/stall", nil)
			result := make(chan error, 1)
			go func() {
				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				result <- err
			}()
			waitProxySignal(test, reached)
			want := context.DeadlineExceeded
			if variant.mode == "cancel" {
				cancel()
				want = context.Canceled
			}
			select {
			case err := <-result:
				if !errors.Is(err, want) || directCalls.Load() != 0 {
					test.Fatalf("err=%v direct=%d, want %v without fallback", err, directCalls.Load(), want)
				}
			case <-time.After(5 * time.Second):
				test.Fatal("client did not honor cancellation")
			}
			client.CloseIdleConnections()
			waitProxySignal(test, stopped)
		})
	}
}

func TestProxyFallbackKeepsTotalDeadline(test *testing.T) {
	var firstDeadline time.Time
	var directCalls int
	transport := testProxyFallback(
		proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			firstDeadline, _ = req.Context().Deadline()
			return nil, errors.New("proxy failed")
		}),
		proxyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			directCalls++
			deadline, ok := req.Context().Deadline()
			if !ok || !deadline.Equal(firstDeadline) {
				test.Errorf("deadline restarted: %v -> %v", firstDeadline, deadline)
			}
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	)
	client := &http.Client{Timeout: 30 * time.Millisecond, Transport: transport}
	_, err := client.Get("http://github.com")
	if !errors.Is(err, context.DeadlineExceeded) || directCalls != 1 || client.Timeout != 30*time.Millisecond {
		test.Fatalf("err=%v direct=%d timeout=%s", err, directCalls, client.Timeout)
	}
}

func TestProxyTransportConfigurationAndNetworks(test *testing.T) {
	if proxyDialTimeout != 10*time.Second || proxyResponseHeaderTimeout != 15*time.Second {
		test.Fatalf("proxy constants=%s/%s", proxyDialTimeout, proxyResponseHeaderTimeout)
	}
	var networks []string
	var dialDeadline time.Time
	base := http.DefaultTransport.(*http.Transport).Clone()
	dialError := errors.New("offline dial")
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		networks = append(networks, network)
		if deadline, ok := ctx.Deadline(); ok {
			dialDeadline = deadline
		}
		return nil, dialError
	}
	client := newProxyAwareHTTPClient(cloud115APITimeout, func() (*url.URL, []string) { return nil, nil }, base)
	test.Cleanup(client.CloseIdleConnections)
	transport := client.Transport.(*proxyFallbackTransport)
	proxy := transport.proxy.(*http.Transport)
	direct := transport.direct.(*http.Transport)
	tmdb := transport.tmdbDirect.(*http.Transport)
	if client.Timeout != cloud115APITimeout || proxy.ResponseHeaderTimeout != proxyResponseHeaderTimeout || direct.ResponseHeaderTimeout != base.ResponseHeaderTimeout || direct.TLSHandshakeTimeout != base.TLSHandshakeTimeout || direct.Proxy != nil || tmdb.Proxy != nil {
		test.Fatal("constructor changed client/direct timeout semantics or proxy configuration")
	}
	_, _ = direct.DialContext(context.Background(), "tcp", "webapi.115.com:80")
	_, _ = tmdb.DialContext(context.Background(), "tcp", "api.themoviedb.org:443")
	if strings.Join(networks, ",") != "tcp,tcp4" {
		test.Fatalf("dial networks=%v", networks)
	}
	before := time.Now()
	_, err := proxy.DialContext(context.Background(), "tcp", "127.0.0.1:7890")
	after := time.Now()
	if !errors.Is(err, dialError) || dialDeadline.Before(before.Add(proxyDialTimeout)) || dialDeadline.After(after.Add(proxyDialTimeout)) {
		test.Fatalf("dial err=%v deadline=%v, want invocation time + %s", err, dialDeadline, proxyDialTimeout)
	}
}

func TestProxyTransportDialStall(test *testing.T) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	configureProxyTransport(transport, func(ctx context.Context, network, address string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, 10*time.Millisecond, time.Second)
	_, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:7890")
	if !errors.Is(err, context.DeadlineExceeded) {
		test.Fatalf("dial stall err=%v", err)
	}
}

func TestProxyTransportStreaming(test *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		test.Run(scheme, func(test *testing.T) {
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			handler := http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusOK)
				writer.(http.Flusher).Flush()
				select {
				case <-release:
				case <-req.Context().Done():
					return
				}
				io.WriteString(writer, "streamed body")
			})
			origin := httptest.NewUnstartedServer(handler)
			origin.EnableHTTP2 = true
			origin.StartTLS()
			test.Cleanup(origin.Close)
			tunnelDone := make(chan struct{})
			proxy := testProxyServer(test, func(writer http.ResponseWriter, req *http.Request) {
				if scheme == "http" {
					handler.ServeHTTP(writer, req)
					return
				}
				upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
				if err != nil {
					test.Error(err)
					return
				}
				defer upstream.Close()
				conn, buffered, err := writer.(http.Hijacker).Hijack()
				if err != nil {
					test.Error(err)
					return
				}
				defer conn.Close()
				defer close(tunnelDone)
				buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
				if err := buffered.Flush(); err != nil {
					test.Error(err)
					return
				}
				copied := make(chan struct{})
				go func() { io.Copy(upstream, buffered); upstream.Close(); close(copied) }()
				io.Copy(conn, upstream)
				conn.Close()
				<-copied
			})
			const headerTimeout = 30 * time.Millisecond
			client := testProxyClient(test, origin, proxy, nil, headerTimeout)
			client.Transport.(*proxyFallbackTransport).direct = proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
				test.Error("successful streaming proxy must not fall back")
				return nil, errors.New("unexpected streaming fallback")
			})
			var gotConn atomic.Int32
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { gotConn.Add(1) }})
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://github.com/stream", nil)
			resp, err := client.Do(req)
			if err != nil {
				test.Fatal(err)
			}
			if scheme == "https" && resp.ProtoMajor != 2 {
				test.Errorf("HTTPS protocol=%s, want HTTP/2", resp.Proto)
			}
			<-time.After(3 * headerTimeout)
			releaseOnce.Do(func() { close(release) })
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(body) != "streamed body" || gotConn.Load() != 1 {
				test.Fatalf("stream body=%q err=%v trace=%d", body, err, gotConn.Load())
			}
			client.CloseIdleConnections()
			if scheme == "https" {
				waitProxySignal(test, tunnelDone)
			}
		})
	}
}

func TestBuildProxyDomainsAppendsCustomDomains(test *testing.T) {
	domains := buildProxyDomains("custom.example.com")

	if !shouldProxyHost("custom.example.com", domains) {
		test.Fatalf("自定义站点应追加到代理规则，domains=%v", domains)
	}
	if !shouldProxyHost("api.themoviedb.org", domains) {
		test.Fatalf("自定义站点不应覆盖内置代理规则，domains=%v", domains)
	}
}

func TestShouldUseProxyRequiresProxyURL(test *testing.T) {
	domains := buildProxyDomains("")

	if shouldUseProxy(nil, "api.telegram.org", domains) {
		test.Fatal("未配置代理服务器地址时应保持直连")
	}

	proxyURL, err := url.Parse("http://127.0.0.1:7890")
	if err != nil {
		test.Fatalf("解析测试代理地址失败: %v", err)
	}
	if !shouldUseProxy(proxyURL, "api.telegram.org", domains) {
		test.Fatal("代理地址有效且命中内置域名时应走代理")
	}
}

func TestBuildNetworkProbeRequestURLAddsTMDBAPIKey(test *testing.T) {
	const apiKey = "tmdb-secret-key"
	requestURL := buildNetworkProbeRequestURL("https://api.themoviedb.org/3/configuration?language=zh-CN", apiKey)

	parsed, err := url.Parse(requestURL)
	if err != nil {
		test.Fatalf("解析探测请求地址失败: %v", err)
	}
	if parsed.Query().Get("api_key") != apiKey {
		test.Fatalf("TMDB 探测请求应携带 API Key，url=%s", redactNetworkProbeSecret(requestURL, apiKey))
	}
	if parsed.Query().Get("language") != "zh-CN" {
		test.Fatalf("原有查询参数应保留，url=%s", redactNetworkProbeSecret(requestURL, apiKey))
	}
}

func TestBuildNetworkProbeRequestURLDoesNotModifyOtherSites(test *testing.T) {
	const rawURL = "https://api.github.com?api_key=public-value"
	if got := buildNetworkProbeRequestURL(rawURL, "tmdb-secret-key"); got != rawURL {
		test.Fatalf("非 TMDB 站点不应被修改，got=%s", got)
	}
}

func TestRedactNetworkProbeSecret(test *testing.T) {
	const apiKey = "tmdb-secret-key"
	message := redactNetworkProbeSecret("request failed: https://api.themoviedb.org/3/configuration?api_key="+apiKey, apiKey)
	if strings.Contains(message, apiKey) {
		test.Fatalf("错误信息不应泄漏 TMDB API Key: %s", message)
	}
	if !strings.Contains(message, "******") {
		test.Fatalf("错误信息应保留脱敏占位符: %s", message)
	}
}
