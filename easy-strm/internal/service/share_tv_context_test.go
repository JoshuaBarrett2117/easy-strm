package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShareContextEpisodeRegression(t *testing.T) {
	for _, tc := range []struct {
		file, title     string
		season, episode int
	}{
		{"剧集/如懿传（2018）/第01集.mp4", "如懿传", 1, 1},
		{"剧集/爱情公寓系列.4K/爱情公寓.S02.4K（2011）/02.mp4", "爱情公寓", 2, 2},
		{"剧集/林深见鹿（2022）/S01E05.mp4", "林深见鹿", 1, 5},
		{"剧集/仙剑奇侠传三（2009）/第01集(1).mp4", "仙剑奇侠传三", 1, 1},
		{"剧集/喜羊羊与灰太狼/喜羊羊与灰太狼之羊羊运动会（2008）/[盲僧压制][艺洲人音像][喜羊羊与灰太狼之羊羊运动会][01][选拔圣火手][1080P].mkv", "喜羊羊与灰太狼之羊羊运动会", 1, 1},
		{"剧集/心理罪（2015）/S02.2016/心理罪.S02E051080P.mp4", "心理罪", 2, 5},
	} {
		s := NewTmdbService("fixture", nil)
		input := shareEpisodeInput(context.Background(), tc.file, "auto")
		p := s.ParseFilename(input)
		q := s.analyzeShareQuery(input, "auto")
		if p.Episode != tc.episode || p.Season != tc.season || q.MediaType != "tv" || !strings.Contains(strings.Join(q.Titles, "|"), tc.title) {
			t.Errorf("%s => input=%s parsed=%+v query=%+v", tc.file, input, p, q)
		}
	}
	for _, file := range []string{"电影/1917/01.mp4", "电影/作品/第01集.mp4", "剧集/作品/00017.m2ts", "01.mp4"} {
		if got := shareEpisodeInput(context.Background(), file, "auto"); got != file {
			t.Errorf("误推断 %s => %s", file, got)
		}
	}
}

func TestShareEpisodeMovieConfusion(t *testing.T) {
	for _, tc := range []struct {
		file, title string
		episode     int
	}{
		{"剧集/新白发魔女传/15.mkv", "新白发魔女传", 15},
		{"剧集/超能异族（2023）/第16集.mkv", "超能异族", 16},
		{"合集1/HQC/大清盐商 2014[全34集][国语中字][WEB-MP4]/06.mp4", "大清盐商", 6},
		{"合集1/HQC/家有喜妇 全42集.Happy.Wife.in.the.House.2014.1080p/09.mp4", "家有喜妇", 9},
		{"合集1/HQC/苏染染追夫记.EP01-50.2016.WEB-DL/06.mp4", "苏染染追夫记", 6},
		{"合集1/HQC/将夜.Ever.Night.EP00-60.2018.1080p/06.mp4", "将夜 Ever Night", 6},
	} {
		t.Run(tc.file, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/search/movie" {
					t.Error("剧集不能进入电影搜索")
					fmt.Fprintf(w, `{"results":[{"id":101230,"title":%q,"release_date":"1994-01-01"}]}`, r.URL.Query().Get("query"))
					return
				}
				if r.URL.Path == "/search/tv" {
					fmt.Fprintf(w, `{"results":[{"id":42,"name":%q,"first_air_date":"2014-01-01"}]}`, tc.title)
					return
				}
				fmt.Fprint(w, `{"id":42,"results":[],"seasons":[],"translations":[]}`)
			}))
			defer server.Close()
			s := NewTmdbService("fixture", nil)
			s.baseURL, s.httpClient = server.URL, server.Client()
			r, err := s.IdentifyShareFile(context.Background(), tc.file, "tmdb", "auto")
			if err != nil || r == nil || !r.Success || r.MediaType != "tv" || r.TmdbID != 42 || r.EpisodeNumber != tc.episode || r.SeasonNumber != 1 {
				t.Fatalf("result=%+v err=%v", r, err)
			}
		})
	}
}

