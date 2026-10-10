package dao

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCategoryPreparationBlockedQueryAndRowError(t *testing.T) {
	database, mock, _ := sqlmock.New()
	previous := DB
	DB = database
	defer func() { DB = previous; database.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	mock.ExpectQuery("SELECT id, name, media_type").WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "path", "rules", "enabled", "created", "updated"}))
	started := time.Now()
	if _, err := NewMediaCategoryDAO().GetAllContext(ctx); err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("category query cancellation: %v", err)
	}
	rows := sqlmock.NewRows([]string{"id", "name", "type", "path", "rules", "enabled", "created", "updated"}).AddRow(1, "movie", "movie", "/mock", nil, true, time.Now(), time.Now()).RowError(0, errors.New("row failed"))
	mock.ExpectQuery("SELECT id, name, media_type").WillReturnRows(rows)
	if _, err := NewMediaCategoryDAO().GetAllContext(context.Background()); err == nil {
		t.Fatal("swallowed category row failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
