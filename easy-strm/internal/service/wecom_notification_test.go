package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWeComConfigValidateRequiresRecipient(t *testing.T) {
	config := WeComConfig{CorpID: "corp", AgentID: 1000002, Secret: "secret"}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "至少填写一项") {
		t.Fatalf("缺少接收范围应校验失败: %v", err)
	}
}

func TestWeComConfigValidateAPIBaseURL(t *testing.T) {
	config := WeComConfig{CorpID: "corp", AgentID: 1000002, Secret: "secret", ToUser: "@all", APIBaseURL: "http://wxchat.local:8080"}
	if err := config.Validate(); err != nil {
		t.Fatalf("有效消息转发代理地址不应失败: %v", err)
	}
	config.APIBaseURL = "socks5://wxchat.local:1080"
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "转发代理地址") {
		t.Fatalf("非 HTTP 消息转发地址应被拒绝: %v", err)
	}
}

func TestWeComConfigValidateCallback(t *testing.T) {
	config := WeComConfig{CorpID: "corp", CallbackToken: "token", EncodingAESKey: strings.Repeat("a", 42)}
	if err := config.ValidateCallback(); err == nil || !strings.Contains(err.Error(), "43") {
		t.Fatalf("错误长度的EncodingAESKey应被拒绝: %v", err)
	}
	config.EncodingAESKey = strings.Repeat("a", 43)
	if err := config.ValidateCallback(); err != nil {
		t.Fatalf("有效回调配置不应失败: %v", err)
	}
}

func TestNotificationServiceSendWeComCardAndReuseToken(t *testing.T) {
	var tokenRequests atomic.Int32
	var sendRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			tokenRequests.Add(1)
			if request.URL.Query().Get("corpid") != "corp-id" || request.URL.Query().Get("corpsecret") != "app-secret" {
				t.Errorf("鉴权参数异常: %s", request.URL.RawQuery)
			}
			_, _ = response.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"token-value","expires_in":7200}`))
		case "/cgi-bin/message/send":
			sendRequests.Add(1)
			if request.URL.Query().Get("access_token") != "token-value" {
				t.Errorf("access_token 异常: %s", request.URL.RawQuery)
			}
			var payload struct {
				ToUser  string `json:"touser"`
				ToParty string `json:"toparty"`
				ToTag   string `json:"totag"`
				Message string `json:"msgtype"`
				AgentID int64  `json:"agentid"`
				Text    struct {
					Content string `json:"content"`
				} `json:"text"`
				TextCard struct {
					Title       string `json:"title"`
					Description string `json:"description"`
					URL         string `json:"url"`
					Button      string `json:"btntxt"`
				} `json:"textcard"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("解析发送请求失败: %v", err)
			}
			if payload.ToUser != "alice|bob" || payload.ToParty != "2" || payload.ToTag != "3" || payload.Message != "textcard" || payload.AgentID != 1000002 {
				t.Errorf("发送字段异常: %#v", payload)
			}
			if !strings.Contains(payload.TextCard.Title, "测试任务") || !strings.Contains(payload.TextCard.Description, "STRM") || payload.TextCard.Button != "查看详情" {
				t.Errorf("卡片内容异常: %#v", payload.TextCard)
			}
			_, _ = response.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	notifications := NewNotificationService(nil, server.Client())
	notifications.weComAPIBaseURL = server.URL
	config := fmt.Sprintf(`{"corp_id":"corp-id","agent_id":1000002,"secret":"app-secret","to_user":"alice|bob","to_party":"2","to_tag":"3","detail_url":%q}`, server.URL+"/tasks")
	card := NotificationCard{Title: "测试任务", Status: "成功", Fields: [][2]string{{"类型", "STRM"}}, Detail: "发送正常"}
	for index := 0; index < 2; index++ {
		if err := notifications.sendWeComCard(config, card); err != nil {
			t.Fatalf("发送企业微信消息失败: %v", err)
		}
	}
	if tokenRequests.Load() != 1 || sendRequests.Load() != 2 {
		t.Fatalf("Token缓存未生效: token=%d send=%d", tokenRequests.Load(), sendRequests.Load())
	}
}

func TestNotificationServiceReportsWeComAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/cgi-bin/gettoken" {
			_, _ = response.Write([]byte(`{"errcode":40013,"errmsg":"invalid corpid"}`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()

	notifications := NewNotificationService(nil, server.Client())
	notifications.weComAPIBaseURL = server.URL
	err := notifications.sendWeComCard(`{"corp_id":"bad","agent_id":1,"secret":"secret","to_user":"@all"}`, NotificationCard{Title: "测试"})
	if err == nil || !strings.Contains(err.Error(), "40013") {
		t.Fatalf("应返回企业微信业务错误: %v", err)
	}
}

func TestNotificationServiceReportsWeComHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
		_, _ = response.Write([]byte("upstream unavailable"))
	}))
	defer server.Close()

	notifications := NewNotificationService(nil, server.Client())
	notifications.weComAPIBaseURL = server.URL
	err := notifications.sendWeComCard(`{"corp_id":"corp","agent_id":1,"secret":"secret","to_user":"@all"}`, NotificationCard{Title: "测试"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("应返回HTTP错误: %v", err)
	}
}

func TestBuildWeComTextRespectsByteLimit(t *testing.T) {
	content := buildWeComText(NotificationCard{Title: strings.Repeat("通知", 1500), Detail: strings.Repeat("详情", 1500)})
	if len(content) > 2000 {
		t.Fatalf("企业微信纯文本超过字节限制: %d", len(content))
	}
	if !strings.HasSuffix(content, "…") {
		t.Fatalf("截断内容应包含省略号: %q", content[len(content)-16:])
	}
}
