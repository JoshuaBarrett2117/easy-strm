package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"
)

func TestVerifiedCandidateRejectsMissingYearAndDistinctProviders(t *testing.T) {
	q := ShareMediaQuery{Titles: []string{"Example"}, Year: 2020, MediaType: "movie"}
	if got := selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{{Title: "Example", MediaType: "movie", TmdbID: 1}}); got != nil {
		t.Fatal("缺失年份的候选不能通过明确年份核验")
	}
	q.Year = 0
	candidates := []domain.TmdbSearchResult{
		{Title: "Example", MediaType: "movie", MetadataSource: "metatube", MetadataID: "1", MetadataProvider: "a"},
		{Title: "Example", MediaType: "movie", MetadataSource: "metatube", MetadataID: "1", MetadataProvider: "b"},
	}
	if got := selectVerifiedShareCandidate(q, candidates); got != nil {
		t.Fatal("不同 provider 的同名候选不能当作唯一身份")
	}
}

func TestIdentifyWithAssistRejectsUnrelatedFirstCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"id": 12, "title": "Unrelated", "release_date": "2020-01-01"}}})
	}))
	defer server.Close()
	svc := NewTmdbService("key", nil)
	svc.baseURL, svc.httpClient = server.URL, server.Client()
	result, err := svc.IdentifyWithAssist(context.Background(), "Example.2020.mkv", IdentifyAssistOptions{MediaType: "movie", MetadataSource: "tmdb"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("不相关的搜索首项不应被接受")
	}
}

func TestIdentifyWithAssistShareModeParsesOrdinaryISO(t *testing.T) {
	const dexterID = 1405
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/tv" {
			t.Fatalf("share recognition should use TV search, got %s", r.URL.Path)
		}
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]any{"id": dexterID, "name": "Dexter", "original_name": "Dexter", "first_air_date": "2006-10-01"},
		}})
	}))
	defer server.Close()
	svc := NewTmdbService("key", nil)
	svc.baseURL, svc.httpClient = server.URL, server.Client()
	result, err := svc.IdentifyWithAssist(context.Background(), "Dexter S06 Disc02.iso", IdentifyAssistOptions{MetadataSource: "tmdb", ShareMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if !called || !result.Success || result.MediaType != "tv" || result.TmdbID != dexterID {
		t.Fatalf("unexpected share result: called=%v result=%+v", called, result)
	}
}

func TestIdentifyWithAssistRequeriesAndVerifiesAIHintOnce(t *testing.T) {
	aiCalls := 0
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"title":"绿皮书","original_title":"Green Book","year":2018,"media_type":"movie"}`}}}})
	}))
	defer aiServer.Close()
	ai := NewAIRecognitionService(&aiMemoryStore{}, aiServer.Client())
	cfg := defaultAIConfig()
	cfg.BaseURL, cfg.Model, cfg.Enabled = aiServer.URL, "model", true
	if err := ai.Save(cfg); err != nil {
		t.Fatal(err)
	}

	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := []any{}
		if r.URL.Path == "/search/movie" && r.URL.Query().Get("query") == "绿皮书" {
			results = append(results, map[string]any{"id": 490132, "title": "绿皮书", "original_title": "Green Book", "release_date": "2018-11-16"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer tmdbServer.Close()
	svc := NewTmdbService("key", nil)
	svc.baseURL, svc.httpClient = tmdbServer.URL, tmdbServer.Client()
	svc.SetAIRecognitionService(ai)

	result, err := svc.IdentifyWithAssist(context.Background(), "难以解析的名字.mkv", IdentifyAssistOptions{MediaType: "movie", MetadataSource: "tmdb", AllowAI: true, UseCache: false})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.TmdbID != 490132 || result.RecognitionMethod != "ai" || !result.AIUsed || result.AIHint == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if aiCalls != 1 {
		t.Fatalf("AI should be called once, got %d", aiCalls)
	}
}

func TestIdentifyWithAssistDoesNotCallAIOnSourceError(t *testing.T) {
	aiCalls := 0
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { aiCalls++ }))
	defer aiServer.Close()
	ai := NewAIRecognitionService(&aiMemoryStore{}, aiServer.Client())
	cfg := defaultAIConfig()
	cfg.BaseURL, cfg.Model, cfg.Enabled = aiServer.URL, "model", true
	if err := ai.Save(cfg); err != nil {
		t.Fatal(err)
	}

	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusBadGateway) }))
	defer tmdbServer.Close()
	svc := NewTmdbService("key", nil)
	svc.baseURL, svc.httpClient = tmdbServer.URL, tmdbServer.Client()
	svc.SetAIRecognitionService(ai)
	result, err := svc.IdentifyWithAssist(context.Background(), "测试电影.mkv", IdentifyAssistOptions{MediaType: "movie", AllowAI: true, UseCache: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || aiCalls != 0 || result.AIUsed {
		t.Fatalf("unexpected result=%+v calls=%d", result, aiCalls)
	}
}
