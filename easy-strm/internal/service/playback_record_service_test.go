package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type playbackStoreStub struct {
	mu       sync.Mutex
	records  []domain.PlaybackRecord
	err      error
	saveErr  error
	location string
}

func (s *playbackStoreStub) UpdatePoster(_ context.Context, id, poster string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].Poster = poster
		}
	}
	return nil
}

func (s *playbackStoreStub) Save(_ context.Context, r domain.PlaybackRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.records = append(s.records, r)
	return nil
}
func (s *playbackStoreStub) List(context.Context) ([]domain.PlaybackRecord, error) {
	return s.records, s.err
}
func (s *playbackStoreStub) Metadata(_ context.Context, _ int, _ string, name string) (domain.PlaybackMetadata, error) {
	return domain.PlaybackMetadata{Title: name, Poster: "/poster.jpg"}, nil
}
func (s *playbackStoreStub) ShareMetadata(_ context.Context, _ string) (domain.PlaybackMetadata, error) {
	return domain.PlaybackMetadata{Title: "分享影片", Poster: "/share-poster.jpg"}, nil
}

func TestPlaybackDisplayNameEpisodes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		episodes []domain.ShareEpisode
		want     string
	}{
		{"电影", nil, "电影"},
		{"龙珠", []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}}, "龙珠 · 第 2 季 · 第 3 集"},
		{"特别篇", []domain.ShareEpisode{{SeasonNumber: 0, EpisodeNumber: 1}}, "特别篇 · 第 0 季 · 第 1 集"},
		{"合集", []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 2}, {SeasonNumber: 1, EpisodeNumber: 3}, {SeasonNumber: 1, EpisodeNumber: 2}}, "合集 · 第 1 季 · 第 2 集 · 第 1 季 · 第 3 集"},
		{"未知", []domain.ShareEpisode{{SeasonNumber: -1, EpisodeNumber: 3}, {SeasonNumber: 1, EpisodeNumber: 0}}, "未知"},
	} {
		if got := playbackDisplayName(domain.PlaybackMetadata{Title: tt.name, Episodes: tt.episodes}); got != tt.want {
			t.Fatalf("got %q, want %q", got, tt.want)
		}
	}
}
func (s *playbackStoreStub) GetLocation(context.Context, string) string { return s.location }
func (s *playbackStoreStub) SetLocation(_ context.Context, _, location string) error {
	s.location = location
	return nil
}

func TestSharePlaybackRecordUsesExportMetadata(t *testing.T) {
	store := &playbackStoreStub{}
	s := NewPlaybackRecordService(store)
	s.RecordShare("entry-1", "https://cdn.test/share", "192.168.1.2", "GET")
	s.RecordShare("entry-1", "https://cdn.test/share-refreshed", "192.168.1.2", "HEAD")
	if len(store.records) != 1 {
		t.Fatalf("分享播放会话记录数量 = %d，期望 1", len(store.records))
	}
	record := store.records[0]
	if record.Name != "分享影片" || record.Poster != "https://image.tmdb.org/t/p/w342/share-poster.jpg" || record.Method != "GET" {
		t.Fatalf("分享播放记录错误: %+v", record)
	}
}

func TestPlaybackRecordAndPagination(t *testing.T) {
	store := &playbackStoreStub{}
	s := NewPlaybackRecordService(store)
	s.Record("/电影/movie.mkv", "pick", 3, "https://cdn.test/a", "127.0.0.1", "HEAD")
	r := store.records[0]
	if r.Name != "movie.mkv" || r.Method != "HEAD" || r.ID == "" || r.Time.IsZero() || r.Poster != "https://image.tmdb.org/t/p/w342/poster.jpg" {
		t.Fatalf("记录错误: %+v", r)
	}
	rows, total, err := s.List(context.Background(), 0, 20)
	if err != nil || total != 1 || rows[0].Location != "内网" {
		t.Fatal(rows, total, err)
	}
	rows, _, err = s.List(context.Background(), 5, 20)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	store.err = errors.New("offline")
	if _, _, err = s.List(context.Background(), 0, 20); err == nil {
		t.Fatal("应返回错误")
	}
}

func TestPlaybackRecordMergesRepeatedResolutionsIntoSession(t *testing.T) {
	store := &playbackStoreStub{}
	s := NewPlaybackRecordService(store)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/first", "172.17.0.3", "HEAD")
	now = now.Add(time.Minute)
	s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/retry", "172.17.0.3", "GET")
	if len(store.records) != 1 {
		t.Fatalf("同一播放会话记录数 = %d，期望 1", len(store.records))
	}

	// 不同文件、不同来源仍是独立播放；静默超过五分钟后重播也会新增记录。
	s.Record("/动画/龙珠2.mkv", "pick-dragon-ball-2", 3, "https://cdn.test/other", "172.17.0.3", "GET")
	s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/device", "172.17.0.4", "GET")
	now = now.Add(playbackSessionIdleTimeout + time.Second)
	s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/replay", "172.17.0.3", "GET")
	if len(store.records) != 4 {
		t.Fatalf("独立播放记录数 = %d，期望 4", len(store.records))
	}
}

func TestPlaybackRecordConcurrentResolutionOnlySavesOnce(t *testing.T) {
	store := &playbackStoreStub{}
	s := NewPlaybackRecordService(store)
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/video", "172.17.0.3", "GET")
		}()
	}
	wait.Wait()
	if len(store.records) != 1 {
		t.Fatalf("并发解析记录数 = %d，期望 1", len(store.records))
	}
}

func TestPlaybackRecordSaveFailureCanRetry(t *testing.T) {
	store := &playbackStoreStub{saveErr: errors.New("offline")}
	s := NewPlaybackRecordService(store)
	s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/video", "172.17.0.3", "GET")
	store.saveErr = nil
	s.Record("/动画/龙珠.mkv", "pick-dragon-ball", 3, "https://cdn.test/video", "172.17.0.3", "GET")
	if len(store.records) != 1 {
		t.Fatalf("保存失败重试后的记录数 = %d，期望 1", len(store.records))
	}
}

type playbackTransport func(*http.Request) (*http.Response, error)

func (f playbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPlaybackGeoLookupAndFallback(t *testing.T) {
	store := &playbackStoreStub{}
	s := NewPlaybackRecordService(store)
	s.client = &http.Client{Transport: playbackTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		w.WriteString(`{"success":true,"country":"中国","region":"广东","city":"深圳"}`)
		return w.Result(), nil
	})}
	if got := s.location(context.Background(), "1.1.1.1"); got != "中国 广东 深圳" {
		t.Fatal(got)
	}
	store.location = ""
	s.client.Transport = playbackTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if got := s.location(context.Background(), "1.1.1.1"); got != "未知" {
		t.Fatal(got)
	}
}
