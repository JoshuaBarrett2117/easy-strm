package service

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const sampleEmbyPluginJS = `function play(){var initialSubtitleStream=getSubtitle();initialSubtitleStream&&(elem.crossOrigin=initialSubtitleStream);initialSubtitleStream=function(track){return track};}`

func TestPatchEmbyWebPlayerRemovesCrossOrigin(t *testing.T) {
	patched := string(PatchEmbyWebPlayer([]byte(sampleEmbyPluginJS)))
	if strings.Contains(patched, "crossOrigin") {
		t.Fatalf("crossOrigin 未被移除: %s", patched)
	}
	if !strings.Contains(patched, "getSubtitle()") || !strings.Contains(patched, "initialSubtitleStream=function(track)") {
		t.Fatalf("其余逻辑被误删: %s", patched)
	}
}

func TestPatchEmbyWebPlayerIsNoopWithoutCrossOrigin(t *testing.T) {
	src := []byte("var a=1;function f(){return a}")
	if got := PatchEmbyWebPlayer(src); !bytes.Equal(got, src) {
		t.Fatalf("无 crossOrigin 时不应改动内容")
	}
}

func TestPatchEmbyWebPlayerFallsBackToLooseMatch(t *testing.T) {
	src := []byte(`if(x){video.crossOrigin="anonymous";video.play()}`)
	patched := string(PatchEmbyWebPlayer(src))
	if strings.Contains(patched, "crossOrigin") {
		t.Fatalf("宽松匹配未生效: %s", patched)
	}
	if !strings.Contains(patched, "video.play()") {
		t.Fatalf("其余逻辑被误删: %s", patched)
	}
}

func TestIsEmbyWebPlayerScript(t *testing.T) {
	cases := map[string]bool{
		"/web/modules/htmlvideoplayer/plugin.js":         true,
		"/emby/web/modules/htmlvideoplayer/plugin.js":    true,
		"/web/modules/htmlvideoplayer/basehtmlplayer.js": false,
		"/web/modules/hlsjs/hls.js":                      false,
	}
	for path, want := range cases {
		if got := IsEmbyWebPlayerScript(path); got != want {
			t.Fatalf("%s: got %v want %v", path, got, want)
		}
	}
}

func TestRewriteEmbyWebPlayerResponseHandlesGzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(sampleEmbyPluginJS)); err != nil {
		t.Fatalf("gzip 写入失败: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip 关闭失败: %v", err)
	}

	u, _ := url.Parse("http://emby.local/web/modules/htmlvideoplayer/plugin.js")
	response := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Encoding": []string{"gzip"},
			"Content-Length":   []string{"999"},
		},
		Body:    io.NopCloser(bytes.NewReader(buf.Bytes())),
		Request: &http.Request{URL: u},
	}

	if err := rewriteEmbyWebPlayerResponse(response); err != nil {
		t.Fatalf("修正失败: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "crossOrigin") {
		t.Fatalf("gzip 响应未去除 crossOrigin: %s", body)
	}
	if response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding 应被清除")
	}
	if response.ContentLength != int64(len(body)) {
		t.Fatalf("ContentLength 未同步: %d vs %d", response.ContentLength, len(body))
	}
}

func TestRewriteEmbyWebPlayerResponseIgnoresOtherFiles(t *testing.T) {
	u, _ := url.Parse("http://emby.local/web/modules/hlsjs/hls.js")
	original := `elem.crossOrigin="anonymous";`
	response := &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(original)),
		Request:    &http.Request{URL: u},
	}
	if err := rewriteEmbyWebPlayerResponse(response); err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != original {
		t.Fatalf("非播放器脚本不应被改动")
	}
}
