package service

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"easy-strm/internal/domain"
	"github.com/andybalholm/brotli"
)

func TestEmbyWebCacheRewriteMatrix(t *testing.T) {
	for _, tc := range []struct{ path, source, want string }{
		{"/web/index.html", `<html><script src="apploader.js" defer></script></html>`, `apploader.js?corsfix=` + EmbyWebCORSRevision},
		{"/web/index.html", `<script src="apploader.js?v=4.9.1.90" defer>`, `apploader.js?corsfix=` + EmbyWebCORSRevision + `&v=4.9.1.90`},
		{"/web/index.html", `<script defer src='apploader.js?v=4.9.1.90'>`, `apploader.js?corsfix=` + EmbyWebCORSRevision + `&v=4.9.1.90`},
		{"/web/index.html", `<script src="apploader.js?corsfix=old&v=4.9.1.90">`, `apploader.js?corsfix=` + EmbyWebCORSRevision + `&v=4.9.1.90`},
		{"/", `<html><script src="apploader.js" defer></script></html>`, `apploader.js?corsfix=` + EmbyWebCORSRevision},
		{"/web/", `<html><script src="apploader.js" defer></script></html>`, `apploader.js?corsfix=` + EmbyWebCORSRevision},
		{"/emby/web/index.html", `<script src="apploader.js" defer>`, `apploader.js?corsfix=` + EmbyWebCORSRevision},
		{"/web/apploader.js", `globalThis.urlCacheParam="v="+docElem;`, `globalThis.urlCacheParam="v="+(docElem||"")+"&corsfix=` + EmbyWebCORSRevision + `";`},
		{"/web/modules/htmlvideoplayer/plugin.js", sampleEmbyPluginJS, `getSubtitle()`},
	} {
		for _, encoding := range []string{"identity", "gzip", "br", "gzip, br"} {
			t.Run(tc.path+"/"+encoding, func(t *testing.T) {
				source := []byte(tc.source)
				if strings.Contains(encoding, "gzip") {
					var b bytes.Buffer
					w := gzip.NewWriter(&b)
					_, _ = w.Write(source)
					_ = w.Close()
					source = b.Bytes()
				}
				if strings.Contains(encoding, "br") {
					var b bytes.Buffer
					w := brotli.NewWriter(&b)
					_, _ = w.Write(source)
					_ = w.Close()
					source = b.Bytes()
				}
				request := httptest.NewRequest("GET", tc.path, nil)
				response := &http.Response{StatusCode: 200, Request: request, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(source)), TransferEncoding: []string{"chunked"}}
				for k, v := range map[string]string{"Content-Type": "text/html", "Content-Encoding": encoding, "Transfer-Encoding": "chunked", "ETag": "old", "Last-Modified": "old", "Content-Length": "999", "Cache-Control": "public, max-age=31536000"} {
					response.Header.Set(k, v)
				}
				if err := rewriteEmbyWebPlayerResponse(response); err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), tc.want) || strings.Contains(string(body), "crossOrigin") {
					t.Fatalf("unexpected body: %s", body)
				}
				for _, k := range []string{"Content-Encoding", "Transfer-Encoding", "ETag", "Last-Modified"} {
					if response.Header.Get(k) != "" {
						t.Fatalf("stale header %s", k)
					}
				}
				if len(response.TransferEncoding) != 0 || response.ContentLength != int64(len(body)) || response.Header.Get("Content-Length") != strconv.Itoa(len(body)) {
					t.Fatal("length/transfer mismatch")
				}
				if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
					t.Fatal("patched response cacheable")
				}
			})
		}
	}
}

func TestEmbyWebCacheDoesNotTouchOtherResponses(t *testing.T) {
	for _, tc := range []struct {
		path, contentType string
		status            int
	}{
		{"/web/index.html.bak", "text/html", 200}, {"/web/apploader.js.map", "text/javascript", 200},
		{"/web/modules/htmlvideoplayer/plugin.js.map", "text/javascript", 200}, {"/", "application/json", 200},
		{"/web/index.html", "text/html", 302}, {"/web/apploader.js", "text/javascript", 304},
		{"/web/modules/htmlvideoplayer/plugin.js", "text/javascript", 404},
	} {
		response := &http.Response{StatusCode: tc.status, Request: httptest.NewRequest("GET", tc.path, nil), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(sampleEmbyPluginJS))}
		response.Header.Set("ETag", "unchanged")
		response.Header.Set("Content-Type", tc.contentType)
		if err := rewriteEmbyWebPlayerResponse(response); err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(response.Body)
		if string(b) != sampleEmbyPluginJS || response.Header.Get("ETag") != "unchanged" {
			t.Fatalf("unexpected rewrite: %s", tc.path)
		}
	}
}

func TestEmbyWebCacheProxyBypassesValidatorsAndPlaybackCache(t *testing.T) {
	for _, path := range []string{"/web/index.html", "/web/apploader.js", "/web/modules/htmlvideoplayer/plugin.js"} {
		t.Run(path, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Accept-Encoding") != "identity" || r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" || r.Header.Get("Range") != "" {
					t.Errorf("validators/encoding not cleared: %v", r.Header)
				}
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("ETag", "old")
				_, _ = io.WriteString(w, `<script src="apploader.js" defer>globalThis.urlCacheParam="v="+docElem;`+sampleEmbyPluginJS)
			}))
			defer upstream.Close()
			svc := NewEmbyProxyService(nil, upstream.Client())
			handler := svc.buildHandler(&domain.EmbyServer{BaseURL: upstream.URL})
			for i := 0; i < 2; i++ {
				r := httptest.NewRequest("GET", path+"?v=4.9.1.90", nil)
				r.Header.Set("Accept-Encoding", "br")
				r.Header.Set("If-None-Match", "old")
				r.Header.Set("If-Modified-Since", "old")
				r.Header.Set("Range", "bytes=0-1")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != 200 || w.Header().Get("ETag") != "" {
					t.Fatalf("bad response: %d %v", w.Code, w.Header())
				}
			}
			if calls != 2 || len(svc.cache.entries) != 0 {
				t.Fatalf("web resource entered playback cache: calls=%d entries=%d", calls, len(svc.cache.entries))
			}
		})
	}
	request := httptest.NewRequest("GET", "/Videos/42/stream", nil)
	request.Header.Set("Accept-Encoding", "br")
	request.Header.Set("Range", "bytes=42-")
	prepareEmbyWebPatchRequest(request)
	if request.Header.Get("Range") != "bytes=42-" || request.Header.Get("Accept-Encoding") != "br" {
		t.Fatal("media request changed")
	}
}

func TestEmbyWebCacheInvalidEncodingFailsClosed(t *testing.T) {
	for _, encoding := range []string{"gzip", "br", "zstd"} {
		response := &http.Response{StatusCode: 200, Request: httptest.NewRequest("GET", "/web/apploader.js", nil), Header: make(http.Header), Body: io.NopCloser(strings.NewReader("not-compressed"))}
		response.Header.Set("Content-Encoding", encoding)
		if err := rewriteEmbyWebPlayerResponse(response); err == nil {
			t.Fatalf("invalid %s accepted", encoding)
		}
	}
}
