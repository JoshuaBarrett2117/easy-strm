package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"easy-strm/internal/pkg/logger"
)

type sharePreparationReport func(int, int)
type sharePreparationObserver func(string, logger.Fields, error)
type sharePreparationReportKey struct{}

func runSharePreparation(ctx context.Context, phase string, cancelled func() bool, heartbeat, poll time.Duration, observer sharePreparationObserver, work func(context.Context, sharePreparationReport) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	started := time.Now()
	processed, total := 0, -1
	sequence := 0
	var mutex sync.Mutex
	emit := func(event string, err error) {
		sequence++
		elapsed := time.Since(started).Seconds()
		rate := 0.0
		if elapsed > 0 {
			rate = float64(processed) / elapsed
		}
		observer(event, logger.Fields{"event": "preparation_" + event, "seq": sequence, "phase": phase, "processed": processed, "total": total, "rate": rate, "eta": "unknown", "elapsed_seconds": elapsed}, err)
	}
	report := func(done, count int) {
		mutex.Lock()
		defer mutex.Unlock()
		if ctx.Err() != nil {
			return
		}
		processed, total = done, count
		emit("batch", nil)
	}
	mutex.Lock()
	emit("start", nil)
	mutex.Unlock()
	stop, joined := make(chan struct{}), make(chan struct{})
	checkCancel := func() {
		if cancelled != nil && cancelled() {
			cancel()
		}
	}
	checkCancel()
	go func() {
		defer close(joined)
		heartbeats, polls := time.NewTicker(heartbeat), time.NewTicker(poll)
		defer heartbeats.Stop()
		defer polls.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-polls.C:
				checkCancel()
			case <-heartbeats.C:
				mutex.Lock()
				if ctx.Err() == nil {
					emit("heartbeat", nil)
				}
				mutex.Unlock()
			}
		}
	}()
	var stopOnce sync.Once
	join := func() {
		stopOnce.Do(func() { close(stop) })
		<-joined
	}
	defer join()
	err := ctx.Err()
	if err == nil {
		err = work(context.WithValue(ctx, sharePreparationReportKey{}, report), report)
	}
	checkCancel()
	join()
	err = errors.Join(err, ctx.Err())
	mutex.Lock()
	defer mutex.Unlock()
	if err != nil {
		event := "error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			event = "cancel"
		}
		emit(event, err)
	} else {
		emit("end", nil)
	}
	return err
}

func (s *ShareStrmService) preparationStage(ctx context.Context, phase string, work func(context.Context, sharePreparationReport) error) error {
	entry := logger.WithContext(ctx, "share_export")
	return runSharePreparation(ctx, phase, func() bool {
		id := logger.TaskID(ctx)
		if id == "" || s.tasks == nil {
			return false
		}
		pollCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer cancel()
		return s.tasks.IsCancelledContext(pollCtx, id)
	}, 2*time.Second, 200*time.Millisecond, func(_ string, fields logger.Fields, err error) {
		level := logger.INFO
		if err != nil {
			level = logger.ERROR
		}
		entry.Log(level, "分享 STRM 准备阶段", fields, err)
	}, work)
}
