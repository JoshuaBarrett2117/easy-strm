package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"
)

func TestParseFilenameMovie(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)
	parsed := svc.parseFilename("Inception.2010.1080p.BluRay.x264.mkv")

	if parsed.Title != "Inception" {
		t.Fatalf("unexpected title: %q", parsed.Title)
	}
	if parsed.Year != 2010 {
		t.Fatalf("unexpected year: %d", parsed.Year)
	}
	if parsed.MediaType != "movie" {
		t.Fatalf("unexpected media type: %q", parsed.MediaType)
	}
}

func TestParseFilenameTV(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)
	parsed := svc.parseFilename("Breaking.Bad.S01E02.1080p.BluRay.mkv")

	if parsed.MediaType != "tv" || parsed.Season != 1 || parsed.Episode != 2 {
		t.Fatalf("unexpected tv parse: %+v", parsed)
	}
}

func TestParseFilenameFairyTailHundredYearsQuest(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)
	parsed := svc.parseFilename("妖精的尾巴 百年任务 - S01E05 - 艰难的决断.mp4")

	if parsed.Title != "妖精的尾巴 百年任务" {
		t.Fatalf("unexpected title: %q", parsed.Title)
	}
	if parsed.MediaType != "tv" || parsed.Season != 1 || parsed.Episode != 5 {
		t.Fatalf("unexpected tv parse: %+v", parsed)
	}
}

func TestParseFilenameUsesParentDirectoryContext(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)
	parsed := svc.parseFilename("默杀 2160P 高码率 非60帧版/默杀.A.Place.Called.Silence.2024.2160p.WEB-DL.H265.HQ.DDP5.1.mkv")

	if parsed.Title != "默杀 A Place Called Silence" {
		t.Fatalf("unexpected primary title: %q", parsed.Title)
	}
	if parsed.Year != 2024 {
		t.Fatalf("unexpected year: %d", parsed.Year)
	}

	foundParentTitle := false
	for _, title := range parsed.SearchTitles {
		if title == "默杀" {
			foundParentTitle = true
			break
		}
	}
	if !foundParentTitle {
		t.Fatalf("expected parent directory title to be included, got=%v", parsed.SearchTitles)
	}
}

func TestParseFilenameEnglishReleaseName(t *testing.T) {
	svc := NewTmdbService("fake-key", nil)
	parsed := svc.parseFilename("Maze.Runner.The.Death.Cure.2018.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.TrueHD.7.1.Atmos-老K.mkv")

	if parsed.Title != "Maze Runner The Death Cure" {
		t.Fatalf("unexpected primary title: %q", parsed.Title)
	}
	if parsed.Year != 2018 {
		t.Fatalf("unexpected year: %d", parsed.Year)
	}
	if parsed.MediaType != "movie" {
		t.Fatalf("unexpected media type: %q", parsed.MediaType)
	}
}

func TestBuildSearchQueryVariants(t *testing.T) {
	variants := buildSearchQueryVariants("Doraemon: Nobita's Dinosaur")

	want := map[string]bool{
		"Doraemon: Nobita's Dinosaur": false,
		"Doraemon Nobita s Dinosaur":  false,
		"DoraemonNobitasDinosaur":     false,
	}

	for _, variant := range variants {
		if _, ok := want[variant]; ok {
			want[variant] = true
		}
	}

	for variant, ok := range want {
		if !ok {
			t.Fatalf("expected variant %q to be generated, got=%v", variant, variants)
		}
	}
}

func TestApplyCachedMetadata(t *testing.T) {
	result := &domain.TmdbIdentifyResult{}
	raw := json.RawMessage(`{"genre_ids":[16,18],"origin_country":["JP"],"original_language":"ja"}`)

	applyCachedMetadata(result, raw)

	if len(result.GenreIDs) != 2 || result.GenreIDs[0] != 16 || result.GenreIDs[1] != 18 {
		t.Fatalf("unexpected genre ids: %+v", result.GenreIDs)
	}
	if len(result.Countries) != 1 || result.Countries[0] != "JP" {
		t.Fatalf("unexpected countries: %+v", result.Countries)
	}
	if result.Language != "ja" {
		t.Fatalf("unexpected language: %s", result.Language)
	}
}

func TestSearchMovieTop3(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/movie" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{"id": 100, "title": "Movie 1", "original_title": "Movie 1", "release_date": "2020-01-01", "poster_path": "/poster1.jpg", "overview": "desc1", "vote_average": 8.5},
				{"id": 200, "title": "Movie 2", "original_title": "Movie 2", "release_date": "2021-06-15", "poster_path": "/poster2.jpg", "overview": "desc2", "vote_average": 7.0},
				{"id": 300, "title": "Movie 3", "original_title": "Movie 3", "release_date": "2022-03-20", "poster_path": "/poster3.jpg", "overview": "desc3", "vote_average": 6.5},
				{"id": 400, "title": "Movie 4", "original_title": "Movie 4", "release_date": "2019-12-01", "poster_path": "/poster4.jpg", "overview": "desc4", "vote_average": 5.0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL

	results, err := svc.SearchMovie("test", 0)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected top 3 results, got %d", len(results))
	}
	if results[0].TmdbID != 100 || results[0].Year != 2020 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
}

func TestSearchMovieWithoutAPIKey(t *testing.T) {
	svc := NewTmdbService("", nil)
	if _, err := svc.SearchMovie("test", 0); err == nil {
		t.Fatal("expected error when API key is empty")
	}
}

