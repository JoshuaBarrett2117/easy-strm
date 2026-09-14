package service

import (
	"context"
	"easy-strm/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlaybackPosterCompletesInBackground(t *testing.T) {
	store := &playbackStoreStub{records: []domain.PlaybackRecord{{ID: "r1", Name: "Movie.2025.mkv"}}}
	s := NewPlaybackRecordService(store)
	started, release := make(chan struct{}), make(chan struct{})
	s.SetPosterResolver(func(context.Context, string) (string, error) { close(started); <-release; return "/movie.jpg", nil })
	s.completePoster(store.records[0])
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("补全未启动")
	}
	s.completePoster(store.records[0])
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		poster := store.records[0].Poster
		store.mu.Unlock()
		if poster == "https://image.tmdb.org/t/p/w342/movie.jpg" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("海报未持久化")
}

func TestPlaybackMoviePosterMatching(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"id":822119,"title":"美国队长4","original_title":"Captain America: Brave New World","release_date":"2025-02-12","poster_path":"/captain.jpg"},{"id":1771,"title":"美国队长","original_title":"Captain America","release_date":"2011-07-22","poster_path":"/wrong.jpg"}]}`))
	}))
	defer server.Close()
	s := NewTmdbService("test", nil)
	s.baseURL = server.URL
	poster, err := s.ResolvePlaybackMoviePoster(context.Background(), "美国队长4.Captain America： Brave New World.2025.2160p.mkv")
	if err != nil || poster == "" {
		t.Fatalf("poster=%q err=%v", poster, err)
	}
	poster, err = s.ResolvePlaybackMoviePoster(context.Background(), "无关电影.2025.mkv")
	if err != nil || poster != "" {
		t.Fatalf("不能匹配无关电影: %q %v", poster, err)
	}
	poster, err = s.ResolvePlaybackMoviePoster(context.Background(), "龙珠.S01E08.mkv")
	if err != nil || poster != "" {
		t.Fatalf("不能按电影识别剧集: %q %v", poster, err)
	}
}
