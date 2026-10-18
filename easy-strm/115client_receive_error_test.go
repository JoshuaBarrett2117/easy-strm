package main

import (
	"context"
	"crypto/sha256"
	"easy-strm/internal/domain"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

func TestReceiveSharePreservesUpstreamMessage(t *testing.T) {
	for _, field := range []string{"errmsg", "error", "message"} {
		t.Run(field, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/share/receive" || r.Method != "POST" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"state":false,"errno":4100018,"` + field + `":"上游原始消息"}`)), Request: r}, nil
			})}
			d := driver.New(driver.WithClient(client)).ImportCredential(&driver.Credential{})
			driverCache.Lock()
			driverCache.drivers[99116] = cached115Driver{driver: d, cookieHash: sha256.Sum256(nil)}
			driverCache.Unlock()
			defer func() { driverCache.Lock(); delete(driverCache.drivers, 99116); driverCache.Unlock() }()
			err := (&Client{}).ReceiveShare("test", "", "file", "dir", 99116, "")
			var received *domain.ShareReceiveError
			if !errors.As(err, &received) || received.Errno != 4100018 || received.Message != "上游原始消息" {
				t.Fatalf("lost upstream detail: %v", err)
			}
		})
	}
}

type receiveResponseBody struct {
	io.Reader
	readBytes int
	closed    bool
	closeErr  error
}

func (b *receiveResponseBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.readBytes += n
	return n, err
}

func (b *receiveResponseBody) Close() error {
	b.closed = true
	return b.closeErr
}

type receiveFailureReader struct{ err error }

func (r receiveFailureReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestReceiveShareResponseBoundsAndFailures(t *testing.T) {
	const limit = 1 << 20
	for _, tc := range []struct {
		name   string
		status int
		body   *receiveResponseBody
		want   string
	}{
		{"at limit", 200, &receiveResponseBody{Reader: strings.NewReader(`{"state":true}` + strings.Repeat(" ", limit-len(`{"state":true}`)))}, ""},
		{"over limit", 200, &receiveResponseBody{Reader: strings.NewReader(`{"state":true}` + strings.Repeat(" ", limit))}, "response exceeds"},
		{"nil body", 200, nil, "response"},
		{"redirect", 302, &receiveResponseBody{Reader: strings.NewReader(`{"state":true,"token":"body-secret"}`)}, "HTTP 302"},
		{"unauthorized", 401, &receiveResponseBody{Reader: strings.NewReader(`{"state":true,"token":"body-secret"}`)}, "HTTP 401"},
		{"rate limited", 429, &receiveResponseBody{Reader: strings.NewReader("token=body-secret")}, "HTTP 429"},
		{"unavailable", 503, &receiveResponseBody{Reader: strings.NewReader("token=body-secret")}, "HTTP 503"},
		{"invalid json", 200, &receiveResponseBody{Reader: strings.NewReader("token=body-secret")}, "parse share receive response"},
		{"read failure", 200, &receiveResponseBody{Reader: receiveFailureReader{err: errors.New("token=read-secret")}}, "read share receive response"},
		{"read timeout", 200, &receiveResponseBody{Reader: receiveFailureReader{err: context.DeadlineExceeded}}, "read share receive response timeout"},
		{"close failure", 200, &receiveResponseBody{Reader: strings.NewReader(`{"state":true}`), closeErr: errors.New("token=close-secret")}, "close share receive response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body io.ReadCloser
				if tc.body != nil {
					body = tc.body
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: body, Request: r}, nil
			})}
			d := driver.New(driver.WithClient(client)).ImportCredential(&driver.Credential{})
			driverCache.Lock()
			driverCache.drivers[99117] = cached115Driver{driver: d, cookieHash: sha256.Sum256(nil)}
			driverCache.Unlock()
			t.Cleanup(func() { driverCache.Lock(); delete(driverCache.drivers, 99117); driverCache.Unlock() })
			err := (&Client{}).ReceiveShare("test", "", "file", "dir", 99117, "")
			if tc.want == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
				t.Errorf("want safe %q error, got %v", tc.want, err)
			}
			if tc.body != nil && (!tc.body.closed || tc.body.readBytes > limit+1) {
				t.Errorf("response body closed=%v bytes=%d", tc.body.closed, tc.body.readBytes)
			}
			if tc.status >= 300 && tc.body != nil && tc.body.readBytes != 0 {
				t.Error("non-success response body must not be read")
			}
		})
	}
}
