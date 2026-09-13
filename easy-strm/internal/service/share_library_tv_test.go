package service

import (
	"context"
	"easy-strm/internal/dao"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func newLibraryTVTestService(t *testing.T, handler http.HandlerFunc) (*ShareRecordService, sqlmock.Sqlmock, func()) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	tmdb := NewTmdbService("fixture", nil)
	tmdb.baseURL = server.URL
	tmdb.httpClient = server.Client()
	return NewShareRecordService(dao.NewShareRecordDAO(database), tmdb, nil, nil), mock, func() {
		server.Close()
		database.Close()
	}
}

func expectLibraryTVMedia(mock sqlmock.Sqlmock, mediaType string, tmdbID interface{}) {
	mock.ExpectQuery(`SELECT work_key,tmdb_id,title,media_type,metadata_source`).WithArgs("tmdb:tv:100").
		WillReturnRows(sqlmock.NewRows([]string{"work_key", "tmdb_id", "title", "media_type", "metadata_source"}).
			AddRow("tmdb:tv:100", tmdbID, "测试剧", mediaType, "tmdb"))
}

func TestLibraryTVDetailMergesCompleteTMDBSeasons(t *testing.T) {
	s, mock, closeTest := newLibraryTVTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/100" {
			t.Errorf("错误的 TMDB 路径: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "测试剧", "overview": "剧集简介",
			"seasons": []map[string]interface{}{
				{"season_number": 0, "name": "特别篇", "episode_count": 1},
				{"season_number": 1, "name": "第一季", "air_date": "2024-01-01", "episode_count": 3},
			},
		})
	})
	defer closeTest()
	expectLibraryTVMedia(mock, "tv", int64(100))
	mock.ExpectQuery(`SELECT e.season_number,count\(DISTINCT e.episode_number\)`).WithArgs("tmdb:tv:100").
		WillReturnRows(sqlmock.NewRows([]string{"season", "episodes", "files"}).AddRow(1, 2, 3).AddRow(2, 1, 1))
	detail, err := s.LibraryTVDetail(context.Background(), "tmdb:tv:100")
	if err != nil || !detail.MetadataComplete || len(detail.Seasons) != 3 || detail.Seasons[0].SeasonNumber != 0 || detail.Seasons[2].SeasonNumber != 2 {
		t.Fatalf("季目录合并错误: %+v %v", detail, err)
	}
	if detail.Seasons[1].EpisodeCount != 3 || detail.Seasons[1].MatchedEpisodeCount != 2 || detail.Seasons[2].Name != "第 2 季" {
		t.Fatalf("季覆盖统计错误: %+v", detail.Seasons)
	}
}

func TestLibraryTVSeasonMergesFilesAndUnknownEpisode(t *testing.T) {
	s, mock, closeTest := newLibraryTVTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/100/season/1" {
			t.Errorf("错误的 TMDB 路径: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "第一季", "episodes": []map[string]interface{}{
				{"episode_number": 1, "name": "首集", "air_date": "2024-01-01"},
				{"episode_number": 3, "name": "第三集"},
			},
		})
	})
	defer closeTest()
	expectLibraryTVMedia(mock, "tv", int64(100))
	mock.ExpectQuery(`SELECT e.episode_number,f.id,f.share_id`).WithArgs("tmdb:tv:100", 1).
		WillReturnRows(sqlmock.NewRows([]string{"episode", "id", "share_id", "remote", "file", "size", "available", "status", "name", "url", "password", "cancelled"}).
			AddRow(1, 10, 7, "r10", "Show.S01E01E02.mkv", 1, true, "identified", "来源一", "https://example.com/1", "", false).
			AddRow(1, 11, 8, "r11", "Show.S01E01.mkv", 1, true, "identified", "来源二", "https://example.com/2", "", false).
			AddRow(2, 10, 7, "r10", "Show.S01E01E02.mkv", 1, true, "identified", "来源一", "https://example.com/1", "", false))
	detail, err := s.LibraryTVSeason(context.Background(), "tmdb:tv:100", 1)
	if err != nil || !detail.MetadataComplete || len(detail.Episodes) != 3 {
		t.Fatalf("分集目录合并错误: %+v %v", detail, err)
	}
	if len(detail.Episodes[0].Files) != 2 || detail.Episodes[1].Name != "第 2 集" || len(detail.Episodes[2].Files) != 0 {
		t.Fatalf("文件映射或占位集错误: %+v", detail.Episodes)
	}
}

func TestLibraryTVDetailFallsBackAndRejectsMovie(t *testing.T) {
	s, mock, closeTest := newLibraryTVTestService(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	defer closeTest()
	expectLibraryTVMedia(mock, "tv", int64(100))
	mock.ExpectQuery(`SELECT e.season_number,count\(DISTINCT e.episode_number\)`).WithArgs("tmdb:tv:100").
		WillReturnRows(sqlmock.NewRows([]string{"season", "episodes", "files"}).AddRow(1, 1, 1))
	detail, err := s.LibraryTVDetail(context.Background(), "tmdb:tv:100")
	if err != nil || detail.MetadataComplete || len(detail.Seasons) != 1 || !strings.Contains(detail.Warning, "TMDB") {
		t.Fatalf("降级结果错误: %+v %v", detail, err)
	}
	expectLibraryTVMedia(mock, "movie", int64(100))
	_, err = s.LibraryTVDetail(context.Background(), "tmdb:tv:100")
	if !errors.Is(err, ErrLibraryNotTV) {
		t.Fatalf("电影应被拒绝: %v", err)
	}
}

func TestGetTVSeasonDetailUsesDetailCache(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	dao.InitTaskRedisDAO(client)
	defer dao.InitTaskRedisDAO(nil)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": "第一季", "episodes": []interface{}{}})
	}))
	defer server.Close()
	tmdb := NewTmdbService("fixture", nil)
	tmdb.baseURL = server.URL
	tmdb.httpClient = server.Client()
	for i := 0; i < 2; i++ {
		if _, err := tmdb.GetTVSeasonDetail(100, 1); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("单季详情应命中缓存，实际请求 %d 次", calls.Load())
	}
}
