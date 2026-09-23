package service

import (
	"errors"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestConfigSecretRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := dao.DB
	dao.DB = db
	dao.InitDAO(db)
	t.Cleanup(func() { dao.DB = old; dao.InitDAO(old) })
	s := NewConfigSecretService(dao.NewSystemConfigDAO(), NewNotificationConfigService(dao.NewNotificationConfigDAO()), dao.NewEmbyServerDAO(db), func() string { return "runtime-key" }, NewAIRecognitionService(dao.NewSystemConfigDAO(), nil))
	mock.ExpectQuery("SELECT id, config_key").WithArgs(AIRecognitionConfigKey).WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).AddRow(1, AIRecognitionConfigKey, `{"api_key":"ai-secret"}`, time.Now(), time.Now()))
	if value, err := s.Read("ai_recognition_api_key", 0); err != nil || value != "ai-secret" {
		t.Fatalf("AI 明文错误: %q %v", value, err)
	}
	for _, key := range []string{"tmdb_api_key", "global_api_key", "emby_cover_ai_api_key"} {
		mock.ExpectQuery("SELECT id, config_key").WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}).AddRow(1, key, "saved-secret", time.Now(), time.Now()))
		value, err := s.Read(key, 0)
		if err != nil || value != "saved-secret" {
			t.Fatalf("%s: value=%q err=%v", key, value, err)
		}
	}
	mock.ExpectQuery("SELECT id, config_key").WithArgs("tmdb_api_key").WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "config_val", "create_time", "update_time"}))
	if value, err := s.Read("tmdb_api_key", 0); err != nil || value != "runtime-key" {
		t.Fatalf("配置文件回退失败: %q %v", value, err)
	}
	if _, err := s.Read("jwt_secret", 0); !errors.Is(err, ErrUnknownConfigSecret) {
		t.Fatalf("应拒绝白名单以外的字段: %v", err)
	}
	mock.ExpectQuery("SELECT id, config_key").WithArgs("global_api_key").WillReturnError(errors.New("database unavailable"))
	if _, err := s.Read("global_api_key", 0); err == nil {
		t.Fatal("不能吞掉读取错误")
	}
	mock.ExpectQuery("SELECT id, name").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}).AddRow(7, "test", "http://localhost", "server-secret", true, true, time.Now(), time.Now()))
	if value, err := s.Read("emby_server_api_key", 7); err != nil || value != "server-secret" {
		t.Fatalf("实例明文错误: %q %v", value, err)
	}

	for _, tc := range []struct{ key, channel, field string }{
		{"telegram_bot_token", "telegram", "bot_token"},
		{"wecom_secret", "wecom", "secret"},
		{"wecom_callback_token", "wecom", "callback_token"},
		{"wecom_encoding_aes_key", "wecom", "encoding_aes_key"},
	} {
		mock.ExpectQuery("SELECT id, channel, config").WithArgs(tc.channel).WillReturnRows(sqlmock.NewRows([]string{"id", "channel", "config", "enabled", "created_at", "updated_at"}).AddRow(1, tc.channel, `{"`+tc.field+`":"notification-secret"}`, true, time.Now(), time.Now()))
		if value, err := s.Read(tc.key, 0); err != nil || value != "notification-secret" {
			t.Fatalf("%s: %q %v", tc.key, value, err)
		}
	}
	mock.ExpectQuery("SELECT id, name").WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "base_url", "api_key", "enabled", "is_default", "create_time", "update_time"}))
	if _, err := s.Read("emby_server_api_key", 99); err == nil {
		t.Fatal("不存在的实例应返回错误")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
