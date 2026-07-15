package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy-strm/internal/domain"
)

func TestEnsureIdentifyMetadataMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/42" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"genres": []map[string]interface{}{
				{"id": 16},
				{"id": 878},
			},
			"production_countries": []map[string]interface{}{
				{"iso_3166_1": "JP"},
			},
			"original_language": "ja",
			"title":             "盗梦空间",
			"original_title":    "Inception",
		})
	}))
	defer server.Close()

	svc := NewTmdbService("fake-key", nil)
	svc.baseURL = server.URL
	svc.httpClient = server.Client()

	result := &domain.TmdbIdentifyResult{
		TmdbID:    42,
		MediaType: "movie",
	}

	svc.EnsureIdentifyMetadata(result)

	if len(result.GenreIDs) != 2 || result.GenreIDs[0] != 16 || result.GenreIDs[1] != 878 {
		t.Fatalf("unexpected genre ids: %+v", result.GenreIDs)
	}
	if len(result.Countries) != 1 || result.Countries[0] != "JP" {
		t.Fatalf("unexpected countries: %+v", result.Countries)
	}
	if result.Language != "ja" {
		t.Fatalf("unexpected language: %s", result.Language)
	}
	if result.Title != "盗梦空间" {
		t.Fatalf("unexpected title: %s", result.Title)
	}
	if result.OriginalTitle != "Inception" {
		t.Fatalf("unexpected original title: %s", result.OriginalTitle)
	}
}

func TestEnsureIdentifyMetadataTVOriginalTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/1396" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"genres": []map[string]interface{}{
				{"id": 18},
			},
			"origin_country":    []string{"US"},
			"original_language": "en",
			"name":              "绝命毒师",
			"original_name":     "Breaking Bad",
		})
	}))
	defer server.Close()

	svc := NewTmdbService("fake-key", nil)
	svc.baseURL = server.URL
	svc.httpClient = server.Client()

	result := &domain.TmdbIdentifyResult{
		TmdbID:    1396,
		MediaType: "tv",
	}

	svc.EnsureIdentifyMetadata(result)

	if result.Title != "绝命毒师" {
		t.Fatalf("unexpected title: %s", result.Title)
	}
	if result.OriginalTitle != "Breaking Bad" {
		t.Fatalf("unexpected original title: %s", result.OriginalTitle)
	}
	if len(result.Countries) != 1 || result.Countries[0] != "US" {
		t.Fatalf("unexpected countries: %+v", result.Countries)
	}
}

func TestExtractMetadataFromDetailFallbackFields(t *testing.T) {
	detail := map[string]interface{}{
		"genres": []interface{}{
			map[string]interface{}{"id": float64(16)},
			map[string]interface{}{"id": float64(12)},
		},
		"production_countries": []interface{}{
			map[string]interface{}{"iso_3166_1": "JP"},
		},
		"origin_country": []interface{}{"JP", "US"},
	}

	genres := extractGenreIDsFromDetail(detail)
	if len(genres) != 2 || genres[0] != 16 || genres[1] != 12 {
		t.Fatalf("unexpected genre ids: %+v", genres)
	}

	movieCountries := extractCountriesFromDetail(detail, "movie")
	if len(movieCountries) != 2 || movieCountries[0] != "JP" || movieCountries[1] != "US" {
		t.Fatalf("unexpected movie countries: %+v", movieCountries)
	}

	tvCountries := extractCountriesFromDetail(detail, "tv")
	if len(tvCountries) != 2 || tvCountries[0] != "JP" || tvCountries[1] != "US" {
		t.Fatalf("unexpected tv countries: %+v", tvCountries)
	}
}
