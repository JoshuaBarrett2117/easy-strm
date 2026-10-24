package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"easy-strm/internal/dao"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestShareYearEvidenceRangeSeparators(t *testing.T) {
	service := NewTmdbService("fixture", nil)
	for _, collection := range []string{"Show_2005-2020", "Show.2005-2020_Complete", "Show.[2005-2020]", "Show-2005-2020", "Show_S01-S15_2005-2020", "Show.S01-S15.2005-2020"} {
		for _, filename := range []string{collection + "/Show.S01E01.mkv", collection + ".S01E01.mkv"} {
			t.Run(filename, func(t *testing.T) {
				for name, query := range map[string]ShareMediaQuery{"filename": AnalyzeShareFilename(filename), "query": service.analyzeShareQuery(filename, "tv")} {
					if query.rangeStart != 2005 || query.rangeEnd != 2020 || query.Year != 0 || query.fileYear != 0 || query.SeasonYear != 0 {
						t.Errorf("%s: %+v", name, query)
					}
					for _, title := range query.Titles {
						if strings.Contains(title, "2005") || strings.Contains(title, "2020") || strings.Contains(title, "S01") || strings.Contains(title, "S15") {
							t.Errorf("range leaked into title %q", title)
						}
					}
				}
			})
		}
	}
	for _, name := range []string{"Show.12005-2020", "Show.2005-20201", "Show.S001-S15", "Show.S01-S150"} {
		if shareYearRangeRE.MatchString(name) || shareSeasonRangeRE.MatchString(name) {
			t.Errorf("matched embedded numeric sequence %q", name)
		}
	}
	for _, filename := range []string{"Show.2005-2020/Show.S01E01.1999.mkv", "Show.S01E01.2005-2020.1999.mkv", "Show.2005-2020 (1999)/Show.S01E01.mkv"} {
		query := service.analyzeShareQuery(filename, "tv")
		if query.rangeStart != 2005 || query.rangeEnd != 2020 || query.Year != 1999 {
			t.Errorf("separate singleton lost: %s: %+v", filename, query)
		}
	}
	for _, title := range []string{"1917", "1900", "2001 A Space Odyssey"} {
		titles, year := cleanShareTitles(title)
		if year != 0 || len(titles) != 1 || titles[0] != title {
			t.Errorf("numeric title lost: %q: %v %d", title, titles, year)
		}
	}
}

func TestShareYearWorkCacheDoesNotHideAmbiguity(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/search/tv":
			fmt.Fprint(writer, `{"results":[{"id":1,"name":"Show","first_air_date":"2005-01-01"},{"id":2,"name":"Show","first_air_date":"2007-01-01"}]}`)
		case "/tv/1", "/tv/2":
			id, first, last := 1, 2005, 2010
			if request.URL.Path == "/tv/2" {
				id, first, last = 2, 2007, 2020
			}
			fmt.Fprintf(writer, `{"id":%d,"name":"Show","first_air_date":"%d-01-01","last_air_date":"%d-12-31","seasons":[{"season_number":2,"air_date":"%d-01-01"}]}`, id, first, last, first+1)
		default:
			fmt.Fprint(writer, `{"results":[],"translations":[]}`)
		}
	}))
	defer server.Close()
	service := NewTmdbService("fixture", nil)
	service.baseURL, service.httpClient = server.URL, server.Client()
	for _, years := range [][]int{{2006, 2009}, {2009, 2006}} {
		t.Run(fmt.Sprint(years), func(t *testing.T) {
			mini.FlushAll()
			ctx := WithRecognitionRound(context.Background())
			for index, year := range years {
				filename := fmt.Sprintf("剧集/Show/Show.S02E%02d.%d.mkv", index+1, year)
				result, err := service.IdentifyShareFile(ctx, filename, "tmdb", "tv")
				if err != nil || result.Success != (year == 2006) || year == 2009 && !strings.Contains(result.FailureReason, "冲突") {
					t.Fatalf("%s: %+v err=%v", filename, result, err)
				}
			}
		})
	}
}

func TestShareYearCollectionOverlapRemainsAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/search/tv":
			fmt.Fprint(writer, `{"results":[{"id":1,"name":"Show","first_air_date":"2005-01-01"},{"id":2,"name":"Show","first_air_date":"2010-01-01"}]}`)
		case "/tv/1", "/tv/2":
			id, first, last := 1, 2005, 2010
			if request.URL.Path == "/tv/2" {
				id, first, last = 2, 2010, 2020
			}
			fmt.Fprintf(writer, `{"id":%d,"name":"Show","first_air_date":"%d-01-01","last_air_date":"%d-12-31","seasons":[{"season_number":1,"air_date":"%d-01-01"}]}`, id, first, last, first)
		default:
			fmt.Fprint(writer, `{"results":[],"translations":[]}`)
		}
	}))
	defer server.Close()
	service := NewTmdbService("fixture", nil)
	service.baseURL, service.httpClient = server.URL, server.Client()
	for _, filename := range []string{"Show.2005-2020/Show.S01E01.mkv", "Show.S01E01.2005-2020.mkv", "Show.2005-2020/Show.S01E01.1999.mkv"} {
		result, err := service.IdentifyShareFile(context.Background(), filename, "tmdb", "tv")
		if err != nil || result.Success || !strings.Contains(filename, "1999") && !strings.Contains(result.FailureReason, "冲突") {
			t.Errorf("%s: %+v err=%v", filename, result, err)
		}
	}
}

func TestShareYearRelevantCandidateDetails(t *testing.T) {
	for _, mode := range []string{"irrelevant404", "irrelevant500", "alias_ambiguous", "alias_unresolved", "season_ambiguous", "season_unresolved", "multiple_seasons"} {
		t.Run(mode, func(t *testing.T) {
			var otherDetails atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/search/tv":
					firstTitle := "Show"
					if mode == "multiple_seasons" {
						firstTitle = "Main"
					}
					fmt.Fprintf(writer, `{"results":[{"id":1,"name":%q,"first_air_date":"2005-01-01"},{"id":2,"name":"Other","first_air_date":"2005-01-01"}]}`, firstTitle)
				case "/tv/1":
					seasons := `[{"season_number":1,"name":"Season 1","air_date":"2005-01-01"}]`
					if mode == "multiple_seasons" {
						seasons = `[{"season_number":1,"name":"Show","air_date":"2005-01-01"},{"season_number":2,"name":"Show","air_date":"2009-01-01"}]`
					}
					fmt.Fprintf(writer, `{"id":1,"name":"Show","first_air_date":"2005-01-01","last_air_date":"2020-01-01","seasons":%s}`, seasons)
				case "/tv/2/alternative_titles":
					if strings.HasPrefix(mode, "alias_") {
						fmt.Fprint(writer, `{"results":[{"title":"Show"}]}`)
					} else {
						fmt.Fprint(writer, `{"results":[]}`)
					}
				case "/tv/2/season/1":
					if strings.HasPrefix(mode, "season_") {
						fmt.Fprint(writer, `{"season_number":1,"name":"Show","air_date":"2009-01-01"}`)
					} else {
						http.NotFound(writer, request)
					}
				case "/tv/2":
					otherDetails.Add(1)
					if strings.HasSuffix(mode, "unresolved") || mode == "irrelevant500" {
						http.Error(writer, "fixture failure", 500)
					} else if mode == "irrelevant404" {
						http.NotFound(writer, request)
					} else if mode == "multiple_seasons" {
						fmt.Fprint(writer, `{"id":2,"name":"Other","first_air_date":"2005-01-01","last_air_date":"2020-01-01","seasons":[{"season_number":1,"name":"Season 1","air_date":"2009-01-01"}]}`)
					} else {
						fmt.Fprint(writer, `{"id":2,"name":"Other","first_air_date":"2005-01-01","last_air_date":"2020-01-01","seasons":[{"season_number":1,"name":"Show","air_date":"2009-01-01"}]}`)
					}
				default:
					fmt.Fprint(writer, `{"results":[],"translations":[]}`)
				}
			}))
			defer server.Close()
			service := NewTmdbService("fixture", nil)
			service.baseURL, service.httpClient = server.URL, server.Client()
			result, err := service.IdentifyShareFile(context.Background(), "Show.S01E01.2009.mkv", "tmdb", "tv")
			switch {
			case strings.HasPrefix(mode, "irrelevant"):
				if err != nil || !result.Success || result.TmdbID != 1 || otherDetails.Load() != 0 {
					t.Fatalf("result=%+v err=%v irrelevant details=%d", result, err, otherDetails.Load())
				}
			case strings.HasSuffix(mode, "unresolved"):
				if err == nil && result.Success {
					t.Fatalf("unresolved relevant evidence must not select first: %+v", result)
				}
			case mode == "alias_ambiguous" || mode == "season_ambiguous":
				// 主标题命中的候选优先于仅靠官方别名/季名命中的候选，不再因为后者撞名判成歧义。
				if err != nil || !result.Success || result.TmdbID != 1 {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			default:
				if err != nil || result.Success || !strings.Contains(result.FailureReason, "冲突") && mode != "multiple_seasons" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
		})
	}
}

