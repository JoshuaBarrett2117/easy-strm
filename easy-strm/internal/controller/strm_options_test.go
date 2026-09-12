package controller

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFullGenerateRejectsInvalidClearOption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := &StrmController{}
	r.POST("/generate", c.GenerateFull)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/generate", strings.NewReader(`{"clear_before_generate":"yes"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("无效参数返回 %d", w.Code)
	}
}
