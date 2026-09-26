package dao

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestTmdbCacheDAOCreateReplacesConflictingCache(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	// 过期记录仍占用唯一键；同一键连续保存必须由数据库原子覆盖。
	query := `(?s)INSERT INTO t_tmdb_cache .*ON CONFLICT \(query_key, media_type\) DO UPDATE SET .*tmdb_id = EXCLUDED.tmdb_id,.*title = EXCLUDED.title,.*original_title = EXCLUDED.original_title,.*year = EXCLUDED.year,.*poster_path = EXCLUDED.poster_path,.*overview = EXCLUDED.overview,.*vote_average = EXCLUDED.vote_average,.*release_date = EXCLUDED.release_date,.*first_air_date = EXCLUDED.first_air_date,.*season_number = EXCLUDED.season_number,.*episode_number = EXCLUDED.episode_number,.*raw_data = EXCLUDED.raw_data,.*expire_at = EXCLUDED.expire_at,.*update_time = NOW\(\).*RETURNING id, create_time, update_time`
	created := time.Now().Add(-8 * 24 * time.Hour)
	for _, title := range []string{"首次选择", "再次修正"} {
		now := time.Now()
		cache := &TmdbCache{QueryKey: "metatube:file-1", MediaType: "movie", TmdbID: 123,
			Title: title, OriginalTitle: "original", Year: 2026, PosterPath: "https://example.com/poster.jpg",
			Overview: "overview", VoteAverage: 8, ReleaseDate: "2026-07-10",
			RawData: json.RawMessage(`{"metadata_source":"metatube","metadata_id":"DSOD-028"}`), ExpireAt: now.Add(7 * 24 * time.Hour)}
		mock.ExpectQuery(query).WithArgs(cache.QueryKey, cache.MediaType, cache.TmdbID, title,
			cache.OriginalTitle, cache.Year, cache.PosterPath, cache.Overview, cache.VoteAverage,
			cache.ReleaseDate, nil, nil, nil, cache.RawData, cache.ExpireAt).
			WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time"}).AddRow(42, created, now))
		if err := NewTmdbCacheDAO().Create(cache); err != nil {
			t.Fatalf("save conflicting cache: %v", err)
		}
		if cache.ID != 42 || !cache.CreateTime.Equal(created) || !cache.UpdateTime.Equal(now) {
			t.Fatalf("unexpected persisted identity/timestamps: %+v", cache)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTmdbCacheDAOCreateReturnsDatabaseError(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()
	mock.ExpectQuery("INSERT INTO t_tmdb_cache").WillReturnError(errors.New("database unavailable"))
	if err := NewTmdbCacheDAO().Create(&TmdbCache{}); err == nil {
		t.Fatal("expected database failure to propagate")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

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
