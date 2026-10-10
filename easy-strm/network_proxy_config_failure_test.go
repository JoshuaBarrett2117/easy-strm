package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkglogger "easy-strm/internal/pkg/logger"
)

func TestProxyConfigReadFailureSendsDirectHTTP(test *testing.T) {
	var output bytes.Buffer
	pkglogger.SetOutputs(&output, &output, &output, &output)
	pkglogger.SetLevel(pkglogger.DEBUG)
	pkglogger.SetFormat("json")
	test.Cleanup(func() {
		pkglogger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr)
		pkglogger.SetLevel(pkglogger.INFO)
		pkglogger.SetFormat("")
	})

	for _, partial := range []bool{false, true} {
		name := "empty config"
		if partial {
			name = "partial config"
		}
		test.Run(name, func(test *testing.T) {
			output.Reset()
			var serverCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				serverCalls.Add(1)
				body, err := io.ReadAll(request.Body)
				if err != nil {
					test.Error(err)
				}
				if request.Method != http.MethodPost || string(body) != "payload" {
					test.Errorf("direct request changed: method=%s body=%q", request.Method, body)
				}
				_, _ = writer.Write([]byte("direct response"))
			}))
			defer server.Close()

			ctx := pkglogger.WithTaskID(pkglogger.WithRequestID(context.Background(), "proxy-config-request"), "proxy-config-task")
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			var gotConnection atomic.Bool
			ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
				GotConn: func(httptrace.GotConnInfo) { gotConnection.Store(true) },
			})
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader("payload"))
			if err != nil {
				test.Fatal(err)
			}
			client := newProxyAwareHTTPClient(0, nil, http.DefaultTransport.(*http.Transport))
			defer client.CloseIdleConnections()
			transport := client.Transport.(*proxyFallbackTransport)
			var loaderCalls, directCalls int
			failure := errors.New("synthetic config read failure")
			transport.loadProxyConfigContext = func(loadedContext context.Context) (*url.URL, []string, error) {
				loaderCalls++
				if loadedContext != ctx {
					test.Error("config loader did not receive original request context")
				}
				if partial {
					return &url.URL{Scheme: "http", Host: "unused.invalid:7890"}, []string{"127.0.0.1"}, failure
				}
				return nil, nil, failure
			}
			transport.proxy = proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
				test.Error("failed config must not route through a partially loaded proxy")
				return nil, errors.New("unexpected proxy request")
			})
			direct := transport.direct
			transport.direct = proxyRoundTripFunc(func(sentRequest *http.Request) (*http.Response, error) {
				directCalls++
				if sentRequest != request || sentRequest.Context() != ctx {
					test.Error("direct transport did not receive original request and context")
				}
				return direct.RoundTrip(sentRequest)
			})

			response, err := client.Do(request)
			if err != nil {
				test.Fatalf("config read failure must fail open and send direct HTTP: %v", err)
			}
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil || response.StatusCode != http.StatusOK || string(body) != "direct response" {
				test.Fatalf("direct response=%q status=%d err=%v", body, response.StatusCode, readErr)
			}
			if loaderCalls != 1 || directCalls != 1 || serverCalls.Load() != 1 || !gotConnection.Load() {
				test.Fatalf("loader/direct/server calls=%d/%d/%d trace=%v", loaderCalls, directCalls, serverCalls.Load(), gotConnection.Load())
			}
			var record struct {
				Level      string   `json:"level"`
				RequestID  string   `json:"request_id"`
				TaskID     string   `json:"task_id"`
				Module     string   `json:"module"`
				ErrorChain []string `json:"error_chain"`
			}
			if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
				test.Fatalf("expected one contextual WARN record: %v", err)
			}
			if record.Level != "WARN" || record.Module != "network_proxy" || record.RequestID != "proxy-config-request" || record.TaskID != "proxy-config-task" || len(record.ErrorChain) != 1 || record.ErrorChain[0] != failure.Error() {
				test.Fatalf("missing contextual config failure warning: %+v", record)
			}
		})
	}
}

func TestProxyConfigReadFailurePreservesCancellationDuringLoad(test *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &proxyFallbackTransport{
		loadProxyConfigContext: func(loadedContext context.Context) (*url.URL, []string, error) {
			if loadedContext != ctx {
				test.Error("config loader lost request context")
			}
			cancel()
			return nil, nil, errors.New("synthetic config read failure")
		},
		direct: proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
			test.Error("canceled request must not reach direct transport")
			return nil, errors.New("unexpected direct request")
		}),
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unused.invalid", nil)
	if err != nil {
		test.Fatal(err)
	}
	if response, err := transport.RoundTrip(request); response != nil || !errors.Is(err, context.Canceled) {
		test.Fatalf("request cancellation must override config read error: response=%v err=%v", response, err)
	}
}

func TestProxyConfigReadFailureDirectHTTPCancellation(test *testing.T) {
	started := make(chan struct{})
	serverCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(serverCanceled)
	}))
	defer server.Close()
	client := newProxyAwareHTTPClient(0, nil, http.DefaultTransport.(*http.Transport))
	defer client.CloseIdleConnections()
	client.Transport.(*proxyFallbackTransport).loadProxyConfigContext = func(context.Context) (*url.URL, []string, error) {
		return nil, nil, errors.New("synthetic config read failure")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		test.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		response, err := client.Do(request)
		if response != nil {
			response.Body.Close()
		}
		result <- err
	}()
	select {
	case <-started:
	case err := <-result:
		test.Fatalf("direct HTTP must start before cancellation: %v", err)
	case <-ctx.Done():
		test.Fatal("direct HTTP did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			test.Fatalf("direct HTTP lost cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		test.Fatal("direct HTTP did not stop after cancellation")
	}
	select {
	case <-serverCanceled:
	case <-time.After(5 * time.Second):
		test.Fatal("server did not observe canceled direct request")
	}
}
