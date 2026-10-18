package controller

import (
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSharePlaybackErrorRepresentation(t *testing.T) {
	for _, tc := range []struct {
		name, id, accept, detail string
		err                      error
		status                   int
		json                     bool
	}{
		{name: "invalid", id: "bad", status: 400, detail: "播放标识无效"},
		{name: "invalid json", id: "bad", accept: "application/json", status: 400, detail: "播放标识无效", json: true},
		{name: "missing wrapped", err: fmt.Errorf("lookup: %w", sql.ErrNoRows), status: 404, detail: "重新导出"},
		{name: "missing json", accept: "application/json", err: fmt.Errorf("lookup: %w", sql.ErrNoRows), status: 404, detail: "重新导出", json: true},
		{name: "unknown errno", err: errors.New("115分享转存失败：share receive failed (errno=4100018)"), status: 502, detail: "原因未确认"},
		{name: "typed wrapped errno", err: fmt.Errorf("转存: %w", &domain.ShareReceiveError{Errno: 4100018, Message: "上游原始消息"}), status: 502, detail: "errno=4100018"},
		{name: "deleted file", err: fmt.Errorf("转存: %w", &domain.ShareReceiveError{Errno: 999999, Message: "File not found"}), status: 502, detail: "文件已删除或不存在"},
		{name: "permission json", accept: "application/json", err: fmt.Errorf("转存: %w", &domain.ShareReceiveError{Errno: 999999, Message: "权限不足"}), status: 502, detail: "转存额度和访问权限", json: true},
		{name: "retryable", err: fmt.Errorf("转存: %w", &domain.ShareReceiveError{Errno: 999999, Message: "服务器繁忙"}), status: 502, detail: "避免连续发起转存请求"},
		{name: "escaped", err: errors.New(`<script>alert("x")</script> token=test-secret https://example.test/?password=hidden`), status: 502, detail: "&lt;script&gt;"},
		{name: "json", accept: "application/json", err: errors.New("转存失败 errno=4100018"), status: 502, detail: "4100018", json: true},
		{name: "browser", accept: "text/html,application/json;q=0.5", err: errors.New("转存失败"), status: 502, detail: "播放失败"},
		{name: "json refused", accept: "application/json;q=0,*/*", err: errors.New("转存失败"), status: 502, detail: "播放失败"},
		{name: "wildcard", accept: "*/*", err: errors.New("转存失败"), status: 502, detail: "播放失败"},
	} {
		for _, method := range []string{"GET", "HEAD"} {
			t.Run(tc.name+method, func(t *testing.T) {
				id := tc.id
				if id == "" {
					id = "56c94081-7619-4824-9e6f-ea13141599e1"
				}
				c := &ShareStrmController{s: &strmActionsStub{err: tc.err}}
				r := gin.New()
				r.GET("/share-strm/:id", c.Playback)
				r.HEAD("/share-strm/:id", c.Playback)
				req := httptest.NewRequest(method, "/share-strm/"+id, nil)
				req.Header.Set("Accept", tc.accept)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Location") != "" {
					t.Errorf("status/headers: %d %v", w.Code, w.Header())
				}
				wantType := "text/html"
				if tc.json {
					wantType = "application/json"
				}
				if !strings.HasPrefix(w.Header().Get("Content-Type"), wantType) {
					t.Errorf("Content-Type=%s", w.Header().Get("Content-Type"))
				}
				if method == "HEAD" {
					if w.Body.Len() != 0 {
						t.Error("HEAD has a body")
					}
					return
				}
				body := w.Body.String()
				if !strings.Contains(body, tc.detail) {
					t.Errorf("missing %q: %s", tc.detail, body)
				}
				for _, unsafe := range []string{"<script>", "test-secret", "password=hidden"} {
					if strings.Contains(body, unsafe) {
						t.Errorf("unsafe detail: %s", body)
					}
				}
				if tc.json {
					var v map[string]any
					if json.Unmarshal(w.Body.Bytes(), &v) != nil || v["error"] == nil {
						t.Errorf("JSON contract: %s", body)
					}
				}
			})
		}
	}
}

// 仅测试可导出浏览器检查用 HTML；EASY_STRM_TEST_HTML_DIR 必须是 TMPDIR 下的现有目录。
func TestSharePlaybackErrorHTMLArtifact(t *testing.T) {
	dir := os.Getenv("EASY_STRM_TEST_HTML_DIR")
	if dir == "" {
		t.Skip("set EASY_STRM_TEST_HTML_DIR under TMPDIR to export a browser fixture")
	}
	root, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatal("EASY_STRM_TEST_HTML_DIR must be inside TMPDIR")
	}
	c := &ShareStrmController{s: &strmActionsStub{err: fmt.Errorf("转存: %w", &domain.ShareReceiveError{Errno: 4100018, Message: "服务器繁忙，请稍后重试"})}}
	r := gin.New()
	r.GET("/share-strm/:id", c.Playback)
	req := httptest.NewRequest("GET", "/share-strm/56c94081-7619-4824-9e6f-ea13141599e1", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "上游暂时不可用") {
		t.Fatalf("unexpected browser fixture: %d %s", w.Code, w.Body.String())
	}
	file, err := os.CreateTemp(dir, "share-playback-error-*.html")
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(w.Body.Bytes())
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write artifact: %v; close: %v", writeErr, closeErr)
	}
	t.Logf("browser fixture: %s", file.Name())
}
