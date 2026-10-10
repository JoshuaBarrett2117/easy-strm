package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type embyProxyFailure struct {
	stage          string
	reason         string
	message        string
	detail         string
	mediaSourceID  string
	status         int
	upstreamStatus int
	blocked        bool
}

func newEmbyProxyFailure(stage, reason, message string, upstreamStatus int, err error) *embyProxyFailure {
	return &embyProxyFailure{stage: stage, reason: reason, message: message, detail: embyProxyErrorDetail(err), status: http.StatusBadGateway, upstreamStatus: upstreamStatus, blocked: true}
}

type embyProxyRequestLogKey struct{}

type embyProxyRequestLog struct {
	started  time.Time
	incoming *http.Request
	playback embyProxyPlaybackRequest
}

func beginEmbyProxyRequest(r *http.Request, w http.ResponseWriter, playback embyProxyPlaybackRequest) *http.Request {
	ctx, err := logger.HTTPContext(r)
	if err != nil {
		logger.WithContext(r.Context(), "emby_proxy").Log(logger.ERROR, "请求标识随机源不可用", nil, err)
		return r
	}
	w.Header().Set("X-Request-ID", logger.RequestID(ctx))
	ctx = context.WithValue(ctx, embyProxyRequestLogKey{}, embyProxyRequestLog{started: time.Now(), incoming: r, playback: playback})
	return r.WithContext(ctx)
}

func writeEmbyProxyFailure(server *domain.EmbyServer, w http.ResponseWriter, r *http.Request, failure *embyProxyFailure) {
	logEmbyProxyFailure(server, r, failure)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(failure.status)
		return
	}
	http.Error(w, failure.message, failure.status)
}

func logEmbyProxyFailure(server *domain.EmbyServer, r *http.Request, failure *embyProxyFailure) {
	info, _ := r.Context().Value(embyProxyRequestLogKey{}).(embyProxyRequestLog)
	incoming := r
	elapsed := int64(0)
	if info.incoming != nil {
		incoming = info.incoming
		elapsed = time.Since(info.started).Milliseconds()
	}
	sourceID := failure.mediaSourceID
	if sourceID == "" {
		sourceID = embyQuery(incoming.URL.Query(), "MediaSourceId")
	}
	logger.WithContext(r.Context(), "emby_proxy").Log(logger.ERROR, failure.message, logger.Fields{
		"event": "failed", "server_id": server.ID, "proxy_port": server.ProxyPort, "item_id": info.playback.itemID, "media_source_id": sourceID,
		"method": incoming.Method, "stage": failure.stage, "reason": failure.reason, "status": failure.status,
		"upstream_status": failure.upstreamStatus, "duration_ms": elapsed, "fallback_blocked": failure.blocked, "detail": failure.detail,
	}, nil)
}

// 仅输出可控的诊断信息；url.Error 的完整文本包含查询 Token，任意上游错误文本也可能包含直链。
func embyProxyErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		err = urlError.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "上游请求超时"
	}
	if errors.Is(err, context.Canceled) {
		return "请求已取消"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "上游连接超时"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "上游响应提前结束"
	}
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return fmt.Sprintf("JSON 语法错误，字节位置=%d", syntaxError.Offset)
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		return "JSON 字段类型错误"
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) {
		return fmt.Sprintf("网络操作失败（%s/%s）", operationError.Op, operationError.Net)
	}
	return fmt.Sprintf("错误类型 %T", err)
}
