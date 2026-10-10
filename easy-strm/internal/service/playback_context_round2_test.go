package service

import (
	"context"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

func TestPlaybackListPosterGoroutineRetainsRequestContext(t *testing.T) {
	store := &playbackStoreStub{records: []domain.PlaybackRecord{{ID: "fixture-record", Name: "Fixture"}}}
	service := NewPlaybackRecordService(store)
	contexts := make(chan context.Context, 1)
	release := make(chan struct{})
	service.SetPosterResolver(func(ctx context.Context, name string) (string, error) { contexts <- ctx; <-release; return "", nil })
	parent, cancel := context.WithCancel(logger.WithTraceID(logger.WithRequestID(context.Background(), "poster-request"), "poster-action"))
	defer cancel()
	if _, _, err := service.List(parent, 0, 10); err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case ctx := <-contexts:
		cancel()
		if logger.RequestID(ctx) != "poster-request" || logger.TraceID(ctx) != "poster-action" || ctx.Err() != nil {
			close(release)
			t.Fatal("播放列表后台海报任务丢失关联或沿用 HTTP 取消")
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("海报 goroutine 未启动")
	}
	close(release)
}
