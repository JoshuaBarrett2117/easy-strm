package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

type shareLinkClock struct{ nanos atomic.Int64 }

func newShareLinkClock() *shareLinkClock {
	clock := &shareLinkClock{}
	clock.nanos.Store(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).UnixNano())
	return clock
}

func (clock *shareLinkClock) now() time.Time                 { return time.Unix(0, clock.nanos.Load()) }
func (clock *shareLinkClock) advance(duration time.Duration) { clock.nanos.Add(int64(duration)) }

func shareLinkForExpiry(expiry time.Time) string {
	return fmt.Sprintf("https://cdn.example/video?t=%d", expiry.Unix())
}

func shareTestCache(t *testing.T) (*shareDirectLinks, *shareLinkClock) {
	t.Helper()
	cache, clock := newShareDirectLinks(), newShareLinkClock()
	cache.now = clock.now
	t.Cleanup(func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		for key := range cache.entries {
			cache.remove(key)
		}
		if len(cache.flights) != 0 || len(cache.slots) != 0 {
			t.Error("flight or slot leaked")
		}
	})
	return cache, clock
}

func awaitShareLinkCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("synchronization deadline exceeded")
		}
		runtime.Gosched()
	}
}

func awaitShareLinkWaiters(t *testing.T, cache *shareDirectLinks, count int) {
	t.Helper()
	awaitShareLinkCondition(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		total := 0
		for _, flight := range cache.flights {
			total += flight.waiters
		}
		return total == count
	})
}

func TestShareStrmDirectLinkConcurrentPlayback(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint("failed=", failed), func(t *testing.T) {
			service, _, _ := strmFixture(t)
			entered, release := make(chan struct{}, 1), make(chan struct{})
			var calls atomic.Int32
			failure := errors.New("upstream temporarily unavailable")
			service.SetPlaybackDirectLink(func(context.Context, string, int, string, string) (string, error) {
				calls.Add(1)
				entered <- struct{}{}
				<-release
				if failed {
					return "", failure
				}
				return "https://cdn.example/video?t=4102444800", nil
			})
			const requests = 12
			var wait sync.WaitGroup
			for request := 0; request < requests; request++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					link, err := service.Playback(context.Background(), "entry", "Player/1")
					if failed {
						if !errors.Is(err, failure) || link != "" {
							t.Error("failure was not shared")
						}
					} else if err != nil || link != "https://cdn.example/video?t=4102444800" {
						t.Error("successful resolution was not shared")
					}
				}()
			}
			<-entered
			awaitShareLinkWaiters(t, service.directLinks(), requests)
			close(release)
			wait.Wait()
			if calls.Load() != 1 {
				t.Fatalf("applications = %d, want 1", calls.Load())
			}
			if failed {
				if _, err := service.Playback(context.Background(), "entry", "Player/1"); !errors.Is(err, failure) {
					t.Error("next failure must retry")
				}
				if calls.Load() != 2 {
					t.Error("failure was cached")
				}
			}
		})
	}
}

func TestShareStrmDirectLinkTTL(t *testing.T) {
	for _, lifetime := range []time.Duration{61 * time.Second, 90 * time.Second, 120 * time.Second, time.Hour} {
		t.Run(lifetime.String(), func(t *testing.T) {
			cache, clock := shareTestCache(t)
			key := sha256.Sum256([]byte("ttl"))
			calls := 0
			resolve := func(context.Context) (shareDirectLinkResult, error) {
				calls++
				return shareDirectLinkResult{link: shareLinkForExpiry(clock.now().Add(lifetime))}, nil
			}
			if _, err := cache.resolve(context.Background(), key, [32]byte{}, resolve); err != nil {
				t.Fatal(err)
			}
			ttl := lifetime - time.Minute
			if ttl > time.Minute {
				ttl = time.Minute
			}
			clock.advance(ttl - time.Nanosecond)
			if _, err := cache.resolve(context.Background(), key, [32]byte{}, resolve); err != nil || calls != 1 {
				t.Fatal("cache missed before TTL boundary")
			}
			clock.advance(time.Nanosecond)
			if _, err := cache.resolve(context.Background(), key, [32]byte{}, resolve); err != nil || calls != 2 {
				t.Fatal("cache reused at exact TTL boundary")
			}
		})
	}
}

