package logger

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var correlationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var traceparentPattern = regexp.MustCompile(`^00-([a-f0-9]{32})-([a-f0-9]{16})-[a-f0-9]{2}$`)

// HTTPContext 沿用上游标识或生成密码学随机 UUIDv7，随机源失败时返回错误而不降级。
func HTTPContext(request *http.Request) (context.Context, error) {
	ctx := request.Context()
	id := RequestID(ctx)
	if id == "" {
		id = request.Header.Get("X-Request-ID")
		if !correlationIDPattern.MatchString(id) {
			id = ""
			if match := traceparentPattern.FindStringSubmatch(strings.ToLower(request.Header.Get("traceparent"))); match != nil && match[1] != strings.Repeat("0", 32) && match[2] != strings.Repeat("0", 16) {
				id = match[1]
			}
		}
		if id == "" {
			generated, err := uuid.NewV7()
			if err != nil {
				return nil, err
			}
			id = generated.String()
		}
		ctx = WithRequestID(ctx, id)
	}
	if trace := request.Header.Get("X-Trace-ID"); correlationIDPattern.MatchString(trace) {
		ctx = WithTraceID(ctx, trace)
	}
	return ctx, nil
}
