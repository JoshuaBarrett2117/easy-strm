package service

import (
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestNotificationConfigServicePreservesBotToken(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	oldDB := dao.DB
	dao.Init(database, nil)
	dao.InitDAO(database)
	t.Cleanup(func() {
		dao.Init(oldDB, nil)
		dao.InitDAO(oldDB)
	})
	now := time.Now()
	existing := `{"bot_token":"secret-token","chat_id":"123","notify_task_completed":true,"notify_task_failed":true,"notify_task_cancelled":true,"notify_account_status":true}`
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config WHERE channel = $1")).
		WithArgs("telegram").
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel", "config", "enabled", "created_at", "updated_at"}).AddRow(1, "telegram", existing, true, now, now))
	mock.ExpectQuery("INSERT INTO t_notification_config").
		WithArgs("telegram", telegramConfigArgument{token: "secret-token", chatID: "456"}, true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel", "config", "enabled", "created_at", "updated_at"}).AddRow(1, "telegram", existing, true, now, now))

	service := NewNotificationConfigService(dao.NewNotificationConfigDAO())
	view, err := service.UpdateTelegramConfig(TelegramConfigUpdate{
		Enabled: true, ChatID: "456", NotifyTaskCompleted: true, NotifyTaskFailed: true,
		NotifyTaskCancelled: true, NotifyAccountStatus: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !view.HasBotToken || view.ChatID != "456" {
		t.Fatalf("脱敏视图异常: %#v", view)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type telegramConfigArgument struct {
	token  string
	chatID string
}

func (a telegramConfigArgument) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	config, err := ParseTelegramConfig(raw)
	return err == nil && config.BotToken == a.token && config.ChatID == a.chatID
}

func TestTelegramConfigViewDoesNotExposeToken(t *testing.T) {
	view := buildTelegramConfigView(TelegramConfig{BotToken: "secret", ChatID: "123"}, true)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`secret|"bot_token":`).Match(encoded) {
		t.Fatalf("配置视图泄露Token: %s", encoded)
	}
}

func TestNotificationConfigServicePreservesWeComSecret(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	oldDB := dao.DB
	dao.Init(database, nil)
	dao.InitDAO(database)
	t.Cleanup(func() {
		dao.Init(oldDB, nil)
		dao.InitDAO(oldDB)
	})
	now := time.Now()
	existing := `{"corp_id":"corp-old","agent_id":1000002,"secret":"saved-secret","receive_enabled":true,"callback_token":"saved-callback-token","encoding_aes_key":"abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG","to_user":"old-user"}`
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config WHERE channel = $1")).
		WithArgs("wecom").
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel", "config", "enabled", "created_at", "updated_at"}).AddRow(2, "wecom", existing, true, now, now))
	mock.ExpectQuery("INSERT INTO t_notification_config").
		WithArgs("wecom", weComConfigArgument{secret: "saved-secret", callbackToken: "saved-callback-token", encodingAESKey: testWeComEncodingAESKey, corpID: "corp-new", agentID: 1000003, toUser: "alice"}, true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel", "config", "enabled", "created_at", "updated_at"}).AddRow(2, "wecom", existing, true, now, now))

	configService := NewNotificationConfigService(dao.NewNotificationConfigDAO())
	view, err := configService.UpdateWeComConfig(WeComConfigUpdate{
		Enabled: true, CorpID: "corp-new", AgentID: "1000003", ToUser: "alice", ReceiveEnabled: true,
		NotifyTaskCompleted: true, NotifyTaskFailed: true, NotifyTaskCancelled: true, NotifyAccountStatus: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !view.HasSecret || !view.HasCallbackToken || !view.HasEncodingAESKey || view.AgentID != "1000003" || view.ToUser != "alice" {
		t.Fatalf("企业微信脱敏视图异常: %#v", view)
	}
	if encoded, err := json.Marshal(view); err != nil || regexp.MustCompile(`saved-secret|saved-callback-token|abcdefghijklmnopqrstuvwxyz|"secret":|"callback_token":|"encoding_aes_key":`).Match(encoded) {
		t.Fatalf("企业微信配置视图泄露Secret: %s err=%v", encoded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type weComConfigArgument struct {
	secret         string
	callbackToken  string
	encodingAESKey string
	corpID         string
	agentID        int64
	toUser         string
}

func (a weComConfigArgument) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	config, err := ParseWeComConfig(raw)
	return err == nil && config.Secret == a.secret && config.CallbackToken == a.callbackToken && config.EncodingAESKey == a.encodingAESKey && config.CorpID == a.corpID && config.AgentID == a.agentID && config.ToUser == a.toUser
}

func TestNotificationConfigServiceRejectsInvalidWeComConfig(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	oldDB := dao.DB
	dao.Init(database, nil)
	dao.InitDAO(database)
	t.Cleanup(func() {
		dao.Init(oldDB, nil)
		dao.InitDAO(oldDB)
	})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, channel, config, enabled, created_at, updated_at FROM t_notification_config WHERE channel = $1")).
		WithArgs("wecom").
		WillReturnRows(sqlmock.NewRows([]string{"id", "channel", "config", "enabled", "created_at", "updated_at"}))

	configService := NewNotificationConfigService(dao.NewNotificationConfigDAO())
	_, err = configService.UpdateWeComConfig(WeComConfigUpdate{
		Enabled: true, CorpID: "corp", AgentID: "not-a-number", Secret: "secret", ToUser: "alice",
	})
	if err == nil || !strings.Contains(err.Error(), "agent_id") {
		t.Fatalf("非法Agent ID应校验失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