func TestShareStrmDirectLinkRejectsUnsafeExpiry(t *testing.T) {
	clock := newShareLinkClock()
	valid := shareLinkForExpiry(clock.now().Add(time.Hour))
	cases := map[string]string{
		"expired":      shareLinkForExpiry(clock.now().Add(-time.Second)),
		"exact_margin": shareLinkForExpiry(clock.now().Add(time.Minute)),
		"near":         shareLinkForExpiry(clock.now().Add(59 * time.Second)),
		"unknown":      "https://cdn.example/video",
		"malformed":    "https://cdn.example/video?t=not-a-time",
		"overflow":     "https://cdn.example/video?t=99999999999999999999",
		"milliseconds": "https://cdn.example/video?t=1792972800000",
		"duplicate":    valid + "&t=4102444800",
		"broken_query": valid + "&k=%zz",
		"userinfo":     strings.Replace(valid, "cdn.example", "user:fixture@cdn.example", 1),
		"scheme":       strings.Replace(valid, "https:", "ftp:", 1),
		"relative":     "/video?t=4102444800",
		"fragment":     valid + "#fragment",
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			cache, _ := shareTestCache(t)
			cache.now = clock.now
			calls := 0
			resolve := func(context.Context) (shareDirectLinkResult, error) {
				calls++
				return shareDirectLinkResult{link: link}, nil
			}
			for attempt := 0; attempt < 2; attempt++ {
				result, err := cache.resolve(context.Background(), [32]byte{}, [32]byte{}, resolve)
				if !errors.Is(err, errShareDirectLinkURL) || result.link != "" {
					t.Fatal("unsafe upstream URL accepted")
				}
			}
			if calls != 2 || len(cache.entries) != 0 {
				t.Fatal("invalid URL cached")
			}
			cache.mu.Lock()
			cache.save([32]byte{}, shareDirectLinkResult{link: link}, clock.now().Add(time.Minute))
			cache.mu.Unlock()
			if result, err := cache.resolve(context.Background(), [32]byte{}, [32]byte{}, resolve); err == nil || result.link != "" {
				t.Fatal("unsafe cached URL accepted")
			}
		})
	}
}

func TestShareStrmDirectLinkExpiresDuringSharedWait(t *testing.T) {
	cache, clock := shareTestCache(t)
	entered, release := make(chan struct{}), make(chan struct{})
	link := shareLinkForExpiry(clock.now().Add(61 * time.Second))
	resolve := func(context.Context) (shareDirectLinkResult, error) {
		close(entered)
		<-release
		return shareDirectLinkResult{link: link}, nil
	}
	var wait sync.WaitGroup
	for request := 0; request < 5; request++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := cache.resolve(context.Background(), [32]byte{}, [32]byte{}, resolve)
			if !errors.Is(err, errShareDirectLinkURL) || result.link != "" {
				t.Error("URL expired during wait was delivered")
			}
		}()
	}
	<-entered
	awaitShareLinkWaiters(t, cache, 5)
	clock.advance(time.Second)
	close(release)
	wait.Wait()
}

