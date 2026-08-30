package service

import (
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"easy-strm/internal/dao"
)

type fakeDashboardStorageProvider struct {
	values map[int][2]int64
	errors map[int]error
	calls  map[int]int
}

func (f *fakeDashboardStorageProvider) GetAccountStorage(cloud115ID int, _ string) (int64, int64, error) {
	if f.calls == nil {
		f.calls = map[int]int{}
	}
	f.calls[cloud115ID]++
	if err := f.errors[cloud115ID]; err != nil {
		return 0, 0, err
	}
	value := f.values[cloud115ID]
	return value[0], value[1], nil
}

type fakeDashboardStorageCache struct {
	values map[int][2]int64
	errors map[int]error
	writes map[int][2]int64
}

func (f *fakeDashboardStorageCache) GetAccountStorage(accountID int) (int64, int64, bool, error) {
	if err := f.errors[accountID]; err != nil {
		return 0, 0, false, err
	}
	value, found := f.values[accountID]
	return value[0], value[1], found, nil
}

func (f *fakeDashboardStorageCache) SetAccountStorage(accountID int, used, total int64) error {
	if f.writes == nil {
		f.writes = map[int][2]int64{}
	}
	f.writes[accountID] = [2]int64{used, total}
	return nil
}

func expectDashboardAccounts(mock sqlmock.Sqlmock) {
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "name", "cookie", "cookie_source", "refresh_token", "access_token", "expires_in",
		"transfer_account_id", "transfer_directory", "account_type", "quota_used",
		"priority", "status", "cooling_start_time", "transfer_method", "alist_url",
		"alist_token", "create_time", "update_time",
	}).
		AddRow(1, "115大号", "cookie-1", "微信小程序", "", "", 0, 0, "", "resource", 5, 5, "active", nil, "115driver", "", "", now, now).
		AddRow(2, "115小号", "cookie-2", "", "", "", 0, 0, "", "resource", 7, 5, "active", nil, "115driver", "", "", now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name, cookie, COALESCE(cookie_source, ''), refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 ORDER BY id ASC`)).WillReturnRows(rows)
}

func TestDashboardServiceFillStorageStatsUsesLiveAccountCapacity(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectDashboardAccounts(mock)

	provider := &fakeDashboardStorageProvider{
		values: map[int][2]int64{1: {75, 100}, 2: {120, 100}},
		errors: map[int]error{},
	}
	cache := &fakeDashboardStorageCache{values: map[int][2]int64{}, errors: map[int]error{}}
	service := NewDashboardService(dao.NewCloud115DAO(), nil, nil, nil, provider, cache)
	stats := &DashboardStats{}

	if err := service.fillStorageStats(stats); err != nil {
		t.Fatalf("填充账号容量失败: %v", err)
	}
	if len(stats.Storage.Accounts) != 2 {
		t.Fatalf("期望返回2个账号，实际为%d", len(stats.Storage.Accounts))
	}
	if first := stats.Storage.Accounts[0]; !first.Available || first.Used != 75 || first.Total != 100 || first.Percentage != 75 {
		t.Fatalf("首个账号容量不正确: %+v", first)
	}
	if second := stats.Storage.Accounts[1]; second.Percentage != 100 {
		t.Fatalf("超出总容量时进度应限制为100%%，实际为%v", second.Percentage)
	}
	if cache.writes[1] != [2]int64{75, 100} || cache.writes[2] != [2]int64{120, 100} {
		t.Fatalf("实时容量应回填缓存: %+v", cache.writes)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("数据库调用不符合预期: %v", err)
	}
}

func TestDashboardServiceFillStorageStatsKeepsOtherAccountsWhenOneFails(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectDashboardAccounts(mock)

	provider := &fakeDashboardStorageProvider{
		values: map[int][2]int64{2: {25, 100}},
		errors: map[int]error{1: errors.New("cookie expired")},
	}
	service := NewDashboardService(dao.NewCloud115DAO(), nil, nil, nil, provider, nil)
	stats := &DashboardStats{}

	if err := service.fillStorageStats(stats); err != nil {
		t.Fatalf("单个账号失败不应中断列表: %v", err)
	}
	if failed := stats.Storage.Accounts[0]; failed.Available || failed.Used != 5 {
		t.Fatalf("失败账号应保留数据库缓存并标记不可用: %+v", failed)
	}
	if succeeded := stats.Storage.Accounts[1]; !succeeded.Available || succeeded.Percentage != 25 {
		t.Fatalf("其余账号仍应正常显示: %+v", succeeded)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("数据库调用不符合预期: %v", err)
	}
}

func TestDashboardServiceFillStorageStatsPrefersCache(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectDashboardAccounts(mock)

	provider := &fakeDashboardStorageProvider{
		values: map[int][2]int64{1: {1, 1}, 2: {2, 2}},
		errors: map[int]error{},
	}
	cache := &fakeDashboardStorageCache{
		values: map[int][2]int64{1: {30, 100}, 2: {40, 100}},
		errors: map[int]error{},
	}
	service := NewDashboardService(dao.NewCloud115DAO(), nil, nil, nil, provider, cache)
	stats := &DashboardStats{}

	if err := service.fillStorageStats(stats); err != nil {
		t.Fatalf("读取缓存容量失败: %v", err)
	}
	if stats.Storage.Accounts[0].Used != 30 || stats.Storage.Accounts[1].Used != 40 {
		t.Fatalf("应优先使用缓存容量: %+v", stats.Storage.Accounts)
	}
	if provider.calls[1] != 0 || provider.calls[2] != 0 {
		t.Fatalf("缓存命中后不应调用115接口: %+v", provider.calls)
	}
}

func TestDashboardServiceFillStorageStatsFallsBackAfterCacheError(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectDashboardAccounts(mock)

	provider := &fakeDashboardStorageProvider{
		values: map[int][2]int64{1: {50, 100}, 2: {60, 100}},
		errors: map[int]error{},
	}
	cache := &fakeDashboardStorageCache{
		values: map[int][2]int64{},
		errors: map[int]error{1: errors.New("invalid cache")},
	}
	service := NewDashboardService(dao.NewCloud115DAO(), nil, nil, nil, provider, cache)
	stats := &DashboardStats{}

	if err := service.fillStorageStats(stats); err != nil {
		t.Fatalf("缓存异常后应回源成功: %v", err)
	}
	if provider.calls[1] != 1 || provider.calls[2] != 1 {
		t.Fatalf("缓存异常或未命中后应调用115接口: %+v", provider.calls)
	}
	if cache.writes[1] != [2]int64{50, 100} || cache.writes[2] != [2]int64{60, 100} {
		t.Fatalf("回源结果应写入缓存: %+v", cache.writes)
	}
}
