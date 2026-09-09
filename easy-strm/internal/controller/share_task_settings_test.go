package controller

import (
	"bytes"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestShareTaskSettingsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := service.NewShareRecordService(nil, nil, nil, nil)
	s.SetTaskSettingsStore(&aiControllerStore{})
	c := NewShareRecordController(s)
	r := gin.New()
	r.GET("/settings", c.GetTaskSettings)
	r.PUT("/settings", c.SaveTaskSettings)
	for _, tt := range []struct {
		method, body string
		code         int
	}{
		{"GET", "", 200}, {"PUT", "{}", 400}, {"PUT", `{"timeout_minutes":null}`, 400}, {"PUT", `{"timeout_minutes":-1}`, 400},
		{"PUT", `{"timeout_minutes":0}`, 200}, {"PUT", `{"timeout_minutes":60}`, 200}, {"PUT", `{"timeout_minutes":0.5}`, 400},
	} {
		req := httptest.NewRequest(tt.method, "/settings", bytes.NewBufferString(tt.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tt.code {
			t.Fatalf("%s: %d %s", tt.body, w.Code, w.Body)
		}
	}
}
