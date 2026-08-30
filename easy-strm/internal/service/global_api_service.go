package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"easy-strm/internal/dao"
)

const (
	GlobalAPIEnabledKey = "global_api_enabled"
	GlobalAPIBaseURLKey = "global_api_base_url"
	GlobalAPIKeyKey     = "global_api_key"
)

// GlobalAPIConfigView 是全局外部 API 的脱敏配置视图。
type GlobalAPIConfigView struct {
	Enabled   bool   `json:"enabled"`
	BaseURL   string `json:"base_url"`
	HasAPIKey bool   `json:"has_api_key"`
}

// GlobalAPIConfigUpdate 描述全局 API 配置更新请求。
type GlobalAPIConfigUpdate struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// GlobalAPIService 管理系统级 API Key，并提供请求认证能力。
type GlobalAPIService struct{ configs *dao.SystemConfigDAO }

func NewGlobalAPIService(configs *dao.SystemConfigDAO) *GlobalAPIService {
	return &GlobalAPIService{configs: configs}
}

func (s *GlobalAPIService) GetConfig() (GlobalAPIConfigView, error) {
	enabled, base, key, err := s.values()
	if err != nil {
		return GlobalAPIConfigView{}, err
	}
	return GlobalAPIConfigView{Enabled: enabled, BaseURL: base, HasAPIKey: key != ""}, nil
}

func (s *GlobalAPIService) Update(req GlobalAPIConfigUpdate) (GlobalAPIConfigView, error) {
	base := strings.TrimSpace(req.BaseURL)
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return GlobalAPIConfigView{}, fmt.Errorf("外部 API 地址无效")
		}
	}
	_, _, oldKey, err := s.values()
	if err != nil {
		return GlobalAPIConfigView{}, err
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		key = oldKey
	}
	if req.Enabled && key == "" {
		key, err = generateAPIKey()
		if err != nil {
			return GlobalAPIConfigView{}, err
		}
	}
	if err = s.configs.BatchUpsert(map[string]string{GlobalAPIEnabledKey: fmt.Sprintf("%t", req.Enabled), GlobalAPIBaseURLKey: base, GlobalAPIKeyKey: key}); err != nil {
		return GlobalAPIConfigView{}, err
	}
	return s.GetConfig()
}

// Regenerate 生成新密钥，仅在本次响应中返回明文。
func (s *GlobalAPIService) Regenerate() (GlobalAPIConfigView, string, error) {
	e, b, _, err := s.values()
	if err != nil {
		return GlobalAPIConfigView{}, "", err
	}
	k, err := generateAPIKey()
	if err != nil {
		return GlobalAPIConfigView{}, "", err
	}
	if err = s.configs.Upsert(GlobalAPIKeyKey, k); err != nil {
		return GlobalAPIConfigView{}, "", err
	}
	if !e {
		e = true
		_ = s.configs.Upsert(GlobalAPIEnabledKey, "true")
	}
	return GlobalAPIConfigView{Enabled: e, BaseURL: b, HasAPIKey: true}, k, nil
}

func (s *GlobalAPIService) ValidateAPIKey(candidate string) (bool, error) {
	enabled, _, key, err := s.values()
	if err != nil {
		return false, err
	}
	if !enabled || key == "" || candidate == "" {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(key), []byte(candidate)) == 1, nil
}

func (s *GlobalAPIService) values() (bool, string, string, error) {
	get := func(k string) (string, error) {
		c, err := s.configs.GetByKey(k)
		if err != nil {
			return "", err
		}
		if c == nil {
			return "", nil
		}
		return c.ConfigVal, nil
	}
	e, err := get(GlobalAPIEnabledKey)
	if err != nil {
		return false, "", "", err
	}
	b, err := get(GlobalAPIBaseURLKey)
	if err != nil {
		return false, "", "", err
	}
	k, err := get(GlobalAPIKeyKey)
	if err != nil {
		return false, "", "", err
	}
	return strings.EqualFold(strings.TrimSpace(e), "true"), strings.TrimRight(strings.TrimSpace(b), "/"), strings.TrimSpace(k), nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "esk_" + base64.RawURLEncoding.EncodeToString(b), nil
}
