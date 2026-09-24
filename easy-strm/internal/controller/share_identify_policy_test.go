package controller

import (
	"bytes"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestShareIdentifyRejectsConflictingModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewShareRecordController(service.NewShareRecordService(nil, nil, nil, nil))
	r := gin.New()
	r.POST("/batch", c.Batch)
	r.POST("/record/:id", c.IdentifyRecord)
	for _, tc := range []struct{ url, body string }{{"/batch", `{"retry_failed":true,"force_refresh":true}`}, {"/batch", `{"pending_only":true,"force_refresh":true}`}, {"/record/1?force_refresh=bad", `{}`}, {"/record/1?failed_only=true&force_refresh=true", `{}`}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", tc.url, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("%s %s: %d %s", tc.url, tc.body, w.Code, w.Body)
		}
	}
}
