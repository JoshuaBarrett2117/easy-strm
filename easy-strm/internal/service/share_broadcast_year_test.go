package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"easy-strm/internal/domain"

	"easy-strm/internal/dao"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func shareYearFixture(t *testing.T, title string, id int, first, last any, season int, seasonDate any) (*TmdbService, *atomic.Int32) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	details := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search/tv":
			if r.URL.Query().Get("first_air_date_year") != "" {
				t.Error("episode year must not restrict premiere search")
			}
			json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"id": id, "name": title, "original_name": title, "first_air_date": first}}})
		case r.URL.Path == fmt.Sprintf("/tv/%d", id):
			details.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"id": id, "name": title, "original_name": title, "first_air_date": first, "last_air_date": last, "seasons": []any{map[string]any{"season_number": season, "name": fmt.Sprintf("第 %d 季", season), "air_date": seasonDate}}})
		case strings.Contains(r.URL.Path, "/season/"):
			json.NewEncoder(w).Encode(map[string]any{"season_number": season, "air_date": seasonDate})
		default:
			fmt.Fprint(w, `{"results":[],"translations":[]}`)
		}
	}))
	t.Cleanup(server.Close)
	s := NewTmdbService("fixture", nil)
	s.baseURL, s.httpClient = server.URL, server.Client()
	return s, details
}

