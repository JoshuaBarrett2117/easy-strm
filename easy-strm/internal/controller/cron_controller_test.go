package controller

import (
	"easy-strm/internal/service"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCronControllerUsesSharedScheduler 确保处理器目录来自运行中的调度实例。
func TestCronControllerUsesSharedScheduler(t *testing.T) {
	c := NewCronController(service.NewCronService(nil), nil, nil)
	shared := service.NewCronService(nil)
	shared.Register(service.CronHandler{Key: "full_generate", Name: "STRM全量生成"})
	c.SetScheduler(shared)
	r := gin.New()
	r.GET("/cron/handlers", c.Handlers)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/cron/handlers", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "full_generate") {
		t.Fatalf("共享调度器的处理器未返回: %d %s", w.Code, w.Body.String())
	}
	if c.cronService != shared {
		t.Fatal("管理操作必须使用共享调度器，避免创建独立调度状态")
	}
}
