package service

import (
	"context"
	"net/http"
	"time"
)

type shareMetric struct {
	Count    int64
	Duration time.Duration
}

func observeShareMetric(ctx context.Context, phase string, elapsed time.Duration) {
	round, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	if !ok {
		return
	}
	round.mu.Lock()
	defer round.mu.Unlock()
	if round.metrics == nil {
		round.metrics = map[string]shareMetric{}
	}
	metric := round.metrics[phase]
	metric.Count++
	metric.Duration += elapsed
	round.metrics[phase] = metric
}

func shareMetricsSnapshot(ctx context.Context) map[string]int64 {
	result := map[string]int64{}
	round, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	if !ok {
		return result
	}
	round.mu.Lock()
	defer round.mu.Unlock()
	for phase, metric := range round.metrics {
		result[phase+"_count"] = metric.Count
		result[phase+"_ms"] = metric.Duration.Milliseconds()
	}
	return result
}

func (s *TmdbService) performShareTMDBRequest(req *http.Request) (*http.Response, error) {
	start := time.Now()
	defer func() { observeShareMetric(req.Context(), "tmdb_request", time.Since(start)) }()
	return s.httpClient.Do(req)
}
