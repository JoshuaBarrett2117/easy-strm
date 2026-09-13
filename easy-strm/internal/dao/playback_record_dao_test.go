package dao

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestPlaybackMetadataAccountScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := DB
	DB = db
	defer func() { DB = previous }()
	mock.ExpectQuery("SELECT f.file_name").WithArgs(3, "pick").WillReturnRows(sqlmock.NewRows([]string{"file_name"}).AddRow("电影.mkv"))
	mock.ExpectQuery("SELECT i.title").WithArgs(FileHash("电影.mkv"), FileHash("tmdb:电影.mkv"), FileHash("metatube:电影.mkv"), "电影.mkv", 3).WillReturnRows(sqlmock.NewRows([]string{"title", "poster_path", "tmdb_id", "media_type"}).AddRow("电影", "/poster.jpg", 10, "movie"))
	title, poster, err := NewPlaybackRecordDAO(nil).Metadata(context.Background(), 3, "pick", "")
	if err != nil || title != "电影" || poster != "/poster.jpg" {
		t.Fatalf("metadata: %s %s %v", title, poster, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPlaybackMetadataFallsBackToTmdbCacheForLegacyEmptyPoster(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := DB
	DB = db
	defer func() { DB = previous }()
	mock.ExpectQuery("SELECT f.file_name").WithArgs(3, "pick").WillReturnRows(sqlmock.NewRows([]string{"file_name"}).AddRow("旧影片.mkv"))
	mock.ExpectQuery("SELECT i.title").WithArgs(FileHash("旧影片.mkv"), FileHash("tmdb:旧影片.mkv"), FileHash("metatube:旧影片.mkv"), "旧影片.mkv", 3).WillReturnRows(sqlmock.NewRows([]string{"title", "poster_path", "tmdb_id", "media_type"}).AddRow("旧影片", "", 20, "movie"))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(20), "movie", "旧影片").WillReturnRows(sqlmock.NewRows([]string{"title", "poster_path"}).AddRow("旧影片", "/legacy.jpg"))
	title, poster, err := NewPlaybackRecordDAO(nil).Metadata(context.Background(), 3, "pick", "")
	if err != nil || title != "旧影片" || poster != "/legacy.jpg" {
		t.Fatalf("legacy metadata: %s %s %v", title, poster, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSharePlaybackMetadataUsesEntrySnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := DB
	DB = db
	defer func() { DB = previous }()
	mock.ExpectQuery("SELECT\\s+COALESCE").WithArgs("entry-1").
		WillReturnRows(sqlmock.NewRows([]string{"title", "poster_path"}).AddRow("分享影片", "/share.jpg"))
	title, poster, err := NewPlaybackRecordDAO(nil).ShareMetadata(context.Background(), "entry-1")
	if err != nil || title != "分享影片" || poster != "/share.jpg" {
		t.Fatalf("share metadata: %s %s %v", title, poster, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
