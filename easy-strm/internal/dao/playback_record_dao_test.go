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
	mock.ExpectQuery("SELECT title").WithArgs(FileHash("电影.mkv")).WillReturnRows(sqlmock.NewRows([]string{"title", "poster_path"}).AddRow("电影", "/poster.jpg"))
	title, poster, err := NewPlaybackRecordDAO(nil).Metadata(context.Background(), 3, "pick", "")
	if err != nil || title != "电影" || poster != "/poster.jpg" {
		t.Fatalf("metadata: %s %s %v", title, poster, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
