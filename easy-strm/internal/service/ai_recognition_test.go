package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type aiMemoryStore struct {
	value string
	fail  bool
}

func (s *aiMemoryStore) GetByKey(string) (*domain.SystemConfig, error) {
	if s.value == "" {
		return nil, nil
	}
	return &domain.SystemConfig{ConfigVal: s.value}, nil
}
func (s *aiMemoryStore) Upsert(_ string, value string) error {
	if s.fail {
		return fmt.Errorf("db error")
	}
	s.value = value
	return nil
}
func TestAIConfigAndProtocol(t *testing.T) {
	store := &aiMemoryStore{}
	calls := 0
	auth := ""
	bad := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "b"}, {"id": "a"}, {"id": "a"}}})
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		calls++
		var body struct {
			Model    string
			Messages []map[string]string
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "model-a" || len(body.Messages) != 2 || !strings.Contains(body.Messages[0]["content"], "自定义提示") {
			t.Errorf("%+v", body)
		}
		content := "{\"title\":\"七龙珠\",\"original_title\":\"Dragon Ball\",\"year\":1986,\"media_type\":\"tv\"}"
		if bad {
			content = "不是JSON"
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer server.Close()
	s := NewAIRecognitionService(store, server.Client())
	cfg := defaultAIConfig()
	cfg.BaseURL = server.URL + "/v1"
	cfg.APIKey = "secret"
	cfg.Model = "model-a"
	cfg.Prompt = "自定义提示"
	cfg.Enabled = true
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	public, _ := s.PublicConfig()
	if public.APIKey != "" || !public.HasAPIKey {
		t.Fatal(public)
	}
	models, err := s.Models(context.Background(), public)
	if err != nil || strings.Join(models, ",") != "a,b" || auth != "Bearer secret" {
		t.Fatalf("%v %v", models, err)
	}
	if _, called, err := s.Assist(context.Background(), "input", "complex_title"); called || err != nil {
		t.Fatal(called, err)
	}
	hint, called, err := s.Assist(context.Background(), "input", "no_match")
	if err != nil || !called || hint.MediaType != "tv" || calls != 1 {
		t.Fatalf("%+v %v %v", hint, called, err)
	}
	bad = true
	if _, err = s.Test(context.Background(), public, "file"); err == nil {
		t.Fatal("应拒绝无效JSON")
	}
	public.Enabled = false
	if err = s.Save(public); err != nil {
		t.Fatal(err)
	}
	if _, called, _ = s.Assist(context.Background(), "file", "no_match"); called {
		t.Fatal("关闭时不调用")
	}
	stored, _ := s.config()
	if stored.APIKey != "secret" {
		t.Fatal("空值不应擦除密钥")
	}
	public.BaseURL = server.URL + "/other"
	if err = s.Save(public); err != nil {
		t.Fatal(err)
	}
	stored, _ = s.config()
	if stored.APIKey != "" {
		t.Fatal("不能向新端点转移旧密钥")
	}
	public.BaseURL = server.URL + "/v1"
	public.APIKey = "changed"
	store.fail = true
	if err = s.Save(public); err == nil {
		t.Fatal("数据库失败不应成功")
	}
}

func TestAIProtocolFailures(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte("secret error body"))
		}))
		s := NewAIRecognitionService(&aiMemoryStore{}, server.Client())
		cfg := defaultAIConfig()
		cfg.BaseURL = server.URL
		cfg.Model = "a"
		_, err := s.Models(context.Background(), cfg)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal(err)
		}
		server.Close()
	}
	cfg := defaultAIConfig()
	cfg.Enabled = true
	if _, err := normalizeAIConfig(cfg); err == nil {
		t.Fatal("启用需要模型")
	}
	cfg.Enabled = false
	cfg.Scenes = []string{"unknown"}
	if _, err := normalizeAIConfig(cfg); err == nil {
		t.Fatal("不支持的场景")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	cfg = defaultAIConfig()
	cfg.BaseURL = server.URL
	s := NewAIRecognitionService(&aiMemoryStore{}, server.Client())
	if _, err := s.Models(ctx, cfg); err == nil {
		t.Fatal("应响应取消")
	}
}

func TestAIIntegrationNoMatch(t *testing.T) {
	aiCalls := 0
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "{\"title\":\"绿皮书\",\"original_title\":\"Green Book\",\"year\":2018,\"media_type\":\"movie\"}"}}}})
	}))
	defer aiServer.Close()
	ai := NewAIRecognitionService(&aiMemoryStore{}, aiServer.Client())
	cfg := defaultAIConfig()
	cfg.BaseURL = aiServer.URL
	cfg.Model = "model"
	cfg.Enabled = true
	if err := ai.Save(cfg); err != nil {
		t.Fatal(err)
	}
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := []any{}
		if r.URL.Query().Get("query") == "绿皮书" && r.URL.Path == "/search/movie" {
			results = append(results, map[string]any{"id": 490132, "title": "绿皮书", "original_title": "Green Book", "release_date": "2018-11-16"})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer tmdb.Close()
	s := NewTmdbService("test", nil)
	s.baseURL = tmdb.URL
	s.httpClient = tmdb.Client()
	s.SetAIRecognitionService(ai)
	result, err := s.IdentifyShareFile(context.Background(), "难以解析的名字.iso", "tmdb", "auto")
	if err != nil || !result.Success || result.TmdbID != 490132 || aiCalls != 1 {
		t.Fatalf("%+v %v calls=%d", result, err, aiCalls)
	}
}
