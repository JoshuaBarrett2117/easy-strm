package dao

import (
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func setupMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	Init(db, nil)
	return mock, func() { _ = db.Close() }
}

func TestMediaSourceDAOCreate(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewMediaSourceDAO()
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", "local", `C:\\media`, nil, 10, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_media_source (name, source_type, path, cloud115_id, priority, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time`)).
		WithArgs("movies", "local", `C:\media`, nil, 10, true).
		WillReturnRows(rows)

	source, err := dao.Create("movies", "local", `C:\media`, nil, 10, true)
	if err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}
	if source.ID != 1 || source.Name != "movies" || source.SourceType != "local" {
		t.Fatalf("unexpected source: %+v", source)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceDAOGetEnabled(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewMediaSourceDAO()
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "name", "source_type", "path", "cloud115_id", "priority", "enabled", "create_time", "update_time"}).
		AddRow(1, "movies", "local", `C:\\movies`, nil, 10, true, now, now).
		AddRow(2, "shows", "local", `C:\\shows`, nil, 20, true, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, source_type, path, cloud115_id, priority, enabled, create_time, update_time
		FROM t_media_source WHERE enabled = true ORDER BY priority ASC, id ASC`)).
		WillReturnRows(rows)

	list, err := dao.GetEnabled()
	if err != nil {
		t.Fatalf("expected get enabled to succeed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 enabled sources, got %d", len(list))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMediaSourceDAODeleteMissing(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewMediaSourceDAO()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM t_media_source WHERE id = $1`)).
		WithArgs(404).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := dao.Delete(404)
	if err == nil {
		t.Fatal("expected delete to fail for missing source")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
