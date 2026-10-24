package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

// 主标题命中的候选优先于仅靠官方别名命中的候选：
// 《邪恶力量》(tmdb 1622, 2005) 与一部无关剧集(别名同为 Supernatural) 同时命中时，
// 不应因为别名撞名就判成"身份歧义"。
func TestSelectShareTVCandidatePrefersPrimaryTitleOverAliasOnly(t *testing.T) {
	s := newShareTVTestService(t)

	q := ShareMediaQuery{
		MediaType: "tv",
		Titles:    []string{"Supernatural"},
		Year:      2009,
		Season:    3,
		Evidence:  &shareRecognitionEvidence{},
	}
	q.fileYear = 2007
	q.SeasonYear = 2007

	candidates := []domain.TmdbSearchResult{
		{TmdbID: 1622, Title: "邪恶力量", OriginalTitle: "Supernatural", Year: 2005, MediaType: "tv"},
		{TmdbID: 2967, Title: "Unfabulous", OriginalTitle: "Unfabulous", Year: 2004, MediaType: "tv"},
	}

	ctx := WithRecognitionRound(context.Background())
	best, err := s.selectShareTVCandidate(ctx, q, candidates)
	if err != nil {
		t.Fatalf("识别出错: %v", err)
	}
	if best == nil {
		t.Fatalf("应命中 1622，实际判为歧义或无数候选")
	}
	if best.TmdbID != 1622 {
		t.Fatalf("应命中 1622，实际 %d %s", best.TmdbID, best.Title)
	}
	if q.Evidence != nil && q.Evidence.ambiguous {
		t.Fatalf("不应记录为歧义")
	}
}

// 只有别名候选可用时仍要能用别名命中，不能因为分级把唯一合法候选丢掉。
func TestSelectShareTVCandidateFallsBackToAliasOnly(t *testing.T) {
	s := newShareTVTestService(t)

	q := ShareMediaQuery{MediaType: "tv", Titles: []string{"Supernatural"}, Year: 2005, Season: 1, Evidence: &shareRecognitionEvidence{}}
	q.fileYear = 2006

	candidates := []domain.TmdbSearchResult{
		{TmdbID: 2967, Title: "Unfabulous", OriginalTitle: "Unfabulous", Year: 2004, MediaType: "tv"},
	}

	ctx := WithRecognitionRound(context.Background())
	best, err := s.selectShareTVCandidate(ctx, q, candidates)
	if err != nil {
		t.Fatalf("识别出错: %v", err)
	}
	if best == nil || best.TmdbID != 2967 {
		t.Fatalf("别名兜底失败，期望 2967")
	}
}

// 两个主标题候选都通过核验时仍必须保留歧义，不能自动挑一个。
func TestSelectShareTVCandidateKeepsPrimaryAmbiguity(t *testing.T) {
	s := newShareTVTestService(t)

	q := ShareMediaQuery{MediaType: "tv", Titles: []string{"Supernatural"}, Year: 2005, Season: 1, Evidence: &shareRecognitionEvidence{}}
	q.fileYear = 2005

	candidates := []domain.TmdbSearchResult{
		{TmdbID: 1622, Title: "邪恶力量", OriginalTitle: "Supernatural", Year: 2005, MediaType: "tv"},
		{TmdbID: 13836, Title: "超自然故事集", OriginalTitle: "Supernatural", Year: 2005, MediaType: "tv"},
	}

	ctx := WithRecognitionRound(context.Background())
	best, err := s.selectShareTVCandidate(ctx, q, candidates)
	if err != nil {
		t.Fatalf("识别出错: %v", err)
	}
	if best != nil {
		t.Fatalf("同名同年两部作品应保留歧义，实际命中 %d", best.TmdbID)
	}
	if q.Evidence == nil || !q.Evidence.ambiguous {
		t.Fatalf("应记录歧义")
	}
}

// newShareTVTestService 构造只访问本地 httptest 的 TMDB 服务。
// 1622：2005-2020 播出，15 季；2967：2004-2007 播出，4 季，别名包含 Supernatural；
// 13836：2005 单年，1 季。
func newShareTVTestService(t *testing.T) *TmdbService {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "/alternative_titles"):
			switch {
			case strings.Contains(path, "/tv/2967/"):
				fmt.Fprint(w, `{"id":2967,"titles":[{"iso_3166_1":"US","title":"Supernatural"},{"iso_3166_1":"US","title":"Super Natural"}]}`)
			default:
				fmt.Fprint(w, `{"id":0,"titles":[]}`)
			}
		case strings.Contains(path, "/season/"):
			fmt.Fprint(w, `{"season_number":3,"air_date":"2007-10-04"}`)
		case strings.Contains(path, "/tv/1622"):
			fmt.Fprint(w, `{"id":1622,"name":"邪恶力量","original_name":"Supernatural","first_air_date":"2005-09-13","last_air_date":"2020-11-19","seasons":[{"season_number":1,"air_date":"2005-09-13","name":"第 1 季"},{"season_number":3,"air_date":"2007-10-04","name":"第 3 季"}]}`)
		case strings.Contains(path, "/tv/2967"):
			fmt.Fprint(w, `{"id":2967,"name":"Unfabulous","original_name":"Unfabulous","first_air_date":"2004-09-12","last_air_date":"2007-12-16","seasons":[{"season_number":1,"air_date":"2004-09-12","name":"第 1 季"},{"season_number":3,"air_date":"2006-10-07","name":"第 3 季"}]}`)
		case strings.Contains(path, "/tv/13836"):
			fmt.Fprint(w, `{"id":13836,"name":"超自然故事集","original_name":"Supernatural","first_air_date":"2005-06-11","last_air_date":"2005-08-06","seasons":[{"season_number":1,"air_date":"2005-06-11","name":"第 1 季"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"status_code":34,"status_message":"not found"}`)
		}
	}))
	t.Cleanup(srv.Close)

	s := NewTmdbService("test-key", nil)
	s.baseURL = srv.URL
	s.language = "zh-CN"
	s.httpClient = &http.Client{Timeout: 5 * time.Second}
	return s
}
