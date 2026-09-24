package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestShareMetricsConcurrentCounters(t *testing.T) {
	ctx := WithRecognitionRound(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				observeShareMetric(ctx, "db", time.Millisecond)
			}
		}()
	}
	wg.Wait()
	got := shareMetricsSnapshot(ctx)
	if got["db_count"] != 2000 || got["db_ms"] != 2000 {
		t.Fatalf("%v", got)
	}
	if len(shareMetricsSnapshot(context.Background())) != 0 {
		t.Fatal("unexpected global metrics")
	}
}
