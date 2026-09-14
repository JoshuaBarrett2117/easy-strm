package service

import (
	"context"
	"time"
)

// 将旧任务的停止回调桥接到识别请求上下文；退出时必须调用cancel释放轮询。
func organizeCancellationContext(parent context.Context, shouldStop func() bool) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if shouldStop == nil {
		return ctx, cancel
	}
	if shouldStop() {
		cancel()
		return ctx, cancel
	}
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if shouldStop() {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}
