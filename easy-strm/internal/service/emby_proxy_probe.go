package service

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"easy-strm/internal/pkg/logger"
)

const (
	embyProxyProbeDeadline       = 15 * time.Second
	embyProxyProbeAttemptTimeout = 3 * time.Second
	embyProxyProbeRetryInterval  = 2 * time.Second
)

func validateEmbyProxyMediaLocation(location string) error {
	if strings.ContainsAny(location, "\r\n") {
		return errEmbyProxyRelayUnsafe
	}
	location = strings.TrimSpace(location)
	parsed, err := url.Parse(location)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errEmbyProxyRelayUnsafe
	}
	if address, err := netip.ParseAddr(parsed.Hostname()); err == nil && !isEmbyProxyRelayAllowedIP(address) {
		return errEmbyProxyRelayUnsafe
	}
	return nil
}

// prepareMediaLocation 独立执行可选预解析与 HEAD 探测，输入已确认 STRM 原地址，输出原地址和最终地址。
// 网络与状态失败的预解析保留原地址，探测失败保留原地址供安全中继；任何一跳不安全均不可降级。
func (s *EmbyProxyService) prepareMediaLocation(incoming *http.Request, original string) embyProxyLocationResult {
	result := embyProxyLocationResult{location: original, original: original}
	if err := validateEmbyProxyMediaLocation(original); err != nil {
		result.failure = newEmbyProxyFailure("redirect", "unsafe_location", "媒体播放地址不可安全访问，已禁止回源", 0, err)
		return result
	}
	if !s.options.ResolveFinalURL && !s.options.ProbeHEAD {
		return result
	}
	ctx, cancel := context.WithTimeout(incoming.Context(), embyProxyProbeDeadline)
	defer cancel()
	stop := context.AfterFunc(s.relayContext, cancel)
	defer stop()
	if s.options.ResolveFinalURL {
		final, failure := s.mediaHEAD(ctx, incoming, original, false)
		if failure == nil {
			result.location = final
		} else if failure.reason == "unsafe_location" || incoming.Context().Err() != nil || s.relayContext.Err() != nil {
			result.failure = failure
			return result
		} else {
			logger.WithContext(incoming.Context(), "emby_proxy").Log(logger.WARN, "媒体直链预解析失败，保留原地址", logger.Fields{"stage": failure.stage, "reason": failure.reason, "upstream_status": failure.upstreamStatus, "detail": failure.detail}, nil)
		}
	}
	if s.options.ProbeHEAD {
		_, result.failure = s.mediaHEAD(ctx, incoming, result.location, true)
	}
	return result
}

// mediaHEAD 每次从输入 URL 发起无凭据 HEAD，复用中继逐跳 DNS 校验与固定 IP 拨号。
// 最多三次、间隔两秒、单次三秒；关闭所有响应体，返回最终 URL 或独立错误，不记录原始 URL。
func (s *EmbyProxyService) mediaHEAD(ctx context.Context, incoming *http.Request, location string, probe bool) (string, *embyProxyFailure) {
	var failure *embyProxyFailure
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(embyProxyProbeRetryInterval)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return "", newEmbyProxyFailure("probe", "connection_failed", "媒体探测已取消或超时", 0, ctx.Err())
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, embyProxyProbeAttemptTimeout)
		request := incoming.Clone(attemptCtx)
		request.Method = http.MethodHead
		request.Header = make(http.Header)
		request.Header["User-Agent"] = append([]string(nil), incoming.Header.Values("User-Agent")...)
		response, fetchFailure := s.fetchRelayMedia(attemptCtx, request, location)
		if fetchFailure != nil {
			failure = fetchFailure
			cancel()
			if failure.reason == "unsafe_location" || ctx.Err() != nil {
				return "", failure
			}
			continue
		}
		status := response.StatusCode
		contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		invalidJSON := probe && (strings.EqualFold(contentType, "application/json") || (err != nil && strings.EqualFold(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]), "application/json")))
		final := response.Request.URL.String()
		closeErr := response.Body.Close()
		cancel()
		if status >= 200 && status < 300 && !invalidJSON && closeErr == nil {
			return final, nil
		}
		reason := "upstream_http_error"
		if invalidJSON {
			reason = "invalid_media_type"
		}
		failure = newEmbyProxyFailure("probe", reason, "媒体直链 HEAD 探测失败", status, closeErr)
	}
	if failure == nil {
		failure = newEmbyProxyFailure("probe", "connection_failed", "媒体直链 HEAD 探测失败", 0, errors.New("探测未完成"))
	}
	return "", failure
}
