package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestShareRomanSeasonParsing(t *testing.T) {
	s := NewTmdbService("fixture", nil)
	for _, tc := range []struct {
		name            string
		season, episode int
	}{
		{"妙手仁心Ⅰ-31.mkv", 1, 31}, {"妙手仁心Ⅱ-01.mkv", 2, 1}, {"妙手仁心III-02.mkv", 3, 2},
	} {
		p := s.ParseFilename(tc.name)
		q := AnalyzeShareFilename(tc.name)
		if p.Title != "妙手仁心" || p.Season != tc.season || p.Episode != tc.episode || q.MediaType != "tv" || len(q.Titles) != 1 || q.Titles[0] != "妙手仁心" {
			t.Fatalf("%s: parsed=%+v query=%+v", tc.name, p, q)
		}
	}
	if q := AnalyzeShareFilename("Apollo-13.mkv"); q.MediaType == "tv" {
		t.Fatalf("普通数字标题误判: %+v", q)
	}
}

func TestShareWorkReusesIdentityAcrossSeasons(t *testing.T) {
	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search/") {
			searches.Add(1)
			if r.URL.Path != "/search/tv" || r.URL.Query().Get("query") != "妙手仁心" {
				t.Errorf("错误的搜索: %s", r.URL)
			}
			fmt.Fprint(w, `{"results":[{"id":20464,"name":"妙手仁心","original_name":"Healing Hands","first_air_date":"1998-08-31"}]}`)
			return
		}
		fmt.Fprint(w, `{"id":20464,"name":"妙手仁心","original_name":"Healing Hands","first_air_date":"1998-08-31","original_language":"zh","origin_country":["HK"],"genres":[{"id":18}],"vote_average":8,"seasons":[{"season_number":1},{"season_number":2}]}`)
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	ctx := WithRecognitionRound(context.Background())
	for _, name := range []string{"合集/妙手仁心Ⅰ-31.mkv", "合集/妙手仁心Ⅰ-32.mkv", "合集/妙手仁心Ⅱ-01.mkv"} {
		r, err := s.IdentifyShareFile(ctx, name, "tmdb", "auto")
		if err != nil || !r.Success || r.TmdbID != 20464 {
			t.Fatalf("%s: %+v %v", name, r, err)
		}
	}
	if searches.Load() != 1 {
		t.Fatalf("同作品应只搜索一次，实际 %d", searches.Load())
	}
}

