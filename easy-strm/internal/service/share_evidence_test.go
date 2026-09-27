package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

func TestShareEvidenceTitleYears(t *testing.T) {
	s := NewTmdbService("fixture", nil)
	for _, tt := range []struct {
		input, title string
		year         int
	}{
		{"唐探1900 (2025)/唐探1900 [DIY].Detective.Chinatown.1900.2025.1080p.iso", "唐探1900", 2025},
		{"1917.2019.1080p.mkv", "1917", 2019},
		{"2001.A.Space.Odyssey.1968.mkv", "2001 A Space Odyssey", 1968},
		{"[绿皮书][UHD原盘DIY].Green.Book.2018.ULTRAHD.Blu-ray.iso", "绿皮书", 2018},
	} {
		q := s.analyzeShareQuery(tt.input, "auto")
		if q.Year != tt.year || !strings.Contains(strings.Join(q.Titles, "|"), tt.title) {
			t.Errorf("%s: %+v", tt.input, q)
		}
	}
}

func TestShareEvidenceSearch(t *testing.T) {
	for _, scenario := range []string{"eighth", "translation", "bilingual", "ambiguous", "missing_year", "stream_year", "year_conflict", "parent_title", "release_year"} {
		t.Run(scenario, func(t *testing.T) {
			calls := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls[r.URL.Path]++
				results := []map[string]any{}
				title := "目标电影"
				original := "Target"
				date := "2025-01-01"
				switch scenario {
				case "translation":
					title = "重生岛"
					original = "Остров"
				case "missing_year":
					date = ""
				case "stream_year", "year_conflict":
					date = "2023-01-01"
				case "release_year":
					date = "2024-01-01"
				}
				switch {
				case r.URL.Path == "/search/movie":
					if scenario == "release_year" && r.URL.Query().Get("year") != "" {
						break
					}
					if scenario == "parent_title" && r.URL.Query().Get("query") != "目标电影" {
						break
					}
					if scenario == "eighth" {
						for i := 1; i < 8; i++ {
							results = append(results, map[string]any{"id": i, "title": fmt.Sprintf("其他%d", i), "release_date": date})
						}
					}
					results = append(results, map[string]any{"id": 8, "title": title, "original_title": original, "release_date": date})
					if scenario == "bilingual" {
						results = append(results, map[string]any{"id": 9, "title": "Target", "original_title": "Target", "release_date": date})
					}
					if scenario == "ambiguous" {
						results = append(results, map[string]any{"id": 9, "title": title, "original_title": original, "release_date": date})
					}
				case strings.HasSuffix(r.URL.Path, "/translations"):
					json.NewEncoder(w).Encode(map[string]any{"translations": []any{map[string]any{"iso_639_1": "zh", "iso_3166_1": "TW", "data": map[string]any{"title": "炭疽836"}}}})
					return
				case strings.HasSuffix(r.URL.Path, "/release_dates"):
					fmt.Fprint(w, `{"results":[]}`)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"results": results})
			}))
			defer server.Close()
			s := NewTmdbService("fixture", nil)
			s.baseURL = server.URL
			s.httpClient = server.Client()
			filename := "目标电影.Target.2025.mkv"
			switch scenario {
			case "translation":
				filename = "炭疽836.2025.mkv"
			case "stream_year":
				filename = "目标电影 (2023) [2025年流媒体版]/Target.2025.mkv"
			case "parent_title":
				filename = "目标电影 (2025)/TARGET.2025.mkv"
			}
			result, err := s.identifyShareWithAssistUncached(WithRecognitionRound(context.Background()), filename, "tmdb", "movie")
			if err != nil {
				t.Fatal(err)
			}
			want := scenario != "ambiguous" && scenario != "missing_year" && scenario != "year_conflict"
			if result.Success != want || want && result.TmdbID != 8 {
				t.Fatalf("%s: %+v", scenario, result)
			}
			if !want && (len(result.Candidates) == 0 || result.FailureReason == "") {
				t.Fatalf("失败应保留候选及明确原因: %+v", result)
			}
		})
	}
}

func TestShareEvidenceMissingYearStillRejected(t *testing.T) {
	q := ShareMediaQuery{Titles: []string{"目标电影"}, Year: 2025, MediaType: "movie"}
	if selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{{TmdbID: 1, Title: "目标电影", MediaType: "movie"}}) != nil {
		t.Fatal("未知年份不能自动确认")
	}
}
