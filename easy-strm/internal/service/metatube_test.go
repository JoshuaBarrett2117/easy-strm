package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

func TestMetaTubeSearchMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/movies/search" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("q") != "流浪地球" || r.URL.Query().Get("fallback") != "true" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer local-token" {
			t.Fatalf("missing token")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"ABC-123","provider":"local","number":"ABC-123","title":"流浪地球","release_date":"2019-02-05","big_cover_url":"/images/cover.jpg","score":7.0}]}`))
	}))
	defer server.Close()

	svc := NewTmdbService("", nil)
	svc.SetMetaTubeConfig(server.URL, "local-token")
	svc.SetAdultContentEnabled(true)
	results, err := svc.SearchMovieBySource("流浪地球", 2019, domain.MetadataSourceMetaTube)
	if err != nil {
		t.Fatalf("SearchMovie failed: %v", err)
	}
	if len(results) != 1 || results[0].MetadataID != "ABC-123" || results[0].Year != 2019 || results[0].PosterPath != server.URL+"/images/cover.jpg" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestMetaTubeMetadataGeneratesMovieNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"ABC-123","provider":"local","number":"ABC-123","title":"测试影片","summary":"简介","release_date":"2026-01-02","genres":["剧情"],"actors":["演员甲"],"director":"导演甲"}}`))
	}))
	defer server.Close()
	tmdb := NewTmdbService("", nil)
	tmdb.SetMetaTubeConfig(server.URL, "")
	scraper := &ScrapeService{tmdbService: tmdb}
	raw := json.RawMessage(`{"tmdb_id":1,"title":"测试影片","media_type":"movie","metadata_source":"metatube","metadata_id":"ABC-123","metadata_provider":"local"}`)
	content, _, err := scraper.GenerateMovieNFO(raw, t.TempDir(), "ABC-123.mp4", scrapeOutputOptions{})
	if err != nil {
		t.Fatalf("GenerateMovieNFO failed: %v", err)
	}
	if !strings.Contains(content, `<uniqueid type="metatube" default="true">ABC-123</uniqueid>`) || !strings.Contains(content, "演员甲") {
		t.Fatalf("unexpected NFO: %s", content)
	}
}

func TestMetaTubeMovieDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/movies/local/ABC-123" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"ABC-123","provider":"local","number":"ABC-123","title":"答案"}}`))
	}))
	defer server.Close()

	svc := NewTmdbService("", nil)
	svc.SetMetaTubeConfig(server.URL, "")
	id := metaTubeSyntheticID("local", "ABC-123")
	svc.metatubeRefs.Store(id, metaTubeRef{Provider: "local", ID: "ABC-123"})
	detail, err := svc.GetMovieDetail(id)
	if err != nil {
		t.Fatalf("GetMovieDetail failed: %v", err)
	}
	if detail["title"] != "答案" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestSearchMovieBySourceForcesTMDB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/movie" {
			t.Fatalf("expected TMDB endpoint, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"TMDB 影片","release_date":"2024-01-01"}]}`))
	}))
	defer server.Close()
	svc := NewTmdbService("key", nil)
	svc.baseURL = server.URL
	svc.SetMetaTubeConfig("http://metatube.local", "")
	svc.SetMetaTubeDefaultEnabled(true)
	results, err := svc.SearchMovieBySource("测试", 0, domain.MetadataSourceTMDB)
	if err != nil {
		t.Fatalf("forced TMDB search failed: %v", err)
	}
	if len(results) != 1 || results[0].MetadataSource != domain.MetadataSourceTMDB {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSearchMovieBySourceAutoPrefersTMDB(t *testing.T) {
	metaTubeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"results":[{"id":42,"title":"TMDB 影片","release_date":"2024-01-01"}]}`))
		case "/v1/movies/search":
			metaTubeCalls++
			_, _ = w.Write([]byte(`{"data":[{"id":"ABC-123","provider":"local","number":"ABC-123","title":"MetaTube 影片"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewTmdbService("key", nil)
	svc.baseURL = server.URL
	svc.SetMetaTubeConfig(server.URL, "")
	svc.SetAdultContentEnabled(true)
	svc.SetMetaTubeDefaultEnabled(true)
	results, err := svc.SearchMovieBySource("测试", 0, domain.MetadataSourceAuto)
	if err != nil {
		t.Fatalf("auto search failed: %v", err)
	}
	if len(results) != 1 || results[0].MetadataSource != domain.MetadataSourceTMDB {
		t.Fatalf("expected TMDB result first, got %#v", results)
	}
	if metaTubeCalls != 0 {
		t.Fatalf("MetaTube should not be called after TMDB matched, calls=%d", metaTubeCalls)
	}
}

func TestSearchMovieBySourceAutoFallsBackToMetaTube(t *testing.T) {
	paths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/search/movie":
			_, _ = w.Write([]byte(`{"results":[]}`))
		case "/v1/movies/search":
			_, _ = w.Write([]byte(`{"data":[{"id":"MIAB-317","provider":"FANZA","number":"MIAB-317","title":"MetaTube 影片"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewTmdbService("key", nil)
	svc.baseURL = server.URL
	svc.SetMetaTubeConfig(server.URL, "")
	svc.SetAdultContentEnabled(true)
	svc.SetMetaTubeDefaultEnabled(true)
	results, err := svc.SearchMovieBySource("MIAB 317", 0, domain.MetadataSourceAuto)
	if err != nil {
		t.Fatalf("auto fallback search failed: %v", err)
	}
	if len(results) != 1 || results[0].MetadataSource != domain.MetadataSourceMetaTube || results[0].MetadataID != "MIAB-317" {
		t.Fatalf("expected MetaTube fallback result, got %#v", results)
	}
	if len(paths) != 2 || paths[0] != "/search/movie" || paths[1] != "/v1/movies/search" {
		t.Fatalf("unexpected search order: %v", paths)
	}
}

func TestSearchMovieBySourceAutoDoesNotHideTMDBFailure(t *testing.T) {
	metaTubeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/movie":
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		case "/v1/movies/search":
			metaTubeCalls++
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewTmdbService("key", nil)
	svc.baseURL = server.URL
	svc.SetMetaTubeConfig(server.URL, "")
	svc.SetAdultContentEnabled(true)
	svc.SetMetaTubeDefaultEnabled(true)
	if _, err := svc.SearchMovieBySource("测试", 0, domain.MetadataSourceAuto); err == nil {
		t.Fatal("TMDB request failure should be returned instead of treated as an empty result")
	}
	if metaTubeCalls != 0 {
		t.Fatalf("MetaTube should only be used after an empty TMDB result, calls=%d", metaTubeCalls)
	}
}

func TestIdentifyMovieHintAvoidsEpisodeRuleAndFallsBackToMetaTube(t *testing.T) {
	queries := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/movie":
			queries = append(queries, "tmdb:"+r.URL.Query().Get("query"))
			_, _ = w.Write([]byte(`{"results":[]}`))
		case "/v1/movies/search":
			queries = append(queries, "metatube:"+r.URL.Query().Get("q"))
			_, _ = w.Write([]byte(`{"data":[{"id":"MIAB-317","provider":"FANZA","number":"MIAB-317","title":"MetaTube 影片"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewTmdbService("key", nil)
	svc.baseURL = server.URL
	svc.SetMetaTubeConfig(server.URL, "")
	svc.SetAdultContentEnabled(true)
	svc.SetMetaTubeDefaultEnabled(true)
	rules := DefaultFilenameRecognitionRules()
	for index := range rules {
		if rules[index].ID == "anime_number" {
			rules[index].Enabled = true
		}
	}
	normalizedRules, compiledRules, compileErr := validateAndCompileFilenameRecognitionRules(rules)
	if compileErr != nil {
		t.Fatalf("compile filename rules failed: %v", compileErr)
	}
	svc.filenameRulesLoaded = true
	svc.filenameRules = normalizedRules
	svc.compiledFilenameRules = compiledRules
	if parsed := svc.parseFilename("MIAB-317-UC.mp4"); parsed.MediaType != "tv" || parsed.Title != "MIAB" {
		t.Fatalf("test fixture should reproduce episode-rule misclassification, got %#v", parsed)
	}
	result, err := svc.IdentifyFileWithPathBySourceAndType("MIAB-317-UC.mp4", domain.MetadataSourceAuto, "movie")
	if err != nil {
		t.Fatalf("identify failed: %v", err)
	}
	if !result.Success || result.MediaType != "movie" || result.MetadataID != "MIAB-317" {
		t.Fatalf("unexpected identify result: %#v", result)
	}
	if len(queries) != 2 || queries[0] != "tmdb:MIAB 317" || queries[1] != "metatube:MIAB 317" {
		t.Fatalf("unexpected queries: %v", queries)
	}
}

func TestSearchMovieBySourceRejectsUnconfiguredMetaTube(t *testing.T) {
	svc := NewTmdbService("key", nil)
	if _, err := svc.SearchMovieBySource("测试", 0, domain.MetadataSourceMetaTube); err == nil {
		t.Fatal("expected forced MetaTube search to require configured endpoint")
	}
}

func TestSearchMovieBySourceRejectsMetaTubeWhenAdultContentDisabled(t *testing.T) {
	svc := NewTmdbService("key", nil)
	svc.SetMetaTubeConfig("http://metatube.local", "")
	if _, err := svc.SearchMovieBySource("测试", 0, domain.MetadataSourceMetaTube); err == nil {
		t.Fatal("expected MetaTube search to require adult content confirmation")
	}
}

func TestIdentifyCacheHashSeparatesMetadataSources(t *testing.T) {
	tmdbHash := identifyCacheHash("Movie.mkv", domain.MetadataSourceTMDB)
	metaTubeHash := identifyCacheHash("Movie.mkv", domain.MetadataSourceMetaTube)
	if tmdbHash == metaTubeHash {
		t.Fatal("metadata source caches must be isolated")
	}
}
