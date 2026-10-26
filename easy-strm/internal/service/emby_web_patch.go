package service

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"easy-strm/internal/pkg/logger"
	"github.com/andybalholm/brotli"
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

// EmbyWebCORSRevision 是唯一破缓存版本键；升级补丁时只改这里，不伪造 data-appversion。
const EmbyWebCORSRevision = "20261026b"

// embyWebPatchKind 只处理入口 HTML、加载器以及既有播放器脚本路径。
func embyWebPatchKind(path string) string {
	switch {
	case path == "/" || path == "/web/" || strings.HasSuffix(path, "/web/index.html"):
		return "index"
	case strings.HasSuffix(path, "/web/apploader.js"):
		return "loader"
	case IsEmbyWebPlayerScript(path):
		return "player"
	default:
		return ""
	}
}

// prepareEmbyWebPatchRequest 不允许旧验证器/Range 导致 304/206 跳过补丁。
// 仅网页静态资源走此分支；媒体请求及其 302/Range 完全不变。
func prepareEmbyWebPatchRequest(request *http.Request) {
	if embyWebPatchKind(request.URL.Path) == "" || (request.Method != http.MethodGet && request.Method != http.MethodHead) {
		return
	}
	request.Header.Set("Accept-Encoding", "identity")
	for _, name := range []string{"If-None-Match", "If-Modified-Since", "If-Match", "If-Unmodified-Since", "If-Range", "Range"} {
		request.Header.Del(name)
	}
}

// rewriteEmbyWebPlayerResponse 统一修正三类网页资源。保留原有播放器正则与兜底。
func rewriteEmbyWebPlayerResponse(response *http.Response) error {
	if response.Request == nil || response.Request.URL == nil || response.StatusCode != http.StatusOK || response.Request.Method == http.MethodHead {
		return nil
	}
	kind := embyWebPatchKind(response.Request.URL.Path)
	if kind == "" || (kind == "index" && !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html")) {
		return nil
	}
	defer response.Body.Close()
	var reader io.Reader = response.Body
	// 即使上游忽略 identity，仍能解 gzip/br（含编码链，按逆序解码）。
	encodings := strings.Split(strings.ToLower(response.Header.Get("Content-Encoding")), ",")
	for i := len(encodings) - 1; i >= 0; i-- {
		switch encoding := strings.TrimSpace(encodings[i]); encoding {
		case "", "identity":
		case "gzip":
			gz, err := gzip.NewReader(reader)
			if err != nil {
				return err
			}
			defer gz.Close()
			reader = gz
		case "br":
			reader = brotli.NewReader(reader)
		default:
			return fmt.Errorf("unsupported Emby web encoding: %s", encoding)
		}
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	patched := body
	switch kind {
	case "index":
		patched = bytes.ReplaceAll(body, []byte(`<script src="apploader.js"`), []byte(`<script src="apploader.js?corsfix=`+EmbyWebCORSRevision+`"`))
	case "loader":
		patched = bytes.ReplaceAll(body, []byte(`globalThis.urlCacheParam="v="+docElem`), []byte(`globalThis.urlCacheParam="v="+(docElem||"")+"&corsfix=`+EmbyWebCORSRevision+`"`))
	case "player":
		patched = PatchEmbyWebPlayer(body)
	}
	response.Body = io.NopCloser(bytes.NewReader(patched))
	response.ContentLength = int64(len(patched))
	response.TransferEncoding = nil
	response.Uncompressed = true
	for _, name := range []string{"Content-Encoding", "Transfer-Encoding", "ETag", "Last-Modified", "Content-MD5", "Digest", "Content-Range", "Accept-Ranges"} {
		response.Header.Del(name)
	}
	response.Header.Set("Content-Length", strconv.Itoa(len(patched)))
	response.Header.Set("Cache-Control", "no-cache, no-store, no-transform, must-revalidate")
	logger.WithContext(response.Request.Context(), "emby_proxy").Log(
		logger.DEBUG, "Emby 网页跨域与破缓存修正",
		logger.Fields{"kind": kind, "revision": EmbyWebCORSRevision, "before": len(body), "after": len(patched)}, nil)
	return nil
}
