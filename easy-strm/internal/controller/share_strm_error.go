package controller

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// 播放入口被 STRM 播放器或浏览器直接访问；默认返回文档，仅显式偏好 JSON 时使用 API 错误格式。
func writeSharePlaybackError(x *gin.Context, status int, err error) {
	info := service.DescribeSharePlaybackError(err)
	if status == http.StatusBadRequest {
		info.Category, info.Advice = "播放地址无效", "请使用重新导出的 STRM 播放地址。"
	}
	if status == http.StatusNotFound {
		info.Category, info.Advice = "播放映射不存在", "请重新导出 STRM 并刷新媒体库。"
	}
	x.Header("Cache-Control", "no-store")
	x.Header("Vary", "Accept")
	x.Header("X-Content-Type-Options", "nosniff")
	if sharePlaybackWantsJSON(x.Request) {
		x.Header("Content-Type", "application/json; charset=utf-8")
		if x.Request.Method == http.MethodHead {
			x.Status(status)
			return
		}
		ErrorResp(x, status, info.Category+"："+info.Detail+"；"+info.Advice)
		return
	}
	x.Header("Content-Type", "text/html; charset=utf-8")
	x.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
	x.Header("Referrer-Policy", "no-referrer")
	if x.Request.Method == http.MethodHead {
		x.Status(status)
		return
	}
	code := ""
	if info.Errno != "" {
		code = "<p>上游错误码 errno=" + html.EscapeString(info.Errno) + "</p>"
	}
	body := fmt.Sprintf(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>分享播放失败</title><style>body{font-family:system-ui,sans-serif;max-width:48rem;margin:8vh auto;padding:1.5rem;line-height:1.7;color:#222}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#f3f4f6;padding:1rem}</style><main><h1>分享播放失败</h1><p>HTTP %d · %s</p>%s<p>%s</p><h2>错误详情</h2><pre>%s</pre></main></html>`, status, html.EscapeString(info.Category), code, html.EscapeString(info.Advice), html.EscapeString(info.Detail))
	x.Data(status, "text/html; charset=utf-8", []byte(body))
}

func sharePlaybackWantsJSON(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Mode"), "navigate") {
		return false
	}
	jsonQ, htmlQ := 0.0, 0.0
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		kind, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		q := 1.0
		if raw, ok := params["q"]; ok {
			q, err = strconv.ParseFloat(raw, 64)
			if err != nil || q < 0 || q > 1 {
				continue
			}
		}
		switch kind {
		case "application/json":
			if q > jsonQ {
				jsonQ = q
			}
		case "text/html":
			if q > htmlQ {
				htmlQ = q
			}
		}
	}
	return jsonQ > 0 && jsonQ > htmlQ
}
