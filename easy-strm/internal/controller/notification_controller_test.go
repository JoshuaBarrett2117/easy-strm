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

func TestNotificationControllerRejectsInvalidWeComConfigBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewNotificationController(nil, nil, &service.TelegramBotService{})
	router.PUT("/notify/wecom/config", controller.UpdateWeComConfig)

	request := httptest.NewRequest(http.MethodPut, "/notify/wecom/config", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("非法请求应返回400: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMaskWeComSecret(t *testing.T) {
	masked := maskWeComSecret(`{"corp_id":"corp","agent_id":1,"secret":"private","callback_token":"callback-private","encoding_aes_key":"aes-private","to_user":"@all"}`)
	if strings.Contains(masked, "private") || strings.Contains(masked, "secret") || strings.Contains(masked, "callback_token") || strings.Contains(masked, "encoding_aes_key") || !strings.Contains(masked, "corp_id") {
		t.Fatalf("企业微信配置脱敏失败: %s", masked)
	}
}

func TestNotificationControllerWeComCallbackUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewNotificationController(nil, nil, &service.TelegramBotService{})
	router.GET("/notify/wecom/callback", controller.VerifyWeComCallback)
	router.POST("/notify/wecom/callback", controller.ReceiveWeComCallback)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/notify/wecom/callback", strings.NewReader("<xml/>"))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("未初始化回调服务应返回503: method=%s code=%d", method, response.Code)
		}
	}
}

type fakeWeComCallbackHandler struct {
	verified bool
	received bool
}

func (f *fakeWeComCallbackHandler) VerifyURL(signature, timestamp, nonce, echo string) (string, error) {
	f.verified = signature == "signature" && timestamp == "1" && nonce == "2" && echo == "encrypted-echo"
	return "plain-echo", nil
}

func (f *fakeWeComCallbackHandler) Receive(signature, timestamp, nonce string, body []byte) error {
	f.received = signature == "signature" && timestamp == "1" && nonce == "2" && string(body) == "<xml/>"
	return nil
}

func TestNotificationControllerHandlesWeComCallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	callback := &fakeWeComCallbackHandler{}
	controller := NewNotificationController(nil, nil, &service.TelegramBotService{})
	controller.SetWeComCallbackService(callback)
	router.GET("/notify/wecom/callback", controller.VerifyWeComCallback)
	router.POST("/notify/wecom/callback", controller.ReceiveWeComCallback)

	query := "?msg_signature=signature&timestamp=1&nonce=2"
	getRequest := httptest.NewRequest(http.MethodGet, "/notify/wecom/callback"+query+"&echostr=encrypted-echo", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || getResponse.Body.String() != "plain-echo" || !callback.verified {
		t.Fatalf("企业微信URL校验响应异常: code=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	postRequest := httptest.NewRequest(http.MethodPost, "/notify/wecom/callback"+query, strings.NewReader("<xml/>"))
	postResponse := httptest.NewRecorder()
	router.ServeHTTP(postResponse, postRequest)
	if postResponse.Code != http.StatusOK || postResponse.Body.String() != "success" || !callback.received {
		t.Fatalf("企业微信消息接收响应异常: code=%d body=%s", postResponse.Code, postResponse.Body.String())
	}
}
