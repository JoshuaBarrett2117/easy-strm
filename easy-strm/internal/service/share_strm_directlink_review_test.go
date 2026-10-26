package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

type shareDirectLinkBlockedLog struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (output *shareDirectLinkBlockedLog) Write(data []byte) (int, error) {
	if output.calls.Add(1) == 1 {
		close(output.entered)
	}
	<-output.release
	return len(data), nil
}

func TestShareStrmDirectLinkPublishesBeforeBlockedLogging(t *testing.T) {
	cache, _ := shareTestCache(t)
	output := &shareDirectLinkBlockedLog{entered: make(chan struct{}), release: make(chan struct{})}
	logger.SetOutputs(output, output, output, output)
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(output.release) })
		logger.SetOutputs(os.Stderr, os.Stderr, os.Stderr, os.Stderr)
	})
	failure := errors.New("isolated resolution failure")
	resolve := func(context.Context) (shareDirectLinkResult, error) { return shareDirectLinkResult{}, failure }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := cache.resolve(ctx, [32]byte{1}, [32]byte{}, resolve)
		finished <- err
	}()
	select {
	case <-output.entered:
	case <-ctx.Done():
		t.Fatal("failure log did not reach sink")
	}
	if !errors.Is(<-finished, failure) {
		t.Fatal("blocked sink delayed flight completion")
	}
	for index := 2; index < 34; index++ {
		if _, err := cache.resolve(ctx, [32]byte{byte(index)}, [32]byte{}, resolve); !errors.Is(err, failure) {
			t.Fatal("blocked log held cache mutex or another resolution")
		}
	}
	cache.mu.Lock()
	remaining := len(cache.flights)
	cache.mu.Unlock()
	if remaining != 0 || len(cache.logSlot) != 1 || output.calls.Load() != 1 {
		t.Fatal("failed resolutions or blocked logging accumulated")
	}
	release.Do(func() { close(output.release) })
}

func TestSharePlaybackWrappedPreparationErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		cause    error
		errno    int
		category string
		retry    bool
		kind     string
	}{
		{"password", fmt.Errorf("SDK wrapper: %w", driver.ErrPasswordIncorrect), 40101009, "分享密码问题", false, "password_incorrect"},
		{"required", driver.ErrUnexpected, 990010, "分享密码问题", false, "unknown"},
		{"incorrect", driver.ErrUnexpected, 990011, "分享密码问题", false, "unknown"},
		{"legacy_password", errors.New("访问密码错误"), 0, "分享密码问题", false, "unknown"},
		{"reset", &net.OpError{Op: "read", Net: "tcp", Err: fmt.Errorf("socket: %w", syscall.ECONNRESET)}, 0, "原因未确认", true, "connection_reset"},
		{"legacy_reset", errors.New("connection reset by peer"), 0, "原因未确认", true, "unknown"},
		{"eof", io.ErrUnexpectedEOF, 0, "原因未确认", true, "unexpected_eof"},
		{"timeout", context.DeadlineExceeded, 0, "上游请求超时", true, "timeout"},
		{"canceled", context.Canceled, 0, "原因未确认", false, "canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := &domain.DirectLinkError{Errno: test.errno, Cause: test.cause}
			err := fmt.Errorf("preparation failed: %w", upstream)
			if isRetryableShareSnapError(err) != test.retry {
				t.Fatal("wrapped preparation retry classification changed")
			}
			if info := DescribeSharePlaybackError(err); info.Category != test.category {
				t.Fatalf("category = %s, want %s", info.Category, test.category)
			}
			mapped := mapShareSnapError(err)
			var actual *domain.DirectLinkError
			if !errors.As(mapped, &actual) || actual != upstream || !errors.Is(mapped, test.cause) {
				t.Fatal("share preparation mapping lost SDK/metadata chain")
			}
			fields := shareDirectLinkFailureFields(err)
			if fields["cause_type"] != test.kind || fields["upstream_http_status"] != "unknown" || fields["risk_control"] != "unknown" {
				t.Fatal("unsafe or invented preparation diagnostics")
			}
		})
	}
}
