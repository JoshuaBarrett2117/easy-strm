package main

import (
	"fmt"
	"net/http"
	"time"

	pkglogger "easy-strm/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func requestLoggingMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		id := uuid.NewString()
		ctx.Request = ctx.Request.WithContext(pkglogger.WithRequestID(ctx.Request.Context(), id))
		ctx.Header("X-Request-ID", id)
		started := time.Now()
		ctx.Next()
		pkglogger.WithContext(ctx.Request.Context(), "http").Log(pkglogger.INFO, "请求完成", pkglogger.Fields{
			"method": ctx.Request.Method, "route": ctx.FullPath(), "status": ctx.Writer.Status(), "duration_ms": time.Since(started).Milliseconds(),
		}, nil)
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
