package service

import (
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestCacheAdminServiceGetOverviewWithDatabaseCaches(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := NewCacheAdminService(nil, nil)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM t_tmdb_cache WHERE expire_at > NOW()")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM t_identify_cache")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM t_media_file_cache")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(8))

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("expected overview to load successfully: %v", err)
	}
	if overview.RedisConnected {
		t.Fatal("redis should be marked disconnected when client is nil")
	}
	if len(overview.Groups) == 0 {
		t.Fatal("expected cache groups to be returned")
	}

	groupMap := make(map[string]int64)
	for _, item := range overview.Groups {
		groupMap[item.Key] = item.Count
	}

	if groupMap["tmdb_cache"] != 3 {
		t.Fatalf("unexpected tmdb_cache count: %d", groupMap["tmdb_cache"])
	}
	if groupMap["identify_cache"] != 5 {
		t.Fatalf("unexpected identify_cache count without redis: %d", groupMap["identify_cache"])
	}
	if groupMap["media_file_cache"] != 8 {
		t.Fatalf("unexpected media_file_cache count: %d", groupMap["media_file_cache"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCacheAdminServiceClearGroupDeletesDatabaseRows(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	svc := NewCacheAdminService(nil, nil)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM t_media_file_cache")).
		WillReturnResult(sqlmock.NewResult(0, 6))

	deleted, err := svc.ClearGroup("media_file_cache")
	if err != nil {
		t.Fatalf("expected clear group to succeed: %v", err)
	}
	if deleted != 6 {
		t.Fatalf("expected deleted rows to be 6, got %d", deleted)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
