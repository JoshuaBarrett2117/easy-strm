package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

func TestNotificationControllerTelegramStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewNotificationController(nil, nil, &service.TelegramBotService{})
	router.GET("/notify/telegram/status", controller.GetTelegramStatus)

	request := httptest.NewRequest(http.MethodGet, "/notify/telegram/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"running":false`) {
		t.Fatalf("状态接口响应异常: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNotificationControllerRejectsInvalidTelegramConfigBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewNotificationController(nil, nil, &service.TelegramBotService{})
	router.PUT("/notify/telegram/config", controller.UpdateTelegramConfig)

	request := httptest.NewRequest(http.MethodPut, "/notify/telegram/config", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("非法请求应返回400: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMaskTelegramToken(t *testing.T) {
	masked := maskTelegramToken(`{"bot_token":"secret","chat_id":"123"}`)
	if strings.Contains(masked, "secret") || strings.Contains(masked, "bot_token") || !strings.Contains(masked, "chat_id") {
		t.Fatalf("Telegram配置脱敏失败: %s", masked)
	}
}
