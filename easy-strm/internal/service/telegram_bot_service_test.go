package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"easy-strm/internal/domain"
	telegram "github.com/go-telegram/bot"
	telegrammodels "github.com/go-telegram/bot/models"
)

func TestTelegramBotAuthorizationAllowsOnlyConfiguredPrivateChat(t *testing.T) {
	service := &TelegramBotService{config: TelegramConfig{ChatID: "123456"}}
	if !service.authorizedChat(123456, telegrammodels.ChatTypePrivate) {
		t.Fatal("配置的管理员私聊应被允许")
	}
	if service.authorizedChat(654321, telegrammodels.ChatTypePrivate) {
		t.Fatal("其他用户私聊不应被允许")
	}
	if service.authorizedChat(123456, telegrammodels.ChatTypeGroup) {
		t.Fatal("群组不应被允许")
	}
}

func TestTelegramBotSendsHelpCardThroughAPI(t *testing.T) {
	requests := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("解析Telegram请求失败: %v", err)
		}
		requests <- map[string]string{
			"chat_id": request.FormValue("chat_id"), "parse_mode": request.FormValue("parse_mode"),
			"reply_markup": request.FormValue("reply_markup"),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":123,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()
	instance, err := telegram.New("1:test", telegram.WithServerURL(server.URL), telegram.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	service := &TelegramBotService{config: TelegramConfig{ChatID: "123"}}
	service.sendHelp(context.Background(), instance)
	request := <-requests
	if request["chat_id"] != "123" || request["parse_mode"] != "HTML" {
		t.Fatalf("Telegram请求字段异常: %#v", request)
	}
	if request["reply_markup"] == "" {
		t.Fatalf("帮助卡片缺少内联按钮: %#v", request)
	}
}

func TestTelegramConfigValidate(t *testing.T) {
	cases := []struct {
		name   string
		config TelegramConfig
		ok     bool
	}{
		{name: "valid", config: TelegramConfig{BotToken: "1:token", ChatID: "123"}, ok: true},
		{name: "missing token", config: TelegramConfig{ChatID: "123"}},
		{name: "invalid chat", config: TelegramConfig{BotToken: "1:token", ChatID: "group"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if (err == nil) != test.ok {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestTelegramBotRoutesShareMessageToResourceService(t *testing.T) {
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("解析Telegram请求失败: %v", err)
		}
		requests <- request.FormValue("text")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":123,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()
	instance, err := telegram.New("1:test", telegram.WithServerURL(server.URL), telegram.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	share := &fakeTelegramShareTransfer{parsed: &domain.ParseShareResponse{
		ShareCode: "share-code",
		Files:     []domain.ShareFileInfo{{Fid: "fid-1", Name: "资源目录", IsDir: true}},
	}}
	resources := NewTelegramResourceService(share, &fakeTelegramOfflineDownload{}, &fakeTelegramAccountStore{accounts: []*domain.Cloud115{{
		ID: 1, Name: "资源号", AccountType: domain.AccountTypeResource, Priority: 10, Status: domain.AccountStatusActive, Cookie: "cookie",
	}}})
	service := &TelegramBotService{config: TelegramConfig{ChatID: "123"}, resources: resources}
	service.handleUpdate(context.Background(), instance, &telegrammodels.Update{Message: &telegrammodels.Message{
		Text: "https://115cdn.com/s/sharecode?password=abcd# /自动转存",
		Chat: telegrammodels.Chat{ID: 123, Type: telegrammodels.ChatTypePrivate},
	}})

	select {
	case text := <-requests:
		if text == "" || share.request.TargetDirectory != "/自动转存" {
			t.Fatalf("机器人未正确提交分享转存: text=%q request=%#v", text, share.request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待Telegram提交卡片超时")
	}
}
