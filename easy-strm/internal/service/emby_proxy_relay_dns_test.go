package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProxyRelayRedirectToSplitDNSDomain(t *testing.T) {
	for _, test := range []struct {
		name       string
		blocked    bool
		wantStatus int
	}{
		{"允许内网分域", false, 200},
		{"下一跳链路本地拒绝", true, 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls, lookups := 0, 0
			service := newEmbyProxyRelayTestService(t, func(request *http.Request) (*http.Response, error) {
				calls++
				assertEmbyProxyRelayRequest(t, request, "GET")
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("split-dns-video")), Request: request}
				if calls == 1 {
					response.StatusCode = 302
					response.Header.Set("Location", "http://cdn.example/video?signature=split-signature")
					return response, nil
				}
				if calls != 2 || request.URL.Hostname() != "cdn.example" || request.URL.RawQuery != "signature=split-signature" {
					t.Fatalf("未跟随原域名直链: calls=%d URL=%s", calls, request.URL)
				}
				target, ok := request.Context().Value(embyProxyRelayTargetKey{}).(embyProxyRelayTarget)
				if !ok || target.address != "cdn.example:80" || len(target.ips) != 1 || target.ips[0] != netip.MustParseAddr("192.168.31.12") {
					t.Fatalf("请求没有绑定已校验的分域 IP: %+v", target)
				}
				connection, err := target.dial(request.Context(), "tcp", func(_ context.Context, _, address string) (net.Conn, error) {
					if address != "192.168.31.12:80" || lookups != 1 {
						t.Errorf("拨号未固定内网 IP 或二次解析: %s lookups=%d", address, lookups)
					}
					client, peer := net.Pipe()
					_ = peer.Close()
					return client, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				_ = connection.Close()
				if test.blocked {
					response.StatusCode = 302
					response.Header.Set("Location", "/next")
				}
				return response, nil
			})
			service.relayLookup = func(_ context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if network != "ip" || host != "cdn.example" {
					t.Fatalf("错误 DNS 查询: %s %s", network, host)
				}
				if lookups > 1 {
					return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
				}
				return []netip.Addr{netip.MustParseAddr("192.168.31.12")}, nil
			}
			response, failure := service.fetchRelayMedia(context.Background(), embyProxyRelayTestRequest("GET", embyProxyTestOrigin), embyProxyTestMediaURL)
			if test.blocked {
				if response != nil || failure == nil || failure.status != test.wantStatus || failure.reason != "unsafe_location" || lookups != 2 {
					t.Fatalf("下一跳未重新校验: response=%v failure=%+v lookups=%d", response, failure, lookups)
				}
			} else {
				if failure != nil || response == nil {
					t.Fatalf("分域中继失败: %+v", failure)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil || string(body) != "split-dns-video" || response.StatusCode != test.wantStatus || lookups != 1 {
					t.Fatalf("分域中继响应错误: status=%d body=%q err=%v lookups=%d", response.StatusCode, body, err, lookups)
				}
			}
			if calls != 2 {
				t.Errorf("媒体请求次数=%d，期望 2", calls)
			}
		})
	}
}

func TestProxyRelayTLSOriginalDomainAndPinnedDial(t *testing.T) {
	for _, validHost := range []bool{true, false} {
		name := "验证原域名"
		if !validHost {
			name = "拒绝证书域名不匹配"
		}
		t.Run(name, func(t *testing.T) {
			var mediaCalls atomic.Int32
			serverNames := make(chan string, 1)
			media := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				mediaCalls.Add(1)
				assertEmbyProxyRelayRequest(t, request, "GET")
				_, _ = io.WriteString(w, "tls-video")
			}))
			media.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
				serverNames <- hello.ServerName
				return nil, nil
			}}
			media.StartTLS()
			defer media.Close()
			if len(media.Certificate().DNSNames) == 0 {
				t.Fatal("TLS 测试证书缺少域名")
			}
			host := media.Certificate().DNSNames[0]
			if !validHost {
				host = "mismatch.example"
			}
			_, port, err := net.SplitHostPort(media.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			service := NewEmbyProxyService(nil, nil)
			defer service.Close()
			lookups, dials := 0, 0
			service.relayLookup = func(_ context.Context, network, domain string) ([]netip.Addr, error) {
				lookups++
				if network != "ip" || domain != host {
					t.Errorf("错误解析参数: %s %s", network, domain)
				}
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			transport := service.relayClient.Transport.(*http.Transport)
			roots := x509.NewCertPool()
			roots.AddCert(media.Certificate())
			transport.TLSClientConfig = &tls.Config{RootCAs: roots}
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				dials++
				if address != net.JoinHostPort(host, port) {
					t.Errorf("拨号前原域名丢失: %s", address)
				}
				return dialEmbyProxyRelay(ctx, network, address)
			}
			response, failure := service.fetchRelayMedia(context.Background(), embyProxyRelayTestRequest("GET", embyProxyTestOrigin), "https://"+net.JoinHostPort(host, port)+"/video")
			if validHost {
				if failure != nil || response == nil {
					t.Fatalf("TLS 原域名校验失败: %+v", failure)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil || string(body) != "tls-video" || mediaCalls.Load() != 1 {
					t.Fatalf("TLS 中继失败: body=%q err=%v calls=%d", body, err, mediaCalls.Load())
				}
			} else if response != nil || failure == nil || failure.reason != "connection_failed" || !strings.Contains(failure.detail, "certificate") || mediaCalls.Load() != 0 {
				t.Fatalf("证书域名不匹配未拒绝: response=%v failure=%+v calls=%d", response, failure, mediaCalls.Load())
			}
			if lookups != 1 || dials != 1 {
				t.Errorf("请求不应二次解析: lookups=%d dials=%d", lookups, dials)
			}
			select {
			case serverName := <-serverNames:
				if serverName != host {
					t.Errorf("TLS SNI=%q，期望原域名 %q", serverName, host)
				}
			default:
				t.Error("未收到 TLS ClientHello")
			}
		})
	}
}
