package service

import (
	"strings"
	"testing"
)

func TestNotificationCardTelegramTextEscapesAndTruncates(t *testing.T) {
	card := NotificationCard{
		Title:  "任务 <完成>",
		Status: "✅",
		Fields: [][2]string{{"文件", "a&b.mkv"}},
		Detail: strings.Repeat("错误<详情>", 700),
	}
	text := card.TelegramText()
	if strings.Contains(text, "<完成>") || strings.Contains(text, "a&b") {
		t.Fatalf("Telegram HTML 未转义: %s", text)
	}
	if !strings.Contains(text, "&lt;完成&gt;") || !strings.Contains(text, "a&amp;b") {
		t.Fatalf("Telegram HTML 转义结果不完整: %s", text)
	}
	if len([]rune(text)) > telegramMessageLimit {
		t.Fatalf("Telegram 文本超过限制: %d", len([]rune(text)))
	}
}

func TestNotificationCardTelegramMarkupFiltersInvalidActions(t *testing.T) {
	card := NotificationCard{Actions: [][]NotificationAction{{
		{Text: "查看", Data: "tasks"},
		{Text: "超长", Data: strings.Repeat("x", 65)},
	}}}
	markup := card.TelegramMarkup()
	if markup == nil || len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("按钮过滤结果异常: %#v", markup)
	}
}

func TestParseTelegramConfigDefaultsOnlyMissingFields(t *testing.T) {
	config, err := ParseTelegramConfig(`{"bot_token":"token","chat_id":"1","notify_task_failed":false}`)
	if err != nil {
		t.Fatal(err)
	}
	if config.NotifyTaskFailed {
		t.Fatal("显式关闭的失败通知不应被默认值覆盖")
	}
	if !config.NotifyTaskStarted || !config.NotifyTaskCompleted || !config.NotifyTaskCancelled || !config.NotifyAccountStatus {
		t.Fatalf("缺失字段未应用默认值: %#v", config)
	}
}

func TestBuildTaskCardActions(t *testing.T) {
	task := map[string]interface{}{
		"task_id": "task-1", "task_type": "watch_auto_organize", "task_name": "自动整理",
		"status": "failed", "progress": 50, "success_files": 1, "failed_files": 1,
		"error_message": "识别失败", "update_time": "2026-08-29 12:00:00",
	}
	card := buildTaskCard(task, true)
	if len(card.Actions) != 2 || card.Actions[1][0].Data != "task:retry:task-1" {
		t.Fatalf("失败自动整理任务缺少重试按钮: %#v", card.Actions)
	}
}
