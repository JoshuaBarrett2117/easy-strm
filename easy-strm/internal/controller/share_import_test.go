package controller

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestShareImportPreview 验证解析入口不依赖数据库，覆盖请求错误与真实解析响应。
func TestShareImportPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := NewShareRecordController(service.NewShareRecordService(nil, nil, nil, nil))
	r.POST("/media/share-records/parse", c.ParseImport)
	for _, tt := range []struct {
		body   string
		status int
	}{
		{"{", 400}, {`{"text":4}`, 400}, {`{}`, 400}, {`{"text":"   "}`, 400}, {`{"text":"无链接"}`, 400},
		{`{"text":"电影 https://share.115.com/sample?password=0000"}`, 200},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/media/share-records/parse", strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.body, w.Code, w.Body)
		}
		if tt.status == 200 {
			var body struct {
				Data domain.ShareImportPreview `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Data.Records) != 1 || body.Data.Records[0].Name != "电影" || body.Data.Records[0].Password != "0000" {
				t.Fatalf("%s", w.Body)
			}
		}
	}
}
