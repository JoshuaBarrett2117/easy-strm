package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultWeComAPIBaseURL = "https://qyapi.weixin.qq.com"

// WeComConfig 定义企业微信自建应用通知配置。
type WeComConfig struct {
	CorpID              string `json:"corp_id"`
	AgentID             int64  `json:"agent_id"`
	Secret              string `json:"secret"`
	ReceiveEnabled      bool   `json:"receive_enabled"`
	CallbackToken       string `json:"callback_token"`
	EncodingAESKey      string `json:"encoding_aes_key"`
	ToUser              string `json:"to_user"`
	ToParty             string `json:"to_party"`
	ToTag               string `json:"to_tag"`
	NotifyTaskCompleted bool   `json:"notify_task_completed"`
	NotifyTaskFailed    bool   `json:"notify_task_failed"`
	NotifyTaskCancelled bool   `json:"notify_task_cancelled"`
	NotifyAccountStatus bool   `json:"notify_account_status"`
}

// ParseWeComConfig 解析企业微信配置，并为历史配置补齐事件默认值。
func ParseWeComConfig(configJSON string) (WeComConfig, error) {
	if strings.TrimSpace(configJSON) == "" {
		configJSON = "{}"
	}
	var config WeComConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return config, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &fields); err != nil {
		return config, err
	}
	if _, exists := fields["notify_task_completed"]; !exists {
		config.NotifyTaskCompleted = true
	}
	if _, exists := fields["notify_task_failed"]; !exists {
		config.NotifyTaskFailed = true
	}
	if _, exists := fields["notify_task_cancelled"]; !exists {
		config.NotifyTaskCancelled = true
	}
	if _, exists := fields["notify_account_status"]; !exists {
		config.NotifyAccountStatus = true
	}
	return config, nil
}

// Validate 校验企业微信应用和接收范围。
func (c WeComConfig) Validate() error {
	if strings.TrimSpace(c.CorpID) == "" {
		return fmt.Errorf("corp_id 不能为空")
	}
	if c.AgentID <= 0 {
		return fmt.Errorf("agent_id 必须是正整数")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return fmt.Errorf("secret 不能为空")
	}
	if strings.TrimSpace(c.ToUser) == "" && strings.TrimSpace(c.ToParty) == "" && strings.TrimSpace(c.ToTag) == "" {
		return fmt.Errorf("成员、部门或标签接收范围至少填写一项")
	}
	return nil
}

// ValidateCallback 校验企业微信 API 接收消息所需配置。
func (c WeComConfig) ValidateCallback() error {
	if strings.TrimSpace(c.CorpID) == "" {
		return fmt.Errorf("corp_id 不能为空")
	}
	if strings.TrimSpace(c.CallbackToken) == "" {
		return fmt.Errorf("回调 Token 不能为空")
	}
	if len(strings.TrimSpace(c.EncodingAESKey)) != 43 {
		return fmt.Errorf("EncodingAESKey 必须是 43 位")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c.EncodingAESKey) + "=")
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("EncodingAESKey 格式无效")
	}
	return nil
}

type weComTokenEntry struct {
	token     string
	expiresAt time.Time
}

type weComTokenCache struct {
	mu      sync.Mutex
	entries map[string]weComTokenEntry
}

func newWeComTokenCache() *weComTokenCache {
	return &weComTokenCache{entries: make(map[string]weComTokenEntry)}
}

type weComAPIResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type weComTokenResponse struct {
	weComAPIResponse
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *NotificationService) sendWeComCard(configJSON string, card NotificationCard) error {
	config, err := ParseWeComConfig(configJSON)
	if err != nil {
		return fmt.Errorf("企业微信配置解析失败: %v", err)
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("企业微信配置不完整: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.sendWeComCardWithContext(ctx, config, card)
}

func (s *NotificationService) sendWeComCardWithContext(ctx context.Context, config WeComConfig, card NotificationCard) error {
	token, err := s.getWeComAccessToken(ctx, config)
	if err != nil {
		return err
	}
	payload := map[string]interface{}{
		"touser":  strings.TrimSpace(config.ToUser),
		"toparty": strings.TrimSpace(config.ToParty),
		"totag":   strings.TrimSpace(config.ToTag),
		"msgtype": "markdown",
		"agentid": config.AgentID,
		"markdown": map[string]string{
			"content": buildWeComMarkdown(card),
		},
		"enable_duplicate_check":   1,
		"duplicate_check_interval": 1800,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("企业微信消息序列化失败: %v", err)
	}
	endpoint := strings.TrimRight(s.weComAPIBaseURL, "/") + "/cgi-bin/message/send?access_token=" + url.QueryEscape(token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建企业微信发送请求失败: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	var response weComAPIResponse
	if err := s.doWeComJSON(request, &response); err != nil {
		return fmt.Errorf("企业微信发送请求失败: %v", err)
	}
	if response.ErrCode != 0 {
		return fmt.Errorf("企业微信发送失败(%d): %s", response.ErrCode, response.ErrMsg)
	}
	return nil
}

// SendWeComCardToUser 向触发回调的企业微信成员发送应用消息。
func (s *NotificationService) SendWeComCardToUser(config WeComConfig, userID string, card NotificationCard) error {
	config.ToUser = strings.TrimSpace(userID)
	config.ToParty = ""
	config.ToTag = ""
	if config.ToUser == "" {
		return fmt.Errorf("企业微信回复成员不能为空")
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("企业微信回复配置不完整: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.sendWeComCardWithContext(ctx, config, card)
}

func (s *NotificationService) getWeComAccessToken(ctx context.Context, config WeComConfig) (string, error) {
	key := config.CorpID + "\x00" + config.Secret
	s.weComTokens.mu.Lock()
	defer s.weComTokens.mu.Unlock()
	if cached, exists := s.weComTokens.entries[key]; exists && time.Now().Before(cached.expiresAt) {
		return cached.token, nil
	}
	query := url.Values{"corpid": {config.CorpID}, "corpsecret": {config.Secret}}
	endpoint := strings.TrimRight(s.weComAPIBaseURL, "/") + "/cgi-bin/gettoken?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("创建企业微信鉴权请求失败: %v", err)
	}
	var response weComTokenResponse
	if err := s.doWeComJSON(request, &response); err != nil {
		return "", fmt.Errorf("企业微信鉴权请求失败: %v", err)
	}
	if response.ErrCode != 0 {
		return "", fmt.Errorf("企业微信鉴权失败(%d): %s", response.ErrCode, response.ErrMsg)
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", fmt.Errorf("企业微信鉴权响应缺少 access_token")
	}
	expires := time.Duration(response.ExpiresIn) * time.Second
	if expires <= time.Minute {
		expires = 2 * time.Hour
	}
	s.weComTokens.entries[key] = weComTokenEntry{token: response.AccessToken, expiresAt: time.Now().Add(expires - time.Minute)}
	return response.AccessToken, nil
}

func (s *NotificationService) doWeComJSON(request *http.Request, target interface{}) error {
	response, err := s.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, truncateRunes(string(body), 300))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("响应解析失败: %v", err)
	}
	return nil
}

func buildWeComMarkdown(card NotificationCard) string {
	var builder strings.Builder
	builder.WriteString("## ")
	if card.Status != "" {
		builder.WriteString(truncateRunes(card.Status, 16))
		builder.WriteString(" ")
	}
	builder.WriteString(truncateRunes(card.Title, 128))
	for _, field := range card.Fields {
		builder.WriteString("\n>")
		builder.WriteString(truncateRunes(field[0], 64))
		builder.WriteString("：")
		builder.WriteString(truncateRunes(field[1], 256))
	}
	if card.Detail != "" {
		builder.WriteString("\n>")
		builder.WriteString(truncateRunes(card.Detail, 900))
	}
	// 企业微信应用 markdown 内容上限为 2048 字节，预留少量空间避免多字节字符越界。
	return truncateUTF8Bytes(builder.String(), 2000)
}

func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= len("…") {
		return ""
	}
	characters := []rune(value)
	low, high := 0, len(characters)
	for low < high {
		mid := (low + high + 1) / 2
		if len(string(characters[:mid]))+len("…") <= limit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return string(characters[:low]) + "…"
}

func parseWeComAgentID(value string) (int64, error) {
	agentID, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || agentID <= 0 {
		return 0, fmt.Errorf("agent_id 必须是正整数")
	}
	return agentID, nil
}
