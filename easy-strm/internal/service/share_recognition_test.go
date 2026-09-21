package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestImportedShareDefaultsToAuto 确认新增及批量导入省略类型时以auto写入数据库。
func TestImportedShareDefaultsToAuto(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_share_record").WithArgs("测试分享", "https://115.com/s/example", "", "", "auto").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "created_at", "updated_at"}).AddRow(1, 1, "now", "now"))
	mock.ExpectCommit()
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
	record := &domain.ShareRecord{Name: "测试分享", URL: "https://115.com/s/example"}
	if err := s.Create(context.Background(), record); err != nil || record.MediaType != "auto" {
		t.Fatalf("%+v %v", record, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareRawDiscQueries(t *testing.T) {
	tests := []struct {
		raw, title, kind string
		year             int
	}{
		{`[The.Godfather.1972][Blu-ray].iso`, "The Godfather", "unknown", 1972},
		{`【 BD-ISO 】整理好的圆盘\电影ISO-01\一代宗师_The_Grandmaster_2013]美版_DIY简繁中字@leo]41GB].iso`, "一代宗师", "unknown", 2013},
		{`4K原盘\086.[绿皮书][第86部UHD原盘DIY 国语音轨][59GB].Green.Book.2018.ULTRAHD.Blu-ray.iso`, "绿皮书", "unknown", 2018},
		{`七龙珠[国粤日语 简体中文 153集全]Dragonball.1986.ESP.Blu-ray\Dragonball.1986.ESP.D11.Blu-ray.iso`, "七龙珠", "tv", 1986},
		{`老友记全10季美国豪华版BDBOX[原盘中字] Friends The Complete Series Blu-ray Box`, "老友记", "tv", 0},
		{`七龙珠 法版`, "七龙珠", "unknown", 0},
		{`[越狱 Jailbreak 2017][DIY简繁中字].iso`, "越狱 Jailbreak", "unknown", 2017},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := AnalyzeShareFilename(tt.raw)
			if got.MediaType != tt.kind || got.Year != tt.year {
				t.Fatalf("%+v", got)
			}
			found := false
			for _, title := range got.Titles {
				found = found || title == tt.title
			}
			if !found {
				t.Fatalf("want=%s got=%+v", tt.title, got)
			}
			for _, title := range got.Titles {
				if strings.Contains(title, "电影ISO") {
					t.Fatal(got)
				}
			}
		})
	}
	for _, name := range []string{"电影ISO-01", "【 BD-ISO 】整理好的圆盘", "原盘电影合集（604T）", "4K原盘 252V 16.94T"} {
		if !AnalyzeShareFilename(name).Container {
			t.Fatal(name)
		}
	}
}

func TestShareDiscFilenameRecognition(t *testing.T) {
	tests := []struct {
		name, title string
		year        int
	}{
		{"Dexter S06 Disc02.iso", "Dexter", 0},
		{"Banshee.S02.2014.Disc2.1080p.GBR.Blu-ray.AVC.DTS-HD.MA.5.1@blucook#162.iso", "Banshee", 2014},
		{"[黑道家族 第三季 The Sopranos Season 3 2001]...Disc2.iso", "黑道家族", 2001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := AnalyzeShareFilename(tt.name)
			if q.MediaType != "tv" || q.Year != tt.year {
				t.Fatalf("got=%+v", q)
			}
			for _, title := range q.Titles {
				if strings.Contains(strings.ToLower(title), "disc") || strings.Contains(title, "S06") || strings.Contains(title, "Season") || strings.Contains(title, "第三季") {
					t.Fatalf("marker leaked into title: %+v", q)
				}
			}
			found := false
			for _, title := range q.Titles {
				if title == tt.title {
					found = true
				}
			}
			if !found {
				t.Fatalf("want title %q got=%+v", tt.title, q)
			}
		})
	}
}

func TestShareCandidateVerification(t *testing.T) {
	q := ShareMediaQuery{Titles: []string{"一代宗师", "The Grandmaster"}, Year: 2013, MediaType: "unknown"}
	wrong := domain.TmdbSearchResult{Title: "海贼王：强者天下", Year: 2009, TmdbID: 1, MediaType: "movie"}
	good := domain.TmdbSearchResult{Title: "一代宗师", Year: 2013, TmdbID: 2, MediaType: "movie"}
	if selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{wrong}) != nil {
		t.Fatal("不可接受无关首项")
	}
	if got := selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{wrong, good}); got == nil || got.TmdbID != 2 {
		t.Fatal(got)
	}
	duplicate := good
	duplicate.TmdbID = 3
	if selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{good, duplicate}) != nil {
		t.Fatal("多项同名同年应待确认")
	}
	q.MediaType = "tv"
	if selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{good}) != nil {
		t.Fatal("显式类型不得错配")
	}
}

func TestShareMixedLookupDoesNotForceMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		results := []map[string]any{}
		if r.URL.Path == "/search/movie" {
			results = append(results, map[string]any{"id": 1, "title": "七龙珠：进化", "original_title": "Dragonball Evolution", "release_date": "2009-01-01"})
		}
		if r.URL.Path == "/search/tv" {
			results = append(results, map[string]any{"id": 2, "name": "七龙珠", "original_name": "Dragon Ball", "first_air_date": "1986-01-01"})
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer server.Close()
	s := NewTmdbService("test", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	result, err := s.IdentifyShareFile(context.Background(), "七龙珠 法版", "tmdb", "auto")
	if err != nil || !result.Success || result.MediaType != "tv" || result.TmdbID != 2 {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = s.IdentifyShareFile(context.Background(), "七龙珠 法版", "tmdb", "movie")
	if err != nil || result.Success {
		t.Fatalf("不得将电影错配接受为成功: %+v %v", result, err)
	}
}

// TestShareEpisodeTitleRegression 覆盖用户提供的混合标题和标题数字。
func TestShareEpisodeTitleRegression(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   int
	}{{"搜查班长1958", 216292}, {"财阀X刑警", 220074}} {
		raw := tc.name + " (2024)/Season 1/" + tc.name + " (2024) S01E03-单集标题 [tmdbid=" + fmt.Sprint(tc.id) + "].mkv"
		q := AnalyzeShareFilename(raw)
		if q.Year != 2024 || q.MediaType != "tv" || q.TmdbID != tc.id || q.Titles[0] != tc.name {
			t.Fatalf("%+v", q)
		}
	}
}

// TestShareExplicitID 优先使用文件内ID，不能进入模糊搜索。
func TestShareExplicitID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/216292" {
			t.Errorf("意外请求 %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"id": 216292, "name": "搜查班长1958", "first_air_date": "2024-04-19"})
	}))
	defer server.Close()
	s := NewTmdbService("test", nil)
	s.baseURL = server.URL
	got, err := s.IdentifyShareFile(context.Background(), "搜查班长1958 (2024) S01E03 [tmdbid=216292].mkv", "auto", "auto")
	if err != nil || !got.Success || got.TmdbID != 216292 || got.Year != 2024 {
		t.Fatal(got, err)
	}
}

// TestShareOfficialAlias 官方别名可匹配，但返回的是正式标题。
func TestShareOfficialAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "alternative_titles") {
			w.Write([]byte(`{"results":[{"title":"闪烁的西瓜"}]}`))
			return
		}
		w.Write([]byte(`{"results":[{"id":218230,"name":"闪亮的西瓜","original_name":"반짝이는 워터멜론","first_air_date":"2023-09-25"}]}`))
	}))
	defer server.Close()
	s := NewTmdbService("test", nil)
	s.baseURL = server.URL
	got, err := s.IdentifyShareFile(context.Background(), "闪烁的西瓜 (2023)/Season 1/闪烁的西瓜 - S01E16.mkv", "auto", "auto")
	if err != nil || !got.Success || got.Title != "闪亮的西瓜" {
		t.Fatal(got, err)
	}
}

// TestShareAIOnlyAfterRules 复杂标题和未知类型也应优先常规查询，失败后最多调用AI一次。
func TestShareAIOnlyAfterRules(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "chat/completions") {
				calls++
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"title":"正确片名","year":2024,"media_type":"movie"}`}}}})
				return
			}
			if r.URL.Path == "/search/movie" && (!fallback || r.URL.Query().Get("query") == "正确片名") {
				title := "测试电影"
				if fallback {
					title = "正确片名"
				}
				json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"id": 9321, "title": title, "release_date": "2024-01-01"}}})
				return
			}
			w.Write([]byte(`{"results":[]}`))
		}))
		ai := NewAIRecognitionService(&aiMemoryStore{}, server.Client())
		cfg := defaultAIConfig()
		cfg.Enabled = true
		cfg.BaseURL = server.URL
		cfg.APIKey = "test"
		cfg.Model = "test"
		cfg.Scenes = []string{"complex_title", "uncertain_type", "no_match"}
		if err := ai.Save(cfg); err != nil {
			t.Fatal(err)
		}
		svc := NewTmdbService("test", nil)
		svc.baseURL = server.URL
		svc.SetAIRecognitionService(ai)
		got, err := svc.IdentifyShareFile(context.Background(), "[测试电影] (2024) [Blu-ray].mkv", "auto", "auto")
		want := 0
		if fallback {
			want = 1
		}
		if err != nil || !got.Success || calls != want {
			t.Fatalf("fallback=%v result=%+v err=%v AI=%d", fallback, got, err, calls)
		}
		server.Close()
	}
}

// TestShareDirectoryYearFallback 目录年份与TMDB不一致时重查，禁止同名多项自动认领。
func TestShareDirectoryYearFallback(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("first_air_date_year") != "" {
				w.Write([]byte(`{"results":[]}`))
				return
			}
			items := []map[string]any{{"id": 130103, "name": "飞起来吧蝴蝶", "first_air_date": "2026-01-01"}}
			if ambiguous {
				items = append(items, map[string]any{"id": 999, "name": "飞起来吧蝴蝶", "first_air_date": "2020-01-01"})
			}
			json.NewEncoder(w).Encode(map[string]any{"results": items})
		}))
		svc := NewTmdbService("test", nil)
		svc.baseURL = server.URL
		raw := `日韩剧集/2022/飞起来吧蝴蝶 (2022)/Season 1/飞起来吧蝴蝶 - S01E03 .mkv`
		q := AnalyzeShareFilename(raw)
		if q.Year != 2022 || !q.YearFromDirectory {
			t.Fatal(q)
		}
		got, err := svc.IdentifyShareFile(context.Background(), raw, "auto", "auto")
		if err != nil || got.Success == ambiguous {
			t.Fatal(got, err)
		}
		server.Close()
	}
}
