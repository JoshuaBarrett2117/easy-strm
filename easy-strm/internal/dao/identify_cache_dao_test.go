package dao

import (
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestIdentifyCacheDAOCreateOrUpdate(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewIdentifyCacheDAO()
	cache := &IdentifyCache{
		FileHash:          "hash-1",
		FileName:          "Breaking.Bad.S01E01.mkv",
		MediaType:         "tv",
		TmdbID:            1396,
		Title:             "Breaking Bad",
		OriginalTitle:     "Breaking Bad",
		Year:              2008,
		SeasonNumber:      1,
		EpisodeNumber:     1,
		PosterPath:        "/poster.jpg",
		IsManual:          true,
		SourceID:          77,
		RecognitionMethod: "manual",
		MetadataSource:    "tmdb",
	}
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_identify_cache (file_hash, file_name, media_type, tmdb_id, title, original_title, year,
		        season_number, episode_number, poster_path, is_manual, source_id, recognition_method,
		        metadata_source, metadata_id, metadata_provider, ai_used, ai_scene, failure_reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		 ON CONFLICT (file_hash) DO UPDATE SET
		        file_name = EXCLUDED.file_name,
		        media_type = EXCLUDED.media_type,
		        tmdb_id = EXCLUDED.tmdb_id,
		        title = EXCLUDED.title,
		        original_title = EXCLUDED.original_title,
		        year = EXCLUDED.year,
		        season_number = EXCLUDED.season_number,
		        episode_number = EXCLUDED.episode_number,
		        poster_path = EXCLUDED.poster_path,
		        is_manual = EXCLUDED.is_manual,
		        source_id = COALESCE(t_identify_cache.source_id, EXCLUDED.source_id),
		        recognition_method = EXCLUDED.recognition_method,
		        metadata_source = EXCLUDED.metadata_source,
		        metadata_id = EXCLUDED.metadata_id,
		        metadata_provider = EXCLUDED.metadata_provider,
		        ai_used = EXCLUDED.ai_used,
		        ai_scene = EXCLUDED.ai_scene,
		        failure_reason = EXCLUDED.failure_reason,
		        updated_at = NOW()
		 WHERE NOT t_identify_cache.is_manual OR EXCLUDED.is_manual
		 RETURNING id, created_at, updated_at`)).
		WithArgs(
			"hash-1",
			"Breaking.Bad.S01E01.mkv",
			"tv",
			1396,
			"Breaking Bad",
			"Breaking Bad",
			2008,
			1,
			1,
			"/poster.jpg",
			true,
			77,
			"manual",
			"tmdb",
			"",
			"",
			false,
			"",
			"",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(5, now, now))

	if err := dao.CreateOrUpdate(cache); err != nil {
		t.Fatalf("expected create or update to succeed: %v", err)
	}
	if cache.ID != 5 {
		t.Fatalf("expected returned id 5, got %d", cache.ID)
	}
	if cache.CreatedAt.IsZero() || cache.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be hydrated, got created=%v updated=%v", cache.CreatedAt, cache.UpdatedAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