func TestShareWorkAIOnceAndKnownTypeCannotBeOverridden(t *testing.T) {
	var aiCalls, movieCalls, details atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "chat/completions"):
			aiCalls.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"title":"正确剧名","original_title":"Correct Show","year":2020,"media_type":"movie"}`}}}})
		case r.URL.Path == "/search/movie":
			movieCalls.Add(1)
			fmt.Fprint(w, `{"results":[]}`)
		case r.URL.Path == "/search/tv" && r.URL.Query().Get("query") == "正确剧名":
			fmt.Fprint(w, `{"results":[{"id":1,"name":"正确剧名","first_air_date":"2020-01-01"}]}`)
		case r.URL.Path == "/tv/1":
			details.Add(1)
			fmt.Fprint(w, `{"id":1,"seasons":[{"season_number":1}]}`)
		default:
			fmt.Fprint(w, `{"results":[]}`)
		}
	}))
	defer server.Close()
	ai := NewAIRecognitionService(&aiMemoryStore{}, server.Client())
	cfg := defaultAIConfig()
	cfg.Enabled = true
	cfg.BaseURL = server.URL
	cfg.APIKey = "fixture"
	cfg.Model = "fixture"
	cfg.Scenes = []string{"no_match", "uncertain_type", "complex_title"}
	if err := ai.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	s.SetAIRecognitionService(ai)
	ctx := WithRecognitionRound(context.Background())
	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		wg.Add(1)
		go func(ep int) {
			defer wg.Done()
			r, err := s.IdentifyShareFile(ctx, fmt.Sprintf("未知剧Ⅰ-%02d.mkv", ep), "tmdb", "auto")
			if err != nil || !r.Success || r.MediaType != "tv" || r.SeasonNumber != 1 || r.EpisodeNumber != ep {
				t.Errorf("%+v %v", r, err)
			}
		}(i)
	}
	wg.Wait()
	r, err := s.IdentifyShareFile(ctx, "未知剧Ⅱ-01.mkv", "tmdb", "auto")
	if err != nil || r.SeasonNumber != 2 || !r.Success || !strings.Contains(r.Message, "季信息待核对") {
		t.Fatalf("季缺失应保留作品身份及季号: %+v %v", r, err)
	}
	if aiCalls.Load() != 1 || movieCalls.Load() != 0 || details.Load() != 1 {
		t.Fatalf("AI=%d movie=%d details=%d", aiCalls.Load(), movieCalls.Load(), details.Load())
	}
}

func TestShareWorkFailuresAreScopedToRound(t *testing.T) {
	for _, networkFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(networkFailure), func(t *testing.T) {
			var calls, unexpected atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/search/tv" {
					unexpected.Add(1)
				}
				calls.Add(1)
				if networkFailure {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, `{"results":[]}`)
			}))
			defer server.Close()
			s := NewTmdbService("fixture", nil)
			s.baseURL = server.URL
			s.httpClient = server.Client()
			ctx := WithRecognitionRound(context.Background())
			for i := 1; i <= 2; i++ {
				r, err := s.IdentifyShareFile(ctx, fmt.Sprintf("无匹配Ⅰ-%02d.mkv", i), "tmdb", "auto")
				if networkFailure {
					if err == nil {
						t.Fatal("网络故障必须返回错误")
					}
				} else if err != nil || r.Success {
					t.Fatalf("%+v %v", r, err)
				}
			}
			want := int32(1)
			if networkFailure {
				want = 2
			}
			if calls.Load() != want || unexpected.Load() != 0 {
				t.Fatalf("搜索=%d 其他请求=%d", calls.Load(), unexpected.Load())
			}
		})
	}
}

func TestShareWorkPersistentCacheAndRetry(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/search/tv" {
			fmt.Fprint(w, `{"results":[{"id":1,"name":"测试剧","first_air_date":"2020-01-01"}]}`)
			return
		}
		fmt.Fprint(w, `{"id":1,"name":"测试剧","original_name":"Show","first_air_date":"2020-01-01","original_language":"zh","origin_country":["CN"],"genres":[{"id":18}],"vote_average":8,"seasons":[{"season_number":1},{"season_number":2}]}`)
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	first, err := s.IdentifyShareFile(context.Background(), "测试剧Ⅰ-01.mkv", "tmdb", "auto")
	if err != nil || !first.Success {
		t.Fatalf("%+v %v", first, err)
	}
	// 清除底层搜索和详情缓存，验证第二轮只依赖作品缓存。
	for _, key := range mini.Keys() {
		if strings.Contains(key, ":tmdb:") {
			mini.Del(key)
		}
	}
	count := calls.Load()
	next, err := s.IdentifyShareFile(context.Background(), "测试剧Ⅱ-02.mkv", "tmdb", "auto")
	if err != nil || !next.Success || next.SeasonNumber != 2 || next.EpisodeNumber != 2 || calls.Load() != count {
		t.Fatalf("作品缓存未复用: %+v %v calls=%d", next, err, calls.Load())
	}
	ctx := withShareWorkRound(context.Background(), true)
	if _, err = s.IdentifyShareFile(ctx, "测试剧Ⅰ-03.mkv", "tmdb", "auto"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() <= count {
		t.Fatal("显式重新识别必须重新核验")
	}
}

func TestShareWorkKeySeparatesConflicts(t *testing.T) {
	s := NewTmdbService("fixture", nil)
	ctx := context.Background()
	key := s.shareWorkKey(ctx, "合集/Show (2020) S01E01.mkv", "tmdb", "tv")
	for _, name := range []string{"另一个合集/Show (2020) S01E02.mkv", "合集/Show (2021) S01E02.mkv", "合集/Other (2020) S01E02.mkv"} {
		if key == s.shareWorkKey(ctx, name, "tmdb", "tv") {
			t.Fatal("冲突作品被合并:", name)
		}
	}
	if key != s.shareWorkKey(ctx, "合集/Show (2020) S02E02.mkv", "tmdb", "tv") {
		t.Fatal("季号不应拆分作品")
	}
}

func TestShareEpisodeContextRequiresEvidence(t *testing.T) {
	ctx := withShareWorkRound(context.Background(), false)
	records := []domain.ShareRecord{{ID: 7, MediaType: "auto", Media: []domain.ShareMedia{
		{FileName: "剧集/Show-01.mkv", Available: true}, {FileName: "剧集/Show-02.mkv", Available: true},
		{FileName: "电影/Apollo-13.mkv", Available: true},
	}}}
	prepareShareEpisodeInputs(ctx, records)
	ctx = context.WithValue(ctx, shareWorkScopeKey{}, 7)
	if got := shareEpisodeInput(ctx, "剧集/Show-02.mkv", "auto"); got != "剧集/Show S01E02.mkv" {
		t.Fatal(got)
	}
	if got := shareEpisodeInput(ctx, "电影/Apollo-13.mkv", "auto"); got != "电影/Apollo-13.mkv" {
		t.Fatal(got)
	}
	if got := shareEpisodeInput(ctx, "剧集/Show-02.mkv", "movie"); got != "剧集/Show-02.mkv" {
		t.Fatal(got)
	}
	if got := shareEpisodeInput(ctx, "Show/Season 2/Show-03.mkv", "auto"); got != "Show/Season 2/Show S02E03.mkv" {
		t.Fatal(got)
	}
}

func TestShareWorkSeedsSavedIdentityAndRejectsConflict(t *testing.T) {
	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/") {
			searches.Add(1)
		}
		fmt.Fprint(w, `{"id":20464,"seasons":[{"season_number":1},{"season_number":2}]}`)
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	records := []domain.ShareRecord{{ID: 7, MediaType: "auto", Media: []domain.ShareMedia{{Available: true, Status: "identified", FileName: "妙手仁心Ⅰ-01.mkv", MetadataSource: "tmdb", Result: &domain.TmdbIdentifyResult{Success: true, MediaType: "tv", TmdbID: 20464, Title: "妙手仁心", Year: 1998}}}}}
	ctx := withShareWorkRound(context.Background(), false)
	s.seedShareWorks(ctx, records)
	ctx = context.WithValue(ctx, shareWorkScopeKey{}, 7)
	got, err := s.IdentifyShareFile(ctx, "妙手仁心Ⅱ-01.mkv", "tmdb", "auto")
	if err != nil || !got.Success || got.TmdbID != 20464 || got.SeasonNumber != 2 || searches.Load() != 0 {
		t.Fatalf("历史身份未复用: %+v %v", got, err)
	}
	other := records[0].Media[0]
	other.Result = cloneShareWorkResult(other.Result)
	other.Result.TmdbID = 999
	records[0].Media = append(records[0].Media, other)
	conflict := withShareWorkRound(context.Background(), false)
	s.seedShareWorks(conflict, records)
	conflict = context.WithValue(conflict, shareWorkScopeKey{}, 7)
	got, err = s.IdentifyShareFile(conflict, "妙手仁心Ⅱ-02.mkv", "tmdb", "auto")
	if err != nil || got.Success || !strings.Contains(got.Message, "不同的已确认身份") {
		t.Fatalf("冲突应待确认: %+v %v", got, err)
	}
}

func TestShareWorkWaitingCallerCanCancel(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		fmt.Fprint(w, `{"results":[]}`)
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	ctx := WithRecognitionRound(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.IdentifyShareFile(ctx, "测试剧Ⅰ-01.mkv", "tmdb", "auto") }()
	<-started
	waiting, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { _, err := s.IdentifyShareFile(waiting, "测试剧Ⅰ-02.mkv", "tmdb", "auto"); result <- err }()
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Error("取消等待必须返回错误")
		}
	case <-time.After(time.Second):
		t.Error("等待同作品请求时无法取消")
	}
	close(release)
	<-done
}