func TestShareSeasonSearchRegression(t *testing.T) {
	for _, tc := range []struct {
		name, file   string
		season, year int
		want         bool
	}{
		{"庆余年", "剧集/庆余年/S02（2024）/庆余年.S02E01.mp4", 2, 2024, true},
		{"庆余年", "剧集/庆余年/S02（2030）/庆余年.S02E01.mp4", 2, 2030, false},
		{"风起霓裳", "剧集/风起西州（2023）/风起西州.S01E01.mp4", 2, 2023, true},
	} {
		t.Run(tc.file, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/search/tv":
					if r.URL.Query().Get("first_air_date_year") != "" {
						fmt.Fprint(w, `{"results":[]}`)
						return
					}
					fmt.Fprintf(w, `{"results":[{"id":1,"name":%q,"first_air_date":"2019-01-01"}]}`, tc.name)
				case "/tv/1":
					seasonName := "第 2 季"
					date := "2024-05-16"
					if tc.name == "风起霓裳" {
						seasonName = "风起西州"
						date = "2023-11-04"
					}
					fmt.Fprintf(w, `{"id":1,"name":%q,"seasons":[{"season_number":1,"name":"第 1 季","air_date":"2019-01-01","episode_count":40},{"season_number":2,"name":%q,"air_date":%q,"episode_count":40}]}`, tc.name, seasonName, date)
				default:
					fmt.Fprint(w, `{"results":[],"translations":[]}`)
				}
			}))
			defer server.Close()
			s := NewTmdbService("fixture", nil)
			s.baseURL = server.URL
			s.httpClient = server.Client()
			r, err := s.IdentifyShareFile(context.Background(), tc.file, "tmdb", "auto")
			if err != nil || r.Success != tc.want || tc.want && (r.SeasonNumber != tc.season || r.EpisodeNumber != 1) {
				t.Fatalf("result=%+v err=%v", r, err)
			}
		})
	}
}

func TestShareNumericWorkTitlePreserved(t *testing.T) {
	s := NewTmdbService("fixture", nil)
	for _, raw := range []string{"电影/1917.2019.mkv", "剧集/24/24.S01E01.mkv"} {
		q := s.analyzeShareQuery(raw, "auto")
		if len(q.Titles) == 0 {
			t.Fatalf("作品数字标题不应被清空: %s %+v", raw, q)
		}
	}
	q := s.analyzeShareQuery("剧集/合集/S01/01.mp4", "auto")
	for _, name := range q.Titles {
		if name == "01" {
			t.Fatal("集号不能用作片名")
		}
	}
}

func TestShareChineseSeasonNumbers(t *testing.T) {
	for _, tt := range []struct {
		name string
		want int
	}{{"第一季", 1}, {"第二季", 2}, {"第三季", 3}, {"第九季", 9}, {"第十季", 10}, {"第十一季", 11}, {"第十九季", 19}, {"第二十季", 20}, {"第二十一季", 21}, {"第00季", 0}} {
		if got, ok := shareContextSeason(tt.name); !ok || got != tt.want {
			t.Errorf("%s: got=%d ok=%v want=%d", tt.name, got, ok, tt.want)
		}
	}
}

func TestShareSeasonDirectoryFullWidthYearBoundary(t *testing.T) {
	for _, tt := range []struct {
		name string
		want int
	}{
		{"S02（2011）", 2},
		{"S03(2015)", 3},
		{"Season 04（2016）", 4},
	} {
		if got, ok := shareContextSeason(tt.name); !ok || got != tt.want {
			t.Fatalf("%s: got=%d ok=%v want=%d", tt.name, got, ok, tt.want)
		}
	}
}

func TestShareSeasonAliasSurvivesFinalValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			fmt.Fprint(w, `{"results":[{"id":99374,"name":"爱情公寓：辣味英雄传","first_air_date":"2015-04-05"}]}`)
		case "/tv/99374":
			fmt.Fprint(w, `{"id":99374,"seasons":[{"season_number":2,"name":"第 2 季","air_date":"2015-12-15","episode_count":4}]}`)
		case "/tv/99374/alternative_titles":
			fmt.Fprint(w, `{"results":[{"title":"爱情公寓番外篇：辣味英雄传"}]}`)
		default:
			fmt.Fprint(w, `{"results":[],"translations":[]}`)
		}
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	r, e := s.IdentifyShareFile(context.Background(), "剧集/爱情公寓番外篇：辣味英雄传 第二季（2015）/01.mp4", "tmdb", "auto")
	if e != nil || !r.Success || r.TmdbID != 99374 || r.SeasonNumber != 2 || r.EpisodeNumber != 1 {
		t.Fatalf("%+v %v", r, e)
	}
}