func TestShareBroadcastYearFullRecognition(t *testing.T) {
	for _, test := range []struct {
		name, title, file, first, last, seasonDate string
		id, season, episode                        int
		want                                       bool
	}{
		{"supernatural", "Supernatural", "剧集/[邪恶力量][全15季]Supernatural.S01-S15.2005-2020/Supernatural.S01E01.2009.1080p.BluRay.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, true},
		{"sheldon", "Young Sheldon", "剧集/[小谢尔顿][全7季]Young.Sheldon.2017-2024/Young.Sheldon.S07E11.2024.1080p.WEB-DL.mkv", "2017-09-25", "2024-05-16", "2024-02-15", 71728, 7, 11, true},
		{"poirot", "Poirot", "剧集/Poirot/Poirot.S10E01.2006.1080p.mkv", "1989-01-08", "", "2006-01-01", 790, 10, 1, true},
		{"berlin", "Babylon Berlin", "剧集/Babylon.Berlin/Babylon.Berlin.S03E01.1080p.mkv", "2017-10-13", "2022-10-22", "2020-01-24", 66980, 3, 1, true},
		{"directory_interval", "Supernatural", "剧集/Supernatural (2009)/Supernatural.S01E01.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, true},
		{"range_only", "Supernatural", "剧集/Supernatural.S01-S15.2005-2020/Supernatural.S01E01.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, true},
		{"range_season", "Poirot", "剧集/Poirot.1989-2013/Poirot.S10E01.2006.mkv", "1989-01-08", "", "2006-01-01", 790, 10, 1, true},
		{"invalid_standalone", "Supernatural", "Supernatural.S01E01.1999.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, false},
		{"invalid_range_file", "Supernatural", "剧集/Supernatural.2005-2020/Supernatural.S01E01.1999.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, false},
		{"invalid_directory", "Supernatural", "剧集/Supernatural (1999)/Supernatural.S01E01.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, false},
		{"invalid_range", "Supernatural", "剧集/Supernatural.1990-1999/Supernatural.S01E01.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, false},
		{"unrelated", "Supernatural", "Other.Show.S01E01.2009.mkv", "2005-09-13", "2020-11-19", "2005-09-13", 1622, 1, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := shareYearFixture(t, test.title, test.id, test.first, test.last, test.season, test.seasonDate)
			result, err := s.IdentifyShareFile(context.Background(), test.file, "tmdb", "tv")
			if err != nil || result == nil || result.Success != test.want || test.want && (result.TmdbID != test.id || result.SeasonNumber != test.season || result.EpisodeNumber != test.episode) {
				t.Logf("input=%s query=%+v", shareEpisodeInput(context.Background(), test.file, "tv"), s.analyzeShareQuery(test.file, "tv"))
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestShareBroadcastYearBoundaries(t *testing.T) {
	for _, test := range []struct {
		name                string
		first, last, season any
		year                int
		want                bool
	}{
		{"first", "2005-09-13", "2020-11-19", "2005-09-13", 2005, true},
		{"last", "2005-09-13", "2020-11-19", "2005-09-13", 2020, true},
		{"before", "2005-09-13", "2020-11-19", "2005-09-13", 2004, false},
		{"after", "2005-09-13", "2020-11-19", "2005-09-13", 2021, false},
		{"future", "2005-09-13", "2099-11-19", "2005-09-13", 2099, true},
		{"season_minus", "1989-01-08", nil, "2006-01-01", 2005, true},
		{"season_plus", "1989-01-08", "bad", "2006-01-01", 2007, true},
		{"season_outside", "1989-01-08", "", "2006-01-01", 2008, false},
		{"premiere", "2005-09-13", nil, nil, 2006, true},
		{"premiere_outside", "2005-09-13", nil, nil, 2009, false},
		{"unknown", nil, nil, nil, 2009, false},
		{"invalid", "bad", "2020-99-99", "garbage", 2009, false},
		{"invalid_date_with_year", "2009-99-99", "bad", nil, 2009, false},
		{"missing_first_season", nil, nil, "2006-01-01", 2006, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := shareYearFixture(t, "Show", 1, test.first, test.last, 1, test.season)
			result, err := s.IdentifyShareFile(context.Background(), fmt.Sprintf("Show.S01E01.%d.mkv", test.year), "tmdb", "tv")
			if err != nil || result.Success != test.want {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestShareBroadcastRangeEvidencePreserved(t *testing.T) {
	s := NewTmdbService("fixture", nil)
	file := "剧集/[邪恶力量][全15季]Supernatural.S01-S15.2005-2020/Supernatural.S01E01.2009.1080p.mkv"
	for _, query := range []ShareMediaQuery{AnalyzeShareFilename(file), s.analyzeShareQuery(file, "tv")} {
		if query.fileYear != 2009 || query.rangeStart != 2005 || query.rangeEnd != 2020 || !strings.Contains(strings.Join(query.Titles, "|"), "Supernatural") {
			t.Fatalf("lost evidence: %+v", query)
		}
		for _, title := range query.Titles {
			if strings.Contains(title, "2005") || strings.Contains(title, "2020") || strings.Contains(title, "S15") {
				t.Fatalf("unclean title=%q", title)
			}
		}
	}
	if query := s.analyzeShareQuery("剧集/Supernatural.S01-S15/Supernatural.S01E01.mkv", "tv"); query.rangeStart != 0 || query.rangeEnd != 0 {
		t.Fatalf("season range is not a year range: %+v", query)
	}
	ctx := WithRecognitionRound(context.Background())
	other := strings.ReplaceAll(file, "2005-2020", "2017-2024")
	if s.shareWorkKey(ctx, file, "tmdb", "tv") == s.shareWorkKey(ctx, other, "tmdb", "tv") {
		t.Fatal("different range contexts collide")
	}
}

func TestShareBroadcastAmbiguousCandidates(t *testing.T) {
	for _, mode := range []string{"direct", "alias", "season_name"} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", mode, reversed), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.URL.Path == "/search/tv":
						title := "Same Show"
						if mode != "direct" {
							title = "Official Show"
						}
						items := []any{map[string]any{"id": 1, "name": title, "first_air_date": "2005-01-01"}, map[string]any{"id": 2, "name": title, "first_air_date": "2007-01-01", "popularity": 1000}}
						if reversed {
							items[0], items[1] = items[1], items[0]
						}
						json.NewEncoder(w).Encode(map[string]any{"results": items})
					case strings.HasSuffix(r.URL.Path, "alternative_titles"):
						if mode == "alias" {
							fmt.Fprint(w, `{"results":[{"title":"Same Show"}]}`)
						} else {
							fmt.Fprint(w, `{"results":[]}`)
						}
					case strings.HasSuffix(r.URL.Path, "/season/2"):
						fmt.Fprint(w, `{"season_number":2,"name":"Same Show","air_date":"2009-01-01"}`)
					case r.URL.Path == "/tv/1" || r.URL.Path == "/tv/2":
						seasonName := "第 2 季"
						if mode == "season_name" {
							seasonName = "Same Show"
						}
						first, last := "2005-01-01", "2020-01-01"
						if r.URL.Path == "/tv/2" {
							first, last = "2007-01-01", "2022-01-01"
						}
						json.NewEncoder(w).Encode(map[string]any{"first_air_date": first, "last_air_date": last, "seasons": []any{map[string]any{"season_number": 2, "name": seasonName, "air_date": "2009-01-01"}}})
					default:
						fmt.Fprint(w, `{"results":[],"translations":[]}`)
					}
				}))
				defer server.Close()
				s := NewTmdbService("fixture", nil)
				s.baseURL, s.httpClient = server.URL, server.Client()
				result, err := s.IdentifyShareFile(context.Background(), "Same.Show.S02E01.2009.1080p.mkv", "tmdb", "tv")
				if err != nil || result.Success || !strings.Contains(result.FailureReason, "同名候选") {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			})
		}
	}
}

func TestShareBroadcastSeasonEndpointAndSharedDetail(t *testing.T) {
	var details, seasons atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			fmt.Fprint(w, `{"results":[{"id":790,"name":"Poirot","first_air_date":"1989-01-08"}]}`)
		case "/tv/790":
			details.Add(1)
			fmt.Fprint(w, `{"id":790,"name":"Poirot","first_air_date":"1989-01-08","seasons":[{"season_number":10}]}`)
		case "/tv/790/season/10":
			seasons.Add(1)
			fmt.Fprint(w, `{"season_number":10,"air_date":"2006-01-01"}`)
		default:
			fmt.Fprint(w, `{"results":[]}`)
		}
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL, s.httpClient = server.URL, server.Client()
	ctx := WithRecognitionRound(context.Background())
	for episode := 1; episode <= 3; episode++ {
		result, err := s.IdentifyShareFile(ctx, fmt.Sprintf("Poirot.S10E%02d.2006.mkv", episode), "tmdb", "tv")
		if err != nil || !result.Success || result.SeasonNumber != 10 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if details.Load() != 1 || seasons.Load() != 1 {
		t.Fatalf("details=%d seasons=%d", details.Load(), seasons.Load())
	}
}

func TestShareBroadcastMovieYearPolicyUnchanged(t *testing.T) {
	for _, year := range []int{0, 2020, 2023, 2024, 2025, 2026, 2027} {
		query := ShareMediaQuery{Titles: []string{"Movie"}, Year: 2025, MediaType: "movie", fileYear: 2025, rangeStart: 2000, rangeEnd: 2030}
		candidate := domain.TmdbSearchResult{TmdbID: 1, Title: "Movie", MediaType: "movie", Year: year}
		if got := shareCandidateYearMatches(query, candidate); got != (year >= 2024 && year <= 2026) {
			t.Fatalf("release=%d matches=%v", year, got)
		}
	}
}

func TestShareBroadcastUnavailableSeasonFallsBackToPremiere(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			fmt.Fprint(w, `{"results":[{"id":1,"name":"Show","first_air_date":"2005-01-01"}]}`)
		case "/tv/1":
			fmt.Fprint(w, `{"id":1,"name":"Show","first_air_date":"2005-01-01","last_air_date":null}`)
		case "/tv/1/season/1":
			http.NotFound(w, r)
		default:
			fmt.Fprint(w, `{"results":[],"translations":[]}`)
		}
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL, s.httpClient = server.URL, server.Client()
	for _, year := range []int{2004, 2005, 2006, 2009} {
		result, err := s.IdentifyShareFile(context.Background(), fmt.Sprintf("Show.S01E01.%d.mkv", year), "tmdb", "tv")
		if err != nil || result.Success != (year <= 2006) {
			t.Fatalf("year=%d result=%+v err=%v", year, result, err)
		}
	}
}

func TestShareBroadcastSeasonEvidenceUsesDetail(t *testing.T) {
	candidate := domain.TmdbSearchResult{TmdbID: 1622, Title: "Supernatural", MediaType: "tv", Year: 2005}
	detail := map[string]interface{}{"first_air_date": "2005-09-13", "last_air_date": "2020-11-19", "seasons": []interface{}{map[string]interface{}{"season_number": float64(1), "air_date": "2005-09-13"}}}
	for _, year := range []int{1999, 2009} {
		query := ShareMediaQuery{Titles: []string{"Supernatural"}, MediaType: "tv", Year: year, SeasonYear: year, Season: 1}
		season, ok := shareSeasonEvidence(query, candidate, detail, nil)
		if ok != (year == 2009) || ok && season != 1 {
			t.Fatalf("year=%d season=%d ok=%v", year, season, ok)
		}
	}
}

func TestShareBroadcastInvalidFirstDoesNotPoisonWorkReuse(t *testing.T) {
	s, _ := shareYearFixture(t, "Young Sheldon", 71728, "2017-09-25", "2024-05-16", 7, "2024-02-15")
	ctx := WithRecognitionRound(context.Background())
	for _, year := range []int{1999, 2024, 1999} {
		result, err := s.IdentifyShareFile(ctx, fmt.Sprintf("剧集/Young.Sheldon.2017-2024/Young.Sheldon.S07E11.%d.mkv", year), "tmdb", "tv")
		if err != nil || result.Success != (year == 2024) {
			t.Fatalf("year=%d result=%+v err=%v", year, result, err)
		}
	}
}

func TestShareBroadcastMovieConflictsFullRecognition(t *testing.T) {
	for _, date := range []string{"2020-01-01", ""} {
		t.Run(date, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/search/movie" {
					fmt.Fprintf(w, `{"results":[{"id":1,"title":"Movie","release_date":%q}]}`, date)
				} else {
					fmt.Fprint(w, `{"results":[]}`)
				}
			}))
			defer server.Close()
			s := NewTmdbService("fixture", nil)
			s.baseURL, s.httpClient = server.URL, server.Client()
			result, err := s.IdentifyShareFile(context.Background(), "Movie.2025.1080p.BluRay.mkv", "tmdb", "movie")
			if err != nil || result.Success || result.FailureReason == "" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestShareBroadcastWorkReuseRechecksYear(t *testing.T) {
	s, details := shareYearFixture(t, "Supernatural", 1622, "2005-09-13", "2020-11-19", 1, "2005-09-13")
	ctx := WithRecognitionRound(context.Background())
	validFile := "剧集/Supernatural (2005)/Supernatural.S01E01.2009.mkv"
	invalidFile := "剧集/Supernatural (2005)/Supernatural.S01E02.1999.mkv"
	if s.shareWorkKey(ctx, validFile, "tmdb", "tv") == s.shareWorkKey(ctx, invalidFile, "tmdb", "tv") {
		t.Fatal("different filename years must reselect candidates")
	}
	for index, year := range []int{2009, 2009, 2010, 1999} {
		file := fmt.Sprintf("剧集/Supernatural (2005)/Supernatural.S01E%02d.%d.mkv", index+1, year)
		result, err := s.IdentifyShareFile(ctx, file, "tmdb", "tv")
		if err != nil || result.Success != (year != 1999) {
			t.Fatalf("%s result=%+v err=%v", file, result, err)
		}
	}
	if details.Load() != 1 {
		t.Fatalf("shared detail calls=%d", details.Load())
	}
	ctx.Value(recognitionRoundKey{}).(*recognitionRound).mu.Lock()
	works := len(ctx.Value(recognitionRoundKey{}).(*recognitionRound).works)
	ctx.Value(recognitionRoundKey{}).(*recognitionRound).mu.Unlock()
	if works != 2 {
		t.Fatalf("expected one successful work per distinct year, got %d", works)
	}
}
