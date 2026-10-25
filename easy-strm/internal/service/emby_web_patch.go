package service

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"easy-strm/internal/pkg/logger"
)

// Emby Web 播放器跨域修正。
//
// 问题：Emby Web 的 htmlvideoplayer 在存在字幕流时会执行
// `initialSubtitleStream && (elem.crossOrigin = initialSubtitleStream)`，
// 给 <video> 挂上 crossorigin，浏览器于是按 CORS 模式取媒体。
// 而 302 方案把播放请求导向 115 CDN，115 不返回 Access-Control-Allow-Origin，
// 浏览器直接判 CORS 失败——请求本身 200/206 成功，却拿不到数据。
// 原生客户端不走浏览器同源策略，所以只有网页端报错。
//
// 处理：在反代层剥掉这行 crossOrigin 赋值，让浏览器用 no-cors 模式加载媒体，
// 302 直连 CDN 的方式保持不变，服务端不中转任何媒体字节。
// 代价：浏览器端内封字幕的 JS 叠加能力会退化（外挂字幕不受影响）。

// embyPlayerCrossOriginCall 匹配插件里的 crossOrigin 赋值语句。
var embyPlayerCrossOriginCall = regexp.MustCompile(
	`initialSubtitleStream&&\(elem\.crossOrigin=[A-Za-z0-9_$.]+\);`,
)

// embyPlayerCrossOriginLoose 兑底：上游改了变量名时只去掉赋值本身。
var embyPlayerCrossOriginLoose = regexp.MustCompile(
	`[A-Za-z_$][A-Za-z0-9_$.]*\.crossOrigin=[A-Za-z0-9_$."']+;?`,
)

// IsEmbyWebPlayerScript 判断是否是需要修正的播放器脚本。
func IsEmbyWebPlayerScript(path string) bool {
	return strings.HasSuffix(path, "/modules/htmlvideoplayer/plugin.js")
}

// PatchEmbyWebPlayer 去掉播放器脚本里的 crossOrigin 赋值。
func PatchEmbyWebPlayer(body []byte) []byte {
	if !bytes.Contains(body, []byte("crossOrigin")) {
		return body
	}
	patched := embyPlayerCrossOriginCall.ReplaceAll(body, []byte(""))
	if bytes.Equal(patched, body) {
		patched = embyPlayerCrossOriginLoose.ReplaceAll(body, []byte(""))
	}
	return patched
}

// rewriteEmbyWebPlayerResponse 读取、修正并回写脚本响应，顺便清掉压缩与长度头。
func rewriteEmbyWebPlayerResponse(response *http.Response) error {
	if !IsEmbyWebPlayerScript(response.Request.URL.Path) {
		return nil
	}
	var reader io.Reader = response.Body
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(response.Body)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	patched := PatchEmbyWebPlayer(body)

	response.Body = io.NopCloser(bytes.NewReader(patched))
	response.ContentLength = int64(len(patched))
	response.Header.Del("Content-Encoding")
	response.Header.Del("Transfer-Encoding")
	response.Header.Set("Content-Length", strconv.Itoa(len(patched)))
	logger.WithContext(response.Request.Context(), "emby_proxy").Log(
		logger.DEBUG, "已去除播放器 crossorigin 属性",
		logger.Fields{"before": len(body), "after": len(patched)}, nil)
	return nil
}
