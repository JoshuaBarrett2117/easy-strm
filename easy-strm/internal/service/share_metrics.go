package service

import (
	"context"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
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

func logShareMetrics(ctx context.Context, taskID string) {
	// 仅输出任务ID与固定指标名；不包含路径、请求URL、查询参数或错误正文。
	payload, _ := json.Marshal(struct {
		Event   string           `json:"event"`
		TaskID  string           `json:"task_id"`
		Metrics map[string]int64 `json:"metrics"`
	}{"share_identify_metrics", taskID, shareMetricsSnapshot(ctx)})
	logger.Infof("%s", payload)
}

func (s *TmdbService) performShareTMDBRequest(req *http.Request) (*http.Response, error) {
	start := time.Now()
	defer func() { observeShareMetric(req.Context(), "tmdb_request", time.Since(start)) }()
	return s.httpClient.Do(req)
}

func observeShareFilePhase(ctx context.Context, fileID int, phase string, elapsed time.Duration) {
	observeShareMetric(ctx, phase, elapsed)
	payload, _ := json.Marshal(struct {
		Event     string `json:"event"`
		FileID    int    `json:"file_id"`
		Phase     string `json:"phase"`
		ElapsedMS int64  `json:"elapsed_ms"`
	}{"share_file_phase", fileID, phase, elapsed.Milliseconds()})
	logger.Infof("%s", payload)
}
