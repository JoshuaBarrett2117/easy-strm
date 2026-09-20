package service

import (
	"regexp"
	"testing"
	"time"

	"easy-strm/internal/dao"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSystemConfigServiceHidesRemovedAlistSettings(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, config_key, config_val, create_time, update_time FROM t_system_config")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).
			AddRow(1, "alist_url", "http://unused", now, now).
			AddRow(2, "alist_token", "unused", now, now).
			AddRow(3, "proxy_url", "http://127.0.0.1:7890", now, now).
			AddRow(4, "metatube_token", "secret", now, now))

	service := NewSystemConfigService(dao.NewSystemConfigDAO())

	configs, err := service.GetAll()
	if err != nil {
		t.Fatalf("读取系统配置失败: %v", err)
	}
	if len(configs) != 1 || configs[0].ConfigKey != "proxy_url" {
		t.Fatalf("已停用的 Alist 配置不应返回: %#v", configs)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 预期未满足: %v", err)
	}
}

func TestSystemConfigServiceDoesNotExposeMetaTubeToken(t *testing.T) {
	service := NewSystemConfigService(nil)
	if config, err := service.GetByKey("metatube_token"); err != nil || config != nil {
		t.Fatalf("MetaTube 令牌不应通过读取接口返回: config=%#v err=%v", config, err)
	}
}

func TestSystemConfigServiceRejectsRemovedAlistSettings(t *testing.T) {
	service := NewSystemConfigService(nil)

	if config, err := service.GetByKey("alist_url"); err != nil || config != nil {
		t.Fatalf("已停用配置应表现为不存在: config=%#v err=%v", config, err)
	}
	if err := service.Upsert("alist_token", "unused"); err == nil {
		t.Fatal("已停用配置不应允许写入")
	}
}

func TestSystemConfigServiceRejectsUnknownRuleSettings(t *testing.T) {
	service := NewSystemConfigService(nil)
	if _, err := service.UpdateRuleSettings("scrape", map[string]bool{"tmdb_api_key": true}); err == nil {
		t.Fatal("expected unknown rule setting to be rejected")
	}
	if _, err := service.GetRuleSettings("unknown"); err == nil {
		t.Fatal("expected unknown rule setting group to be rejected")
	}
}
