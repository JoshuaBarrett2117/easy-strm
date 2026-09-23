package logger

import "context"

type requestIDKey struct{}

// WithRequestID 将服务端生成的请求标识放入上下文，供跨层日志关联使用。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID 返回上下文中的请求标识；非 HTTP 调用返回空字符串。
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
