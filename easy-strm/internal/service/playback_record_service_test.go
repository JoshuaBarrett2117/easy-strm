package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type playbackStoreStub struct {
	records  []domain.PlaybackRecord
	err      error
	location string
}

func (s *playbackStoreStub) Save(_ context.Context, r domain.PlaybackRecord) error {
	s.records = append(s.records, r)
	return nil
}
func (s *playbackStoreStub) List(context.Context) ([]domain.PlaybackRecord, error) {
	return s.records, s.err
}
func (s *playbackStoreStub) Metadata(_ context.Context, _ int, _ string, name string) (string, string, error) {
	return name, "/poster.jpg", nil
}
func (s *playbackStoreStub) GetLocation(context.Context, string) string { return s.location }
func (s *playbackStoreStub) SetLocation(_ context.Context, _, location string) error {
	s.location = location
	return nil
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
