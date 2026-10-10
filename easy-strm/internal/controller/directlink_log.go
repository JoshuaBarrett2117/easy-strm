package controller

import (
	"time"

	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

// 只记录用于定位来源的字段，不记录完整查询串、Cookie、授权头和带签名的直链。
func beginDirectLinkRequest(ctx *gin.Context, source string) func() {
	requestContext, err := logger.HTTPContext(ctx.Request)
	if err != nil {
		ctx.AbortWithStatus(503)
		return func() {}
	}
	ctx.Request = ctx.Request.WithContext(requestContext)
	ctx.Header("X-Request-ID", logger.RequestID(requestContext))
	start := time.Now()
	entry := logger.WithContext(ctx.Request.Context(), "directlink")
	entry.Log(logger.INFO, "直链请求开始", logger.Fields{"event": "request", "source": source, "method": ctx.Request.Method, "route": ctx.FullPath()}, nil)
	return func() {
		entry.Log(logger.INFO, "直链请求完成", logger.Fields{"event": "complete", "source": source, "status": ctx.Writer.Status(), "duration_ms": time.Since(start).Milliseconds()}, nil)
	}
}