func TestShareYearMissingSeasonPreservesWorkUnlessYearConflicts(t *testing.T) {
	service, _ := shareYearFixture(t, "Show", 1, "2005-01-01", "2020-01-01", 1, "2005-01-01")
	for _, filename := range []string{"Show.S02E01.mkv", "Show.S02E01.2009.mkv", "Show.S02E01.1999.mkv"} {
		result, err := service.IdentifyShareFile(context.Background(), filename, "tmdb", "tv")
		want := !strings.Contains(filename, "1999")
		if err != nil || result.Success != want || want && !strings.Contains(result.Message, "季信息待核对") {
			t.Errorf("%s: %+v err=%v", filename, result, err)
		}
	}
}

func TestShareYearWorkKeyEvidenceContract(t *testing.T) {
	service := NewTmdbService("fixture", nil)
	ctx := context.Background()
	for _, year := range []string{"", ".2009"} {
		first := "剧集/Show/Show.S02E01" + year + ".mkv"
		second := "剧集/Show/Show.S02E02" + year + ".mkv"
		if service.shareWorkKey(ctx, first, "tmdb", "tv") != service.shareWorkKey(ctx, second, "tmdb", "tv") {
			t.Errorf("same evidence must reuse work: %s %s", first, second)
		}
	}
	first := service.shareWorkKey(ctx, "剧集/Show/Show.S02E01.2009.mkv", "tmdb", "tv")
	for _, filename := range []string{"剧集/Show/Show.S02E02.2006.mkv", "剧集/Show/S02 (2010)/Show.S02E02.2009.mkv", "剧集/Show.2005-2020/Show.S02E02.2009.mkv"} {
		if first == service.shareWorkKey(ctx, filename, "tmdb", "tv") {
			t.Errorf("different evidence must not reuse work: %s", filename)
		}
	}
}

func TestShareYearSeasonFallbackCacheDoesNotHideAmbiguity(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/search/tv":
			fmt.Fprint(writer, `{"results":[{"id":1,"name":"Show","first_air_date":"2005-01-01"},{"id":2,"name":"Show","first_air_date":"2007-01-01"}]}`)
		case "/tv/1":
			fmt.Fprint(writer, `{"id":1,"name":"Show","first_air_date":"2005-01-01","seasons":[{"season_number":2,"air_date":"2009-01-01"},{"season_number":3,"air_date":"2009-12-01"}]}`)
		case "/tv/2":
			fmt.Fprint(writer, `{"id":2,"name":"Show","first_air_date":"2007-01-01","seasons":[{"season_number":2,"air_date":"2007-01-01"},{"season_number":3,"air_date":"2009-12-01"}]}`)
		default:
			fmt.Fprint(writer, `{"results":[],"translations":[]}`)
		}
	}))
	defer server.Close()
	service := NewTmdbService("fixture", nil)
	service.baseURL, service.httpClient = server.URL, server.Client()
	for _, seasons := range [][]int{{2, 3}, {3, 2}} {
		mini.FlushAll()
		ctx := WithRecognitionRound(context.Background())
		for _, season := range seasons {
			filename := fmt.Sprintf("Show/Show.S%02dE01.2009.mkv", season)
			result, err := service.IdentifyShareFile(ctx, filename, "tmdb", "tv")
			if err != nil || result.Success != (season == 2) || season == 3 && !strings.Contains(result.FailureReason, "冲突") {
				t.Fatalf("%s: %+v err=%v", filename, result, err)
			}
		}
	}
}
