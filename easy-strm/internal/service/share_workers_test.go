package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestShareWorkersBoundedIsolatedAndCancelled(t *testing.T) {
	items := make([]domain.ShareMedia, 20)
	for i := range items {
		items[i].ID = i + 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 20)
	release := make(chan struct{})
	done := make(chan struct{})
	var active, peak, finished atomic.Int32
	go func() {
		defer close(done)
		runShareIdentifyWorkers(ctx, items, 4, func(ctx context.Context, m domain.ShareMedia) (bool, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			if m.ID == 1 {
				panic("fixture")
			}
			if m.ID == 2 {
				return false, errors.New("fixture")
			}
			return true, nil
		}, func(m domain.ShareMedia, ok bool, err error) { finished.Add(1) })
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	select {
	case <-entered:
		t.Fatal("worker limit exceeded")
	default:
	}
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workers did not stop")
	}
	if peak.Load() != 4 || finished.Load() > 4 {
		t.Fatalf("peak=%d finished=%d", peak.Load(), finished.Load())
	}
}

func TestShareWorkersContinueAfterPanic(t *testing.T) {
	items := []domain.ShareMedia{{ID: 1}, {ID: 2}, {ID: 3}}
	seen := map[int]bool{}
	failed := 0
	runShareIdentifyWorkers(context.Background(), items, 1, func(_ context.Context, m domain.ShareMedia) (bool, error) {
		if m.ID == 1 {
			panic("fixture")
		}
		return true, nil
	}, func(m domain.ShareMedia, ok bool, err error) {
		seen[m.ID] = true
		if !ok || err != nil {
			failed++
		}
	})
	if len(seen) != 3 || failed != 1 {
		t.Fatalf("seen=%v failed=%d", seen, failed)
	}
}