func TestSearchMovieWithMaskedAPIKey(t *testing.T) {
	svc := NewTmdbService("f6a4****fea4", nil)
	if _, err := svc.SearchMovie("test", 0); err == nil {
		t.Fatal("expected error when API key is masked")
	}
}

func TestSettersAndHelpers(t *testing.T) {
	svc := NewTmdbService("k1", nil)
	svc.SetAPIKey("k2")
	svc.SetLanguage("en")
	customHTTPClient := &http.Client{}
	svc.SetHTTPClient(customHTTPClient)

	if svc.GetAPIKey() != "k2" {
		t.Fatalf("unexpected api key: %s", svc.GetAPIKey())
	}
	if svc.GetLanguage() != "en" {
		t.Fatalf("unexpected language: %s", svc.GetLanguage())
	}
	if svc.httpClient != customHTTPClient {
		t.Fatal("TMDB 服务应使用注入的 HTTP 客户端")
	}
	if svc.buildCacheKey("Inception.2010.mkv", "movie") != "inception.2010.mkv" {
		t.Fatalf("cache key should be lowercased")
	}
	if svc.getImageURL("/abc.jpg") != "https://image.tmdb.org/t/p/w500/abc.jpg" {
		t.Fatalf("unexpected image url")
	}
	if !looksLikeMaskedAPIKey("k1****k2") {
		t.Fatal("masked key should be treated as invalid")
	}
	if looksLikeMaskedAPIKey("real-api-key") {
		t.Fatal("real key should not be treated as masked")
	}
}

func TestGetTVEpisodeDetail(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/99/season/1/episode/2" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   990102,
			"name": "Episode Two",
		})
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL
	svc.httpClient = mockServer.Client()

	result, err := svc.GetTVEpisodeDetail(99, 1, 2)
	if err != nil {
		t.Fatalf("get episode detail failed: %v", err)
	}
	if result["name"] != "Episode Two" {
		t.Fatalf("unexpected episode payload: %+v", result)
	}
}

func TestIdentifyFileSearchFallback(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/movie" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}

		query := r.URL.Query().Get("query")
		resp := map[string]interface{}{"results": []map[string]interface{}{}}

		if query == "Doraemon Nobita's Dinosaur" {
			resp["results"] = []map[string]interface{}{
				{
					"id": 10515, "title": "Doraemon: Nobita's Dinosaur", "original_title": "Doraemon: Nobita's Dinosaur",
					"release_date": "1980-03-15", "poster_path": "/poster.jpg", "overview": "desc", "vote_average": 8.1,
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL

	result, err := svc.GetCandidates("Doraemon Nobita's Dinosaur.mp4")
	if err != nil {
		t.Fatalf("get candidates failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("search should succeed, got message: %s", result.Message)
	}
	if result.TmdbID != 10515 {
		t.Fatalf("unexpected tmdb id: %d", result.TmdbID)
	}
	if result.Title != "Doraemon: Nobita's Dinosaur" {
		t.Fatalf("unexpected title: %q", result.Title)
	}
}

func TestGetCandidatesSearchesParentDirectoryWhenFileTitleMisses(t *testing.T) {
	queries := make([]string, 0)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/movie" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}

		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		resp := map[string]interface{}{"results": []map[string]interface{}{}}

		if query == "默杀" {
			if r.URL.Query().Get("year") != "2024" {
				t.Fatalf("expected year context to be sent, got %q", r.URL.Query().Get("year"))
			}
			resp["results"] = []map[string]interface{}{
				{
					"id": 1263186, "title": "默杀", "original_title": "默杀",
					"release_date": "2024-07-03", "poster_path": "/poster.jpg", "overview": "desc", "vote_average": 7.0,
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL
	svc.httpClient = mockServer.Client()

	result, err := svc.GetCandidatesWithPath("默杀 2160P 高码率 非60帧版/默杀.A.Place.Called.Silence.2024.2160p.WEB-DL.H265.HQ.DDP5.1.mkv")
	if err != nil {
		t.Fatalf("get candidates failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("search should succeed, got message: %s, queries=%v", result.Message, queries)
	}
	if result.TmdbID != 1263186 || result.Title != "默杀" || result.Year != 2024 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGetCandidatesEnglishReleaseNameReturnsChineseMovieTitle(t *testing.T) {
	queries := make([]string, 0)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/movie" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}

		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		resp := map[string]interface{}{"results": []map[string]interface{}{}}

		if query == "Maze Runner The Death Cure" {
			if r.URL.Query().Get("year") != "2018" {
				t.Fatalf("expected year context to be sent, got %q", r.URL.Query().Get("year"))
			}
			resp["results"] = []map[string]interface{}{
				{
					"id": 336843, "title": "移动迷宫3：死亡解药", "original_title": "Maze Runner: The Death Cure",
					"release_date": "2018-01-10", "poster_path": "/poster.jpg", "overview": "desc", "vote_average": 7.1,
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	svc := NewTmdbService("test-api-key", nil)
	svc.baseURL = mockServer.URL
	svc.httpClient = mockServer.Client()

	result, err := svc.GetCandidates("Maze.Runner.The.Death.Cure.2018.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.TrueHD.7.1.Atmos-老K.mkv")
	if err != nil {
		t.Fatalf("get candidates failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("search should succeed, got message: %s, queries=%v", result.Message, queries)
	}
	if result.TmdbID != 336843 || result.Title != "移动迷宫3：死亡解药" || result.Year != 2018 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