func TestShareStrmDirectLinkUASignatureIsolation(t *testing.T) {
	service, _, _ := strmFixture(t)
	var received []string
	service.SetPlaybackDirectLink(func(_ context.Context, _ string, _ int, _ string, ua string) (string, error) {
		received = append(received, ua)
		return "https://cdn.example/video?t=4102444800", nil
	})
	for _, ua := range []string{"Lavf/1", "Lavf/2", "Mozilla/1", "Mozilla/2", "Other/1", "Other/2", "", " \t"} {
		for repeat := 0; repeat < 2; repeat++ {
			if _, err := service.Playback(context.Background(), "entry", ua); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(received) != 7 || received[6] != "" {
		t.Fatalf("effective UA partitions = %d, want 7", len(received))
	}
}

func TestShareStrmDirectLinkLifecycle(t *testing.T) {
	for _, change := range []string{"mapping", "file_id", "config", "account", "cookie", "delete"} {
		t.Run(change, func(t *testing.T) {
			service, store, _ := strmFixture(t)
			entry := store.entries["entry"]
			entry.FilePath = "Show/Show.S02E03.mkv"
			store.entries["entry"] = entry
			account := &domain.Cloud115{ID: 7, Cookie: "fixture"}
			service.account = func(int) (*domain.Cloud115, error) { return account, nil }
			calls := 0
			service.SetPlaybackDirectLink(func(context.Context, string, int, string, string) (string, error) {
				calls++
				return "https://cdn.example/video?t=4102444800", nil
			})
			if _, err := service.Playback(context.Background(), "entry", "Player/1"); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "mapping":
				entry.Password = "changed"
				store.entries["entry"] = entry
			case "file_id":
				entry.FileID = "changed"
				store.entries["entry"] = entry
			case "config", "account":
				cfg, _ := service.Settings()
				if change == "config" {
					cfg.TransferPath = "/changed"
				} else {
					cfg.Cloud115ID = 8
					account.ID = 8
				}
				raw, _ := json.Marshal(cfg)
				service.settings.(*shareSettingsMemory).values[shareStrmSettingsKey].ConfigVal = string(raw)
			case "cookie":
				account.Cookie = "changed"
			case "delete":
				delete(store.entries, "entry")
			}
			_, err := service.Playback(context.Background(), "entry", "Player/1")
			if change == "delete" {
				if !errors.Is(err, sql.ErrNoRows) || calls != 1 {
					t.Fatal("deleted mapping used cached URL")
				}
			} else if err != nil || calls != 2 {
				t.Fatalf("changed identity reused URL: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestShareStrmDirectLinkCancellationIndependent(t *testing.T) {
	cache, clock := shareTestCache(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var work context.Context
	resolve := func(ctx context.Context) (shareDirectLinkResult, error) {
		work = ctx
		close(entered)
		<-release
		return shareDirectLinkResult{link: shareLinkForExpiry(clock.now().Add(time.Hour))}, nil
	}
	caller, cancel := context.WithCancel(context.Background())
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, err := cache.resolve(caller, [32]byte{}, [32]byte{}, resolve); first <- err }()
	<-entered
	go func() { _, err := cache.resolve(context.Background(), [32]byte{}, [32]byte{}, resolve); second <- err }()
	awaitShareLinkWaiters(t, cache, 2)
	cancel()
	if !errors.Is(<-first, context.Canceled) || work.Err() != nil {
		t.Fatal("abandoned waiter canceled active peer")
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
}

func TestShareStrmDirectLinkAllWaitersGoneRetainsFlightUntilExit(t *testing.T) {
	cache, _ := shareTestCache(t)
	entered, release, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	caller, cancel := context.WithCancel(context.Background())
	resolve := func(ctx context.Context) (shareDirectLinkResult, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return shareDirectLinkResult{}, ctx.Err()
	}
	finished := make(chan error, 1)
	go func() { _, err := cache.resolve(caller, [32]byte{}, [32]byte{}, resolve); finished <- err }()
	<-entered
	cancel()
	if !errors.Is(<-finished, context.Canceled) {
		t.Fatal("caller not canceled")
	}
	<-canceled
	if _, err := cache.resolve(context.Background(), [32]byte{}, [32]byte{}, resolve); !errors.Is(err, errShareDirectLinkBusy) {
		t.Fatal("abandoned upstream was replaced before exit")
	}
	close(release)
	awaitShareLinkCondition(t, func() bool { cache.mu.Lock(); defer cache.mu.Unlock(); return len(cache.flights) == 0 })
}

func TestShareStrmDirectLinkLimiterQueueCancellationAndSlotLifetime(t *testing.T) {
	cache, clock := shareTestCache(t)
	entered, release := make(chan struct{}, 3), make(chan struct{})
	var active, maximum atomic.Int32
	apply := func(context.Context) (string, error) {
		current := active.Add(1)
		for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return shareLinkForExpiry(clock.now().Add(time.Hour)), nil
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	finished := make(chan error, 3)
	go func() { _, err := cache.apply(firstCtx, apply); finished <- err }()
	go func() { _, err := cache.apply(context.Background(), apply); finished <- err }()
	<-entered
	<-entered
	queueCtx, cancelQueue := context.WithCancel(context.Background())
	queueDone := make(chan error, 1)
	go func() { _, err := cache.apply(queueCtx, apply); queueDone <- err }()
	cancelQueue()
	if !errors.Is(<-queueDone, context.Canceled) {
		t.Fatal("queue cancellation lost")
	}
	cancelFirst()
	if len(cache.slots) != 2 || active.Load() != 2 {
		t.Fatal("slot released before SDK exit")
	}
	go func() { _, err := cache.apply(context.Background(), apply); finished <- err }()
	if len(entered) != 0 {
		t.Fatal("third application bypassed limit")
	}
	close(release)
	for request := 0; request < 3; request++ {
		<-finished
	}
	if maximum.Load() != 2 {
		t.Fatal("actual concurrency limit violated")
	}
	if _, err := cache.apply(context.Background(), func(context.Context) (string, error) { return "", errors.New("retry") }); err == nil {
		t.Fatal("error swallowed")
	}
}

func TestShareStrmDirectLinkBoundsAndPanicCleanup(t *testing.T) {
	cache, clock := shareTestCache(t)
	for value := 0; value < shareDirectLinkCapacity+20; value++ {
		key := sha256.Sum256([]byte(fmt.Sprint(value)))
		if _, err := cache.resolve(context.Background(), key, [32]byte{}, func(context.Context) (shareDirectLinkResult, error) {
			return shareDirectLinkResult{link: shareLinkForExpiry(clock.now().Add(time.Hour))}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(cache.entries) != shareDirectLinkCapacity {
		t.Fatal("cache not bounded")
	}
	clock.advance(time.Minute)
	key := sha256.Sum256([]byte("panic"))
	if _, err := cache.resolve(context.Background(), key, [32]byte{}, func(context.Context) (shareDirectLinkResult, error) { panic("synthetic-private-body") }); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("panic cleanup failed")
	}
	if len(cache.entries) != 0 || len(cache.flights) != 0 {
		t.Fatal("stale entries or panic flight retained")
	}
	resolve := func(context.Context) (shareDirectLinkResult, error) {
		return shareDirectLinkResult{link: shareLinkForExpiry(clock.now().Add(time.Hour))}, nil
	}
	if _, err := cache.resolve(context.Background(), key, [32]byte{}, resolve); err != nil {
		t.Fatal("panic poisoned retry")
	}
	cache.mu.Lock()
	for value := 0; value < shareDirectLinkFlights; value++ {
		cache.flights[sha256.Sum256([]byte(fmt.Sprint("flight", value)))] = &shareDirectLinkFlight{}
	}
	cache.mu.Unlock()
	if _, err := cache.resolve(context.Background(), sha256.Sum256([]byte("overflow")), [32]byte{}, resolve); !errors.Is(err, errShareDirectLinkBusy) {
		t.Fatal("flight count not bounded")
	}
	cache.mu.Lock()
	clear(cache.flights)
	cache.mu.Unlock()
	cache.mu.Lock()
	cache.flights[key] = &shareDirectLinkFlight{waiters: shareDirectLinkWaiters}
	cache.remove(key)
	cache.mu.Unlock()
	if _, err := cache.resolve(context.Background(), key, [32]byte{}, resolve); !errors.Is(err, errShareDirectLinkBusy) {
		t.Fatal("waiter count not bounded")
	}
	cache.mu.Lock()
	delete(cache.flights, key)
	cache.mu.Unlock()
}

func TestShareStrmDirectLinkFailureFieldsAndSanitization(t *testing.T) {
	output := captureEmbyProxyLog(t)
	for _, test := range []struct {
		err               error
		errno             string
		status            any
		risk              string
		timeout, canceled bool
	}{
		{&domain.DirectLinkError{Errno: 40101010, HTTPStatus: 403, Cause: driver.ErrLoginTwoStepVerify}, "40101010", 403, "verification_required", false, false},
		{&domain.DirectLinkError{Errno: 911, HTTPStatus: 429, Cause: errors.New("body=synthetic-private-body")}, "911", 429, "unknown", false, false},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "unknown", "unknown", "unknown", true, false},
		{fmt.Errorf("wrapped: %w", context.Canceled), "unknown", "unknown", "unknown", false, true},
		{&domain.DirectLinkError{Cause: errors.New("opaque")}, "unknown", "unknown", "unknown", false, false},
	} {
		fields := shareDirectLinkFailureFields(test.err)
		if fields["upstream_errno"] != test.errno || fields["upstream_http_status"] != test.status || fields["risk_control"] != test.risk || fields["timeout"] != test.timeout || fields["canceled"] != test.canceled {
			t.Fatal("incorrect structured failure fields")
		}
		logShareDirectLinkFailure(context.Background(), test.err)
	}
	logShareDirectLinkFailure(context.Background(), errors.New("cookie=fixture-secret https://cdn.example/video?signature=fixture-signature body=fixture-body"))
	text := output.String()
	for _, secret := range []string{"fixture-secret", "fixture-signature", "fixture-body", "synthetic-private-body", "https://"} {
		if strings.Contains(text, secret) {
			t.Fatal("sensitive diagnostic leaked")
		}
	}
	for _, marker := range []string{"share_strm_directlink_singleflight_v1", "directlink", "share_strm_playback", "upstream_errno", "upstream_http_status", "risk_control", "timeout", "canceled", "error_type"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("missing log field %s", marker)
		}
	}
}

type shareDirectLinkExpiryContext struct {
	context.Context
	done chan struct{}
}

func (ctx shareDirectLinkExpiryContext) Done() <-chan struct{} { return ctx.done }

func (ctx shareDirectLinkExpiryContext) Err() error {
	select {
	case <-ctx.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestShareStrmDirectLinkOperationErrorPrecedence(t *testing.T) {
	for _, operation := range []string{"apply", "run"} {
		for _, expiration := range []string{"deadline", "cancellation"} {
			for _, outcome := range []string{"business_error", "wrapped_business_error", "nil_error"} {
				t.Run(operation+"/"+expiration+"/"+outcome, func(t *testing.T) {
					cache, clock := shareTestCache(t)
					ctx, expire := context.WithCancel(context.Background())
					defer expire()
					contextErr := context.Canceled
					if expiration == "deadline" {
						expiry := shareDirectLinkExpiryContext{Context: context.Background(), done: make(chan struct{})}
						ctx, expire = expiry, func() { close(expiry.done) }
						contextErr = context.DeadlineExceeded
					}
					businessErr := &domain.DirectLinkError{Errno: 40101010, HTTPStatus: 403, Cause: driver.ErrLoginTwoStepVerify}
					var operationErr error = businessErr
					if outcome == "wrapped_business_error" {
						operationErr = fmt.Errorf("operation: %w", businessErr)
					} else if outcome == "nil_error" {
						operationErr = nil
					}
					callback := func(call context.Context) (string, error) {
						expire()
						<-call.Done()
						if call.Err() != contextErr {
							t.Fatal("callback context did not expire as expected")
						}
						return shareLinkForExpiry(clock.now().Add(time.Hour)), operationErr
					}
					var link string
					var err error
					if operation == "apply" {
						link, err = cache.apply(ctx, callback)
					} else {
						key := [32]byte{}
						flight := &shareDirectLinkFlight{done: make(chan struct{}), cancel: func() {}}
						cache.flights[key] = flight
						cache.run(ctx, key, flight, func(call context.Context) (shareDirectLinkResult, error) {
							value, callbackErr := callback(call)
							return shareDirectLinkResult{link: value}, callbackErr
						})
						select {
						case <-flight.done:
						default:
							t.Fatal("flight completion not published")
						}
						link, err = flight.result.link, flight.err
						if len(cache.entries) != 0 {
							t.Error("expired operation result cached")
						}
					}
					if outcome == "nil_error" {
						if err != contextErr || link != "" {
							t.Fatal("nil operation error did not fall back to context error")
						}
						return
					}
					var upstream *domain.DirectLinkError
					if !errors.As(err, &upstream) || upstream != businessErr {
						t.Error("business error pointer not preserved")
					}
					if err != operationErr || !errors.Is(err, driver.ErrLoginTwoStepVerify) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
						t.Error("original operation error identity or chain changed")
					}
					fields := shareDirectLinkFailureFields(err)
					if fields["upstream_errno"] != "40101010" || fields["risk_control"] != "verification_required" || fields["error_type"] != "upstream_error" || fields["timeout"] != false || fields["canceled"] != false {
						t.Error("business error misclassified as context failure")
					}
				})
			}
		}
	}
}

func TestShareStrmDirectLinkWarmPlaybackResolvesOnce(t *testing.T) {
	service, _, _ := strmFixture(t)
	calls := 0
	service.directLink = func(string, int, string, string) (string, error) {
		calls++
		return "https://cdn.example/video?t=4102444800", nil
	}
	for request := 0; request < 2; request++ {
		if _, err := service.Playback(context.Background(), "entry", "Player/1"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("warm Playback applications = %d, want 1", calls)
	}
}

type shareLinkDirectoryCounter struct {
	Cloud115Client
	mkdir, list atomic.Int32
}

func (client *shareLinkDirectoryCounter) MkdirAll115(path string, accountID int, cookie string) (string, error) {
	client.mkdir.Add(1)
	return client.Cloud115Client.MkdirAll115(path, accountID, cookie)
}

func (client *shareLinkDirectoryCounter) GetFileList(cid, showDir, offset, limit, accountID int, cookie string) (*driver.FileListResp, error) {
	client.list.Add(1)
	return client.Cloud115Client.GetFileList(cid, showDir, offset, limit, accountID, cookie)
}

func TestShareStrmDirectLinkCanonicalUUIDAndWarmDirectoryBypass(t *testing.T) {
	service, store, client := strmFixture(t)
	const id = "56c94081-7619-4824-9e6f-ea13141599e1"
	entry := store.entries["entry"]
	entry.ID = id
	store.entries[id] = entry
	client.present = true
	counter := &shareLinkDirectoryCounter{Cloud115Client: client}
	service.client = counter
	var applications atomic.Int32
	service.SetPlaybackDirectLink(func(context.Context, string, int, string, string) (string, error) {
		applications.Add(1)
		return "https://cdn.example/video?t=4102444800", nil
	})
	for _, identifier := range []string{id, strings.ToUpper(id), "urn:uuid:" + id, "{" + id + "}"} {
		if _, err := service.Playback(context.Background(), identifier, "Player/1"); err != nil {
			t.Fatal(err)
		}
	}
	if applications.Load() != 1 || counter.mkdir.Load() != 1 || counter.list.Load() != 1 {
		t.Fatal("canonical identity or warm directory bypass failed")
	}
}
