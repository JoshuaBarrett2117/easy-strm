package main

import (
	"fmt"
	"net/http"

	"time"

	pkglogger "easy-strm/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

func requestLoggingMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestContext, err := pkglogger.HTTPContext(ctx.Request)
		if err != nil {
			ctx.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		ctx.Request = ctx.Request.WithContext(requestContext)
		ctx.Header("X-Request-ID", pkglogger.RequestID(requestContext))
		started := time.Now()
		ctx.Next()
		elapsed := time.Since(started)
		status := ctx.Writer.Status()
		level := requestLogLevel(status, elapsed)
		if ctx.FullPath() == healthzPath {
			// 探针高频且由 healthz 自身按状态变化记录，访问日志一律 DEBUG。
			level = pkglogger.DEBUG
		}
		pkglogger.WithContext(ctx.Request.Context(), "http").Log(level, "请求完成", pkglogger.Fields{
			"method": ctx.Request.Method, "route": ctx.FullPath(), "status": status, "duration_ms": elapsed.Milliseconds(),
		}, nil)
	}
}

// slowRequestThreshold 超过该耗时的请求即使成功也记录为 INFO，便于定位性能问题。
const slowRequestThreshold = 2 * time.Second

// requestLogLevel 按结果决定访问日志级别：
// 5xx -> ERROR；慢请求或 4xx（401 除外）-> INFO/WARN；其余成功请求（含前端轮询、MCP 调用）-> DEBUG，避免刷屏。
func requestLogLevel(status int, elapsed time.Duration) pkglogger.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return pkglogger.ERROR
	case status == http.StatusUnauthorized:
		return pkglogger.DEBUG
	case status >= http.StatusBadRequest:
		return pkglogger.WARN
	case elapsed >= slowRequestThreshold:
		return pkglogger.INFO
	default:
		return pkglogger.DEBUG
	}
}

func safeRecoveryMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if failure := recover(); failure != nil {
				pkglogger.WithContext(ctx.Request.Context(), "http").Log(pkglogger.ERROR, "请求处理异常，原始异常内容已省略", pkglogger.Fields{"panic_type": fmt.Sprintf("%T", failure)}, nil)
				ctx.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		ctx.Next()
	}
}
