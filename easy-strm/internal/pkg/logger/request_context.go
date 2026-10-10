package logger

import (
	"context"
	"sync/atomic"
)

type requestIDKey struct{}
type taskIDKey struct{}
type traceIDKey struct{}
type sequenceKey struct{}

// WithRequestID 将服务端生成的请求标识放入上下文，供跨层日志关联使用。
func WithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, sequenceKey{}, &atomic.Uint64{})
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID 返回上下文中的请求标识；非 HTTP 调用返回空字符串。
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithTaskID 将任务标识绑定到派生上下文，不修改原请求上下文。
func WithTaskID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, taskIDKey{}, id)
}

// TaskID 返回任务标识；非任务调用返回空字符串。
func TaskID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(taskIDKey{}).(string)
	return id
}

// WithTraceID 绑定一次用户动作的聚合键，跨多个 HTTP 请求保持一致。
func WithTraceID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceID 返回动作聚合键；非请求调用返回空字符串。
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

func nextSequence(ctx context.Context) uint64 {
	if counter, _ := ctx.Value(sequenceKey{}).(*atomic.Uint64); counter != nil {
		return counter.Add(1)
	}
	return 0
}
