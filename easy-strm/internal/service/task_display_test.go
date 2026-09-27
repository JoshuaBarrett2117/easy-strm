package service

import (
	"easy-strm/internal/dao"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"strings"
	"testing"
)

func TestTaskDisplayAPI(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	store := dao.NewTaskRedisDAO(client)
	svc := NewTaskService(store)
	if err := svc.Create("partial", "share_identify", "部分成功"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateProgress("partial", 115, 115, 62, 53); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetError("partial", "识别完成，但有53项失败"); err != nil {
		t.Fatal(err)
	}
	task, err := svc.Get("partial")
	if err != nil || task["display_status"] != "partial_success" || task["status"] != "failed" {
		t.Fatalf("详情展示状态错误: %v %v", task, err)
	}
	list, err := svc.GetUnified()
	if err != nil || len(list) != 1 || list[0]["display_status"] != "partial_success" {
		t.Fatalf("列表展示状态错误: %v %v", list, err)
	}
	raw, err := store.Get("partial")
	if err != nil || raw["status"] != "failed" || raw["display_status"] != nil {
		t.Fatalf("展示不应改变存储: %v %v", raw, err)
	}
}

func TestTaskDisplayStatus(t *testing.T) {
	for _, tc := range []struct {
		status          string
		success, failed int
		want            string
	}{
		{"failed", 62, 53, "partial_success"},
		{"completed", 1, 1, "partial_success"},
		{"success", 1, 1, "partial_success"},
		{"partial_failed", 1, 1, "partial_success"},
		{"partial_success", 0, 0, "partial_success"},
		{"failed", 0, 53, "failed"},
		{"completed", 62, 0, "completed"},
		{"running", 1, 1, "running"},
		{"cancelled", 1, 1, "cancelled"},
		{"unknown", 1, 1, "unknown"},
	} {
		task := map[string]interface{}{"status": tc.status, "success_files": float64(tc.success), "failed_files": float64(tc.failed)}
		if got := taskDisplayStatus(task); got != tc.want {
			t.Errorf("%v: got %s, want %s", tc, got, tc.want)
		}
		if task["status"] != tc.status {
			t.Fatal("展示状态不应修改执行状态")
		}
	}
}

func TestPartialSuccessNotification(t *testing.T) {
	for _, status := range []string{"failed", "completed", "partial_success", "partial_failed"} {
		task := map[string]interface{}{"task_name": "分享媒体批量识别", "status": status, "success_files": 62, "failed_files": 53, "error_message": "识别完成，但有53项失败"}
		card := buildTaskEventCard(task, normalizeTerminalStatus(status))
		for _, text := range []string{card.TelegramText(), buildWeComText(card)} {
			if !strings.Contains(text, "部分成功") || strings.Contains(text, "❌") || strings.Contains(text, "任务已失败") {
				t.Fatalf("错误的部分成功通知: %s", text)
			}
		}
	}
}
