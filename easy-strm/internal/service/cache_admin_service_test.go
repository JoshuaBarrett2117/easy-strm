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

func TestCacheAdminServiceIncludesAccountQuotaCache(t *testing.T) {
	svc := NewCacheAdminService(nil, nil)
	for _, group := range svc.groupDefinitions() {
		if group.Key != "account_quota_cache" {
			continue
		}
		if len(group.RedisPatterns) != 1 || group.RedisPatterns[0] != "easy_strm:dashboard:account_quota:*" {
			t.Fatalf("账号容量缓存匹配规则不正确: %+v", group.RedisPatterns)
		}
		return
	}
	t.Fatal("缓存管理缺少账号容量缓存分组")
}

func TestCacheGroupStatusDistinguishesEmptyAndUnavailable(t *testing.T) {
	group := cacheGroupDefinition{Storage: "redis", Enabled: true}
	status, text := cacheGroupStatus(group, 0, false)
	if status != "unavailable" || text != "Redis 不可用" {
		t.Fatalf("Redis未连接状态错误: status=%s text=%s", status, text)
	}
	status, text = cacheGroupStatus(group, 0, true)
	if status != "empty" || text != "暂无有效缓存" {
		t.Fatalf("空缓存状态错误: status=%s text=%s", status, text)
	}
	status, text = cacheGroupStatus(group, 2, false)
	if status != "active" || text != "已启用" {
		t.Fatalf("有缓存状态错误: status=%s text=%s", status, text)
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
