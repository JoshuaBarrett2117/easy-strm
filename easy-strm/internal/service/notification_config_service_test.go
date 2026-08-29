package service

import (
	"database/sql/driver"
	"encoding/json"
	"regexp"
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
