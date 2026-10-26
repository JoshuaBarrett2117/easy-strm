package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

type shareDeliveryGateStore struct {
	ShareStrmStore
	reads            atomic.Int32
	trigger          int32
	entered, release chan struct{}
}

func (store *shareDeliveryGateStore) GetStrmEntry(ctx context.Context, id string) (entry domain.ShareStrmEntry, err error) {
	if store.reads.Add(1) == store.trigger {
		close(store.entered)
		<-store.release
	}
	return store.ShareStrmStore.GetStrmEntry(ctx, id)
}

func TestShareStrmDirectLinkDeliveryRevalidates(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, change := range []string{"near_expiry", "expired", "deleted", "mapping_changed", "canceled"} {
			t.Run(fmt.Sprint(cached, "/", change), func(t *testing.T) {
				service, memory, client := strmFixture(t)
				client.present = true
				clock := newShareLinkClock()
				service.directLinks().now = clock.now
				link := shareLinkForExpiry(clock.now().Add(61 * time.Second))
				service.SetPlaybackDirectLink(func(context.Context, string, int, string, string) (string, error) { return link, nil })
				trigger := int32(4)
				if cached {
					trigger = 6
				}
				store := &shareDeliveryGateStore{ShareStrmStore: memory, trigger: trigger, entered: make(chan struct{}), release: make(chan struct{})}
				service.store = store
				if cached {
					if _, err := service.Playback(context.Background(), "entry", "Player/1"); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				finished := make(chan error, 1)
				go func() {
					result, err := service.Playback(ctx, "entry", "Player/1")
					if result != "" {
						t.Error("invalidated URL delivered")
					}
					finished <- err
				}()
				<-store.entered
				memory.entriesMu.Lock()
				switch change {
				case "near_expiry":
					clock.advance(time.Second)
				case "expired":
					clock.advance(62 * time.Second)
				case "deleted":
					delete(memory.entries, "entry")
				case "mapping_changed":
					entry := memory.entries["entry"]
					entry.Password = "changed"
					memory.entries["entry"] = entry
				case "canceled":
					cancel()
				}
				memory.entriesMu.Unlock()
				close(store.release)
				err := <-finished
				if err == nil {
					t.Fatal("delivery did not revalidate")
				}
				if change == "deleted" && !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("deleted mapping status lost")
				}
			})
		}
	}
}

type shareParallelStore struct{ *strmMemory }

func (store *shareParallelStore) LockStrmPlayback(ctx context.Context, _ string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return func() {}, nil
}

type shareLinkRequestID struct{}

func TestShareStrmDirectLinkPlaybackLimitsDistinctKeysAndCancelsQueue(t *testing.T) {
	service, memory, client := strmFixture(t)
	client.present = true
	const requests = 6
	for request := 0; request < requests; request++ {
		entry := memory.entries["entry"]
		entry.ID = fmt.Sprint(request)
		memory.entries[entry.ID] = entry
	}
	service.store = &shareParallelStore{memory}
	entered, release := make(chan int, requests), make(chan struct{})
	var active, maximum, calls atomic.Int32
	service.SetPlaybackDirectLink(func(ctx context.Context, _ string, _ int, _ string, _ string) (string, error) {
		calls.Add(1)
		current := active.Add(1)
		for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
		}
		entered <- ctx.Value(shareLinkRequestID{}).(int)
		<-release
		active.Add(-1)
		return "https://cdn.example/video?t=4102444800", nil
	})
	cancels := make([]context.CancelFunc, requests)
	results := make([]error, requests)
	var wait sync.WaitGroup
	for request := 0; request < requests; request++ {
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), shareLinkRequestID{}, request))
		cancels[request] = cancel
		defer cancel()
		wait.Add(1)
		go func(request int) {
			defer wait.Done()
			_, results[request] = service.Playback(ctx, fmt.Sprint(request), "Player/1")
		}(request)
	}
	first, second := <-entered, <-entered
	awaitShareLinkWaiters(t, service.directLinks(), requests)
	queued := 0
	for queued == first || queued == second {
		queued++
	}
	cancels[queued]()
	awaitShareLinkCondition(t, func() bool {
		cache := service.directLinks()
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return len(cache.flights) == requests-1
	})
	if active.Load() != 2 || len(entered) != 0 {
		t.Fatal("excess URL applications did not queue")
	}
	close(release)
	wait.Wait()
	for request, err := range results {
		if request == queued {
			if !errors.Is(err, context.Canceled) {
				t.Fatal("queued cancellation lost")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != requests-1 || maximum.Load() != 2 || len(service.directLinks().slots) != 0 {
		t.Fatal("application count, concurrency or slots incorrect")
	}
}

func TestShareStrmDirectLinkQueueDeadlineAndPanicRelease(t *testing.T) {
	cache, clock := shareTestCache(t)
	cache.slots <- struct{}{}
	cache.slots <- struct{}{}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := cache.apply(ctx, func(context.Context) (string, error) { t.Error("expired queue applied upstream"); return "", nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("queue deadline ignored")
	}
	<-cache.slots
	<-cache.slots
	_, err := cache.resolve(context.Background(), [32]byte{}, [32]byte{}, func(ctx context.Context) (shareDirectLinkResult, error) {
		_, applyErr := cache.apply(ctx, func(context.Context) (string, error) { panic("fixture") })
		return shareDirectLinkResult{link: shareLinkForExpiry(clock.now().Add(time.Hour))}, applyErr
	})
	if err == nil || len(cache.slots) != 0 {
		t.Fatal("SDK panic leaked semaphore")
	}
}
