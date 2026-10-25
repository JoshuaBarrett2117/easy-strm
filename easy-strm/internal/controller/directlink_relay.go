package controller

import (
	"io"
	"net/http"
	"strings"
	"time"

	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

// 浏览器播放的直链中继。
//
// 背景：/direct-link 原本一律 302 跳到 115 CDN。原生客户端（Emby App、爆米花、Yamby）
// 不执行同源策略，跳到哪都能播；但浏览器必须校验 CORS，而 115 CDN 不会返回
// Access-Control-Allow-Origin，于是 302 之后的响应在浏览器里直接表现为 CORS error，
// 连响应报文都拿不到。
//
// 因此对浏览器请求改为服务端取流回传：响应同源、带 CORS 头、透传 Range，
// 让 <video> 的拖动进度条和分片请求继续可用。原生客户端仍走 302，零影响。

// relayHTTPClient 专用于中继取流：不设总超时（大文件要连续传），只限制首包时间。
var relayHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true,
	},
}

// browserRelayEnabled 控制是否对浏览器启用中继；关闭后行为回到「一律 302」。
var browserRelayEnabled = true

// SetBrowserRelayEnabled 由启动装配处根据配置注入。
func SetBrowserRelayEnabled(enabled bool) {
	browserRelayEnabled = enabled
}

// IsBrowserPlayback 判断请求是否来自浏览器。
// 只认 Mozilla 系 UA 且带浏览器专有请求头，避免把伪装 UA 的原生客户端误判成浏览器。
func IsBrowserPlayback(ctx *gin.Context) bool {
	ua := strings.ToLower(ctx.GetHeader("User-Agent"))
	if !strings.Contains(ua, "mozilla/") {
		return false
	}
	// 原生客户端偶尔也带 Mozilla 前缀，但不会发这些头
	if ctx.GetHeader("Sec-Fetch-Mode") != "" || ctx.GetHeader("Origin") != "" {
		return true
	}
	return strings.Contains(ctx.GetHeader("Accept"), "text/html")
}

// applyDirectLinkCORS 给直链响应补 CORS 头；无 Origin 的请求不加，避免污染缓存。
func applyDirectLinkCORS(ctx *gin.Context) {
	origin := ctx.GetHeader("Origin")
	if origin == "" {
		return
	}
	h := ctx.Writer.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges, Content-Type, Content-Disposition, ETag")
	if h.Get("Vary") == "" {
		h.Set("Vary", "Origin")
	}
}

// HandleDirectLinkPreflight 响应浏览器预检，回显请求的方法与头，不写死白名单。
func HandleDirectLinkPreflight(ctx *gin.Context) {
	applyDirectLinkCORS(ctx)
	h := ctx.Writer.Header()
	if m := ctx.GetHeader("Access-Control-Request-Method"); m != "" {
		h.Set("Access-Control-Allow-Methods", m+", GET, HEAD, OPTIONS")
	} else {
		h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	}
	if reqHeaders := ctx.GetHeader("Access-Control-Request-Headers"); reqHeaders != "" {
		h.Set("Access-Control-Allow-Headers", reqHeaders)
	}
	h.Set("Access-Control-Max-Age", "86400")
	ctx.Status(http.StatusNoContent)
}

// relayDirectLink 由服务端取流回传，透传 Range，补 CORS 头。
func (c *DirectLinkController) relayDirectLink(ctx *gin.Context, target string, clientUA string) {
	req, err := http.NewRequestWithContext(ctx.Request.Context(), ctx.Request.Method, target, nil)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "invalid direct link"})
		return
	}
	if clientUA != "" {
		req.Header.Set("User-Agent", clientUA)
	}
	// 透传范围请求，浏览器拖动进度条依赖它
	for _, k := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := ctx.GetHeader(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("Accept", "*/*")

	resp, err := relayHTTPClient.Do(req)
	if err != nil {
		logger.WithContext(ctx.Request.Context(), "directlink").Log(logger.ERROR, "直链中继失败", nil, err)
		applyDirectLinkCORS(ctx)
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "upstream fetch failed"})
		return
	}
	defer resp.Body.Close()

	out := ctx.Writer.Header()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	applyDirectLinkCORS(ctx)

	if ctx.Request.Method == http.MethodHead {
		ctx.Status(resp.StatusCode)
		return
	}
	ctx.Status(resp.StatusCode)
	if _, err := io.Copy(ctx.Writer, resp.Body); err != nil {
		// 客户端主动中断（切集、关页面）是常态，只记 debug
		logger.WithContext(ctx.Request.Context(), "directlink").Log(logger.DEBUG, "直链中继传输中断", logger.Fields{"err": err.Error()}, nil)
	}
}
