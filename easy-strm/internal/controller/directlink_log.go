package controller

import (
	"time"

	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 只记录用于定位来源的字段，不记录完整查询串、Cookie、授权头和带签名的直链。
func beginDirectLinkRequest(ctx *gin.Context, source string) func() {
	id := uuid.NewString()
	ctx.Request = ctx.Request.WithContext(logger.WithRequestID(ctx.Request.Context(), id))
	ctx.Header("X-Request-ID", id)
	start := time.Now()
	logger.Infof("[DirectLink] event=request request_id=%s source=%s method=%s path=%q client_ip=%q remote_addr=%q ua=%q range=%q account=%q pickcode=%q file_path=%q entry_id=%q",
		id, source, ctx.Request.Method, ctx.Request.URL.Path, ctx.ClientIP(), ctx.Request.RemoteAddr,
		ctx.Request.UserAgent(), ctx.GetHeader("Range"), ctx.Query("cloud115_id"), ctx.Query("pickcode"), ctx.Query("path"), ctx.Param("id"))
	return func() {
		logger.Infof("[DirectLink] event=complete request_id=%s source=%s status=%d duration_ms=%d",
			id, source, ctx.Writer.Status(), time.Since(start).Milliseconds())
	}
}
