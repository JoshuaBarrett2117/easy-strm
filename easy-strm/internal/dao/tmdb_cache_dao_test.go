package dao

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestTmdbCacheDAOGetByTmdbID(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewTmdbCacheDAO()
	now := time.Now()
	rawData, err := json.Marshal(map[string]interface{}{
		"genre_ids":            []int{16},
		"production_countries": []map[string]string{{"iso_3166_1": "JP"}},
		"original_language":    "ja",
	})
	if err != nil {
		t.Fatalf("marshal raw data failed: %v", err)
	}

	rows := sqlmock.NewRows([]string{
		"id", "query_key", "media_type", "tmdb_id", "title", "original_title", "year", "poster_path",
		"overview", "vote_average", "release_date", "first_air_date", "season_number", "episode_number",
		"raw_data", "expire_at", "create_time", "update_time",
	}).AddRow(
		1, "doraemon.mp4", "movie", 10515, "Doraemon", "Doraemon the Movie", 1980, "/poster.jpg",
		"overview", 7.5, "1980-03-15", nil, nil, nil, rawData, now.Add(24*time.Hour), now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path,
		        overview, vote_average, release_date, first_air_date, season_number, episode_number,
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache
		 WHERE tmdb_id = $1 AND media_type = $2 AND expire_at > NOW()
		 ORDER BY update_time DESC, id DESC
		 LIMIT 1`)).
		WithArgs(10515, "movie").
		WillReturnRows(rows)

	cache, err := dao.GetByTmdbID(10515, "movie")
	if err != nil {
		t.Fatalf("expected get by tmdb id to succeed: %v", err)
	}
	if cache == nil {
		t.Fatal("expected cache result, got nil")
	}
	if cache.TmdbID != 10515 || cache.MediaType != "movie" {
		t.Fatalf("unexpected cache payload: %+v", cache)
	}
	if len(cache.RawData) == 0 {
		t.Fatal("expected raw data to be present")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
