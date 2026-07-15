package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

type mockScrapeSystemConfigDAO struct {
	values map[string]string
}

func (m *mockScrapeSystemConfigDAO) GetByKey(key string) (*domain.SystemConfig, error) {
	val, ok := m.values[key]
	if !ok {
		return nil, nil
	}
	return &domain.SystemConfig{ConfigKey: key, ConfigVal: val}, nil
}

func TestGenerateMovieNFOIncludesArtwork(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/original/poster.jpg", "/original/backdrop.jpg":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("img"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := &ScrapeService{
		imageBaseURL: server.URL + "/original",
		tmdbService: &TmdbService{
			httpClient: server.Client(),
		},
	}

	raw := json.RawMessage(`{
		"id": 42,
		"title": "Interstellar",
		"original_title": "Interstellar",
		"overview": "A team travels through a wormhole.",
		"tagline": "Mankind was born on Earth.",
		"release_date": "2014-11-07",
		"vote_average": 8.6,
		"poster_path": "/poster.jpg",
		"backdrop_path": "/backdrop.jpg",
		"runtime": 169,
		"genres": [{"id": 1, "name": "Sci-Fi"}],
		"production_countries": [{"name": "United States"}],
		"spoken_languages": [{"name": "English"}],
		"production_companies": [{"name": "Legendary Pictures"}],
		"belongs_to_collection": {"name": "Space Saga"},
		"credits": {
			"cast": [{"name": "Matthew McConaughey", "character": "Cooper", "order": 0, "profile_path": "/actor.jpg"}],
			"crew": [{"name": "Christopher Nolan", "job": "Director"}, {"name": "Jonathan Nolan", "job": "Writer"}]
		}
	}`)

	content, generatedFiles, err := svc.GenerateMovieNFO(raw, tmpDir, "Interstellar.2014.mkv", scrapeOutputOptions{
		WriteNFO:    true,
		WritePoster: true,
		WriteFanart: true,
		WriteThumb:  true,
	})
	if err != nil {
		t.Fatalf("GenerateMovieNFO failed: %v", err)
	}
	if !strings.Contains(content, "<tagline>Mankind was born on Earth.</tagline>") {
		t.Fatalf("missing tagline in nfo: %s", content)
	}
	if !strings.Contains(content, "<studio>Legendary Pictures</studio>") {
		t.Fatalf("missing studio in nfo: %s", content)
	}
	if !strings.Contains(content, "<writer>Jonathan Nolan</writer>") {
		t.Fatalf("missing writer in nfo: %s", content)
	}
	if !strings.Contains(content, "<director>Christopher Nolan</director>") {
		t.Fatalf("missing director in nfo: %s", content)
	}
	if len(generatedFiles) != 4 {
		t.Fatalf("expected 4 artwork files, got %d: %+v", len(generatedFiles), generatedFiles)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "Interstellar.2014-poster.jpg")); err != nil {
		t.Fatalf("missing poster file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "Interstellar.2014-fanart.jpg")); err != nil {
		t.Fatalf("missing fanart file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "poster.jpg")); err != nil {
		t.Fatalf("missing generic poster file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "fanart.jpg")); err != nil {
		t.Fatalf("missing generic fanart file: %v", err)
	}
}

func TestGenerateMovieNFORefreshesMinimalCachedPayload(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/42":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             42,
				"title":          "Interstellar",
				"original_title": "Interstellar",
				"overview":       "A team travels through a wormhole.",
				"release_date":   "2014-11-07",
				"vote_average":   8.6,
				"poster_path":    "/poster.jpg",
				"backdrop_path":  "/backdrop.jpg",
				"runtime":        169,
				"genres":         []map[string]interface{}{{"id": 1, "name": "Sci-Fi"}},
				"credits": map[string]interface{}{
					"cast": []map[string]interface{}{},
					"crew": []map[string]interface{}{{"name": "Christopher Nolan", "job": "Director"}},
				},
			})
		case "/original/poster.jpg", "/original/backdrop.jpg":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("img"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := &ScrapeService{
		imageBaseURL: server.URL + "/original",
		tmdbService: &TmdbService{
			apiKey:     "test-api-key",
			baseURL:    server.URL,
			language:   "zh-CN",
			httpClient: server.Client(),
		},
	}

	raw := json.RawMessage(`{
		"tmdb_id": 42,
		"title": "Interstellar",
		"poster_path": "https://image.tmdb.org/t/p/w500/poster.jpg"
	}`)

	content, generatedFiles, err := svc.GenerateMovieNFO(raw, tmpDir, "Interstellar.2014.mkv", scrapeOutputOptions{
		WriteNFO:    true,
		WritePoster: true,
		WriteFanart: true,
		WriteThumb:  true,
	})
	if err != nil {
		t.Fatalf("GenerateMovieNFO failed: %v", err)
	}
	if !strings.Contains(content, "<tmdbid>42</tmdbid>") {
		t.Fatalf("expected enriched tmdb id in nfo: %s", content)
	}
	if !strings.Contains(content, "backdrop.jpg") {
		t.Fatalf("expected enriched fanart reference in nfo: %s", content)
	}
	if len(generatedFiles) != 4 {
		t.Fatalf("expected 4 artwork files after detail refresh, got %d: %+v", len(generatedFiles), generatedFiles)
	}
}

func TestGenerateEpisodeNFOUsesEpisodeDetail(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tv/99/season/1/episode/2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             990102,
				"name":           "Episode Two",
				"overview":       "Episode plot",
				"air_date":       "2020-02-01",
				"vote_average":   8.1,
				"still_path":     "/still.jpg",
				"season_number":  1,
				"episode_number": 2,
				"credits": map[string]interface{}{
					"cast": []map[string]interface{}{
						{"name": "Actor One", "character": "Lead", "order": 0, "profile_path": "/actor1.jpg"},
					},
					"crew": []map[string]interface{}{
						{"name": "Director Two", "job": "Director"},
						{"name": "Writer Two", "job": "Screenplay"},
					},
				},
			})
		case "/original/tvposter.jpg", "/original/tvbackdrop.jpg", "/original/still.jpg":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("img"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmdbSvc := &TmdbService{
		apiKey:     "test-api-key",
		baseURL:    server.URL,
		language:   "zh-CN",
		httpClient: server.Client(),
	}

	svc := &ScrapeService{
		imageBaseURL: server.URL + "/original",
		tmdbService:  tmdbSvc,
	}

	raw := json.RawMessage(`{
		"id": 99,
		"name": "Show Name",
		"original_name": "Show Name",
		"overview": "Show overview",
		"first_air_date": "2020-01-01",
		"vote_average": 7.2,
		"poster_path": "/tvposter.jpg",
		"backdrop_path": "/tvbackdrop.jpg",
		"genres": [{"id": 1, "name": "Drama"}],
		"credits": {
			"cast": [{"name": "Series Actor", "character": "Hero", "order": 0, "profile_path": "/series-actor.jpg"}],
			"crew": [{"name": "Series Director", "job": "Director"}]
		}
	}`)

	content, generatedFiles, err := svc.GenerateEpisodeNFO(raw, tmpDir, "Show.S01E02.mkv", 1, 2, scrapeOutputOptions{
		WriteNFO:    true,
		WritePoster: true,
		WriteFanart: true,
		WriteThumb:  true,
	})
	if err != nil {
		t.Fatalf("GenerateEpisodeNFO failed: %v", err)
	}
	if !strings.Contains(content, "<title>Episode Two</title>") {
		t.Fatalf("missing episode title in nfo: %s", content)
	}
	if !strings.Contains(content, "<aired>2020-02-01</aired>") {
		t.Fatalf("missing aired date in nfo: %s", content)
	}
	if !strings.Contains(content, "<showtitle>Show Name</showtitle>") {
		t.Fatalf("missing show title in nfo: %s", content)
	}
	if len(generatedFiles) != 6 {
		t.Fatalf("expected 6 artwork files, got %d: %+v", len(generatedFiles), generatedFiles)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "Show.S01E02-poster.jpg")); err != nil {
		t.Fatalf("missing poster file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "Show.S01E02-fanart.jpg")); err != nil {
		t.Fatalf("missing fanart file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "Show.S01E02-thumb.jpg")); err != nil {
		t.Fatalf("missing thumb file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "poster.jpg")); err != nil {
		t.Fatalf("missing generic poster file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "fanart.jpg")); err != nil {
		t.Fatalf("missing generic fanart file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "thumb.jpg")); err != nil {
		t.Fatalf("missing generic thumb file: %v", err)
	}
}

func TestScrapeServiceOutputOptionsCanDisableArtworkAndNFO(t *testing.T) {
	tmpDir := t.TempDir()
	mediaPath := filepath.Join(tmpDir, "Movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("video"), 0644); err != nil {
		t.Fatalf("failed to seed media file: %v", err)
	}

	svc := &ScrapeService{
		systemConfigDAO: &mockScrapeSystemConfigDAO{values: map[string]string{
			"scrape_write_nfo":    "false",
			"scrape_write_poster": "false",
			"scrape_write_fanart": "false",
			"scrape_write_thumb":  "false",
		}},
	}

	content, _, generatedFiles, err := svc.scrapeResolvedFile(mediaPath, "Movie.mkv", "movie", json.RawMessage(`{
		"id": 7,
		"title": "Movie",
		"poster_path": "/poster.jpg",
		"backdrop_path": "/backdrop.jpg"
	}`), 0, 0)
	if err != nil {
		t.Fatalf("scrapeResolvedFile failed: %v", err)
	}
	if !strings.Contains(content, "<title>Movie</title>") {
		t.Fatalf("expected NFO content even when file write is disabled: %s", content)
	}
	if len(generatedFiles) != 0 {
		t.Fatalf("expected no artwork files, got %v", generatedFiles)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "Movie.nfo")); !os.IsNotExist(err) {
		t.Fatalf("expected NFO file to be skipped, stat err=%v", err)
	}
}

func TestBuildFallbackRawDataForMovie(t *testing.T) {
	rawData, err := buildFallbackRawData(&domain.TmdbIdentifyResult{
		TmdbID:        550,
		MediaType:     "movie",
		Title:         "Fight Club",
		OriginalTitle: "Fight Club",
		Year:          1999,
	})
	if err != nil {
		t.Fatalf("buildFallbackRawData returned error: %v", err)
	}
	text := string(rawData)
	if !strings.Contains(text, `"id":550`) {
		t.Fatalf("expected tmdb id in fallback payload: %s", text)
	}
	if !strings.Contains(text, `"title":"Fight Club"`) {
		t.Fatalf("expected title in fallback payload: %s", text)
	}
	if !strings.Contains(text, `"release_date":"1999-01-01"`) {
		t.Fatalf("expected release date in fallback payload: %s", text)
	}
}
