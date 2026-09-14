package controller

import (
	"bytes"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

type aiControllerStore struct{ value string }

func (s *aiControllerStore) GetByKey(string) (*domain.SystemConfig, error) {
	if s.value == "" {
		return nil, nil
	}
	return &domain.SystemConfig{ConfigVal: s.value}, nil
}
func (s *aiControllerStore) Upsert(_ string, v string) error { s.value = v; return nil }

func TestAIRecognitionConfigEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewAIRecognitionController(service.NewAIRecognitionService(&aiControllerStore{}, nil))
	r := gin.New()
	r.GET("/ai", c.Get)
	r.PUT("/ai", c.Save)
	r.POST("/models", c.Models)
	r.POST("/test", c.Test)
	r.POST("/assist", c.Assist)
	for _, tt := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/ai", "", 200}, {"PUT", "/ai", "{", 400}, {"PUT", "/ai", `{"enabled":true,"base_url":"https://example.com/v1"}`, 400},
		{"PUT", "/ai", `{"base_url":"https://example.com/v1","api_key":"test-secret","model":"sample","prompt":"提示词","scenes":["no_match"]}`, 200},
		{"GET", "/ai", "", 200}, {"POST", "/models", "{", 400}, {"POST", "/test", "{", 400}, {"POST", "/assist", "{", 400}, {"POST", "/assist", `{"filename":" "}`, 400},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tt.code {
			t.Fatalf("%s %s: %d %s", tt.method, tt.path, w.Code, w.Body)
		}
		if bytes.Contains(w.Body.Bytes(), []byte("test-secret")) {
			t.Fatal("配置响应泄漏密钥")
		}
		if tt.method == "GET" {
			var payload map[string]any
			if json.Unmarshal(w.Body.Bytes(), &payload) != nil {
				t.Fatal(w.Body)
			}
		}
	}
}
