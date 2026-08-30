package service

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/sbzhu/weworkapi_golang/wxbizmsgcrypt"
)

const testWeComEncodingAESKey = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"

type fakeWeComCallbackConfigReader struct {
	config WeComConfig
	view   WeComConfigView
}

func (f *fakeWeComCallbackConfigReader) GetWeComConfig() (WeComConfig, WeComConfigView, error) {
	return f.config, f.view, nil
}

type fakeWeComReplySender struct {
	messages chan NotificationCard
	users    chan string
}

func (f *fakeWeComReplySender) SendWeComCardToUser(_ WeComConfig, userID string, card NotificationCard) error {
	f.users <- userID
	f.messages <- card
	return nil
}

func TestWeComCallbackServiceVerifyURL(t *testing.T) {
	config := testWeComCallbackConfig()
	callback := NewWeComCallbackService(&fakeWeComCallbackConfigReader{config: config}, &fakeWeComReplySender{}, nil, nil, nil)
	timestamp, nonce := "1725000000", "nonce-value"
	encrypted := encryptWeComTestMessage(t, config, "echo-ok", timestamp, nonce)

	echo, err := callback.VerifyURL(encrypted.Signature.Value, timestamp, nonce, encrypted.Encrypt.Value)
	if err != nil {
		t.Fatalf("回调URL校验失败: %v", err)
	}
	if echo != "echo-ok" {
		t.Fatalf("回调URL解密结果异常: %q", echo)
	}
	if _, err := callback.VerifyURL("invalid", timestamp, nonce, encrypted.Encrypt.Value); err == nil {
		t.Fatal("错误签名应被拒绝")
	}
}

func TestWeComCallbackServiceReceivesTextAndDeduplicates(t *testing.T) {
	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	config := testWeComCallbackConfig()
	replies := &fakeWeComReplySender{messages: make(chan NotificationCard, 2), users: make(chan string, 2)}
	callback := NewWeComCallbackService(&fakeWeComCallbackConfigReader{config: config}, replies, nil, nil, redisClient)
	timestamp, nonce := "1725000001", "nonce-message"
	plain := `<xml><ToUserName><![CDATA[ww-test-corp]]></ToUserName><FromUserName><![CDATA[alice]]></FromUserName><CreateTime>1725000001</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[帮助]]></Content><MsgId>123456</MsgId><AgentID>1000002</AgentID></xml>`
	encrypted := encryptWeComTestMessage(t, config, plain, timestamp, nonce)
	body, err := xml.Marshal(encrypted)
	if err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 2; index++ {
		if err := callback.Receive(encrypted.Signature.Value, timestamp, nonce, body); err != nil {
			t.Fatalf("接收企业微信消息失败: %v", err)
		}
	}
	select {
	case userID := <-replies.users:
		if userID != "alice" {
			t.Fatalf("回复成员异常: %s", userID)
		}
	case <-time.After(time.Second):
		t.Fatal("未收到异步回复")
	}
	select {
	case card := <-replies.messages:
		if !strings.Contains(card.Title, "企业微信助手") {
			t.Fatalf("帮助卡片异常: %#v", card)
		}
	case <-time.After(time.Second):
		t.Fatal("未收到帮助卡片")
	}
	select {
	case <-replies.messages:
		t.Fatal("重复消息不应再次回复")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWeComCallbackServiceRejectsMismatchedAgent(t *testing.T) {
	config := testWeComCallbackConfig()
	callback := NewWeComCallbackService(&fakeWeComCallbackConfigReader{config: config}, &fakeWeComReplySender{}, nil, nil, nil)
	timestamp, nonce := "1725000002", "nonce-agent"
	plain := `<xml><FromUserName><![CDATA[alice]]></FromUserName><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[帮助]]></Content><MsgId>789</MsgId><AgentID>9999999</AgentID></xml>`
	encrypted := encryptWeComTestMessage(t, config, plain, timestamp, nonce)
	body, err := xml.Marshal(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback.Receive(encrypted.Signature.Value, timestamp, nonce, body); err == nil || !strings.Contains(err.Error(), "AgentID") {
		t.Fatalf("不匹配的AgentID应被拒绝: %v", err)
	}
}

func testWeComCallbackConfig() WeComConfig {
	return WeComConfig{
		CorpID: "ww-test-corp", AgentID: 1000002, Secret: "secret", ToUser: "@all",
		ReceiveEnabled: true, CallbackToken: "callback-token", EncodingAESKey: testWeComEncodingAESKey,
	}
}

func encryptWeComTestMessage(t *testing.T, config WeComConfig, plain, timestamp, nonce string) *wxbizmsgcrypt.WXBizMsg4Send {
	t.Helper()
	crypt := newWeComMessageCrypt(config)
	body, cryptErr := crypt.EncryptMsg(plain, timestamp, nonce)
	if cryptErr != nil {
		t.Fatalf("构造企业微信加密消息失败: %v", cryptErr)
	}
	var encrypted wxbizmsgcrypt.WXBizMsg4Send
	if err := xml.Unmarshal(body, &encrypted); err != nil {
		t.Fatalf("解析企业微信加密消息失败: %v", err)
	}
	return &encrypted
}
