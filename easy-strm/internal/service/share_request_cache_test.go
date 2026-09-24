package service

import (
	"context"
	"easy-strm/internal/dao"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestShareAliasCacheAcrossTasksAndForceRefresh(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	var aliases atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "alternative_titles") {
			aliases.Add(1)
			fmt.Fprint(w, `{"results":[{"title":"Alias"}]}`)
		} else {
			fmt.Fprint(w, `{"results":[{"id":1,"name":"Original","first_air_date":"2020-01-01"}]}`)
		}
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	for i := 0; i < 3; i++ {
		ctx := withShareWorkRound(context.Background(), i == 2)
		r, err := s.searchShareQuery(ctx, ShareMediaQuery{Titles: []string{"Alias"}, Year: 2020, MediaType: "tv"}, "tmdb")
		if err != nil || r == nil {
			t.Fatalf("%+v %v", r, err)
		}
		want := int32(1)
		if i == 2 {
			want = 2
		}
		if aliases.Load() != want {
			t.Fatalf("round=%d aliases=%d want=%d", i, aliases.Load(), want)
		}
	}
}

func TestShareRoundRequestsCoalesce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{"results":[]}`) }))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	ctx := WithRecognitionRound(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.searchTVContext(ctx, "Unique", 2020); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("requests=%d", calls.Load())
	}
}

func TestShareAmbiguousCandidatesStopBeforeAliases(t *testing.T) {
	var aliases atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "alternative_titles") {
			aliases.Add(1)
			fmt.Fprint(w, `{"results":[]}`)
		} else {
			fmt.Fprint(w, `{"results":[{"id":1,"name":"Same","first_air_date":"2020-01-01"},{"id":2,"name":"Same","first_air_date":"2020-01-01"}]}`)
		}
	}))
	defer server.Close()
	s := NewTmdbService("fixture", nil)
	s.baseURL = server.URL
	s.httpClient = server.Client()
	got, err := s.searchShareQuery(WithRecognitionRound(context.Background()), ShareMediaQuery{Titles: []string{"Same", "Extra"}, Year: 2020, MediaType: "tv"}, "tmdb")
	if err != nil || got != nil || aliases.Load() != 0 {
		t.Fatalf("result=%+v err=%v aliases=%d", got, err, aliases.Load())
	}
}
