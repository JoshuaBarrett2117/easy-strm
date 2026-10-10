package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"easy-strm/internal/dao"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestUnifiedPollingKeepsAllTasksSortAndCancellation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	for index := 0; index < 1101; index++ {
		id := fmt.Sprintf("任务-%04d", index)
		mini.Push("easy_strm:task:list", id)
		if index == 501 {
			continue
		}
		raw, err := json.Marshal(map[string]interface{}{"task_id": id, "create_time": fmt.Sprintf("%08d", index), "status": "completed", "task_type": "emby_cover"})
		if err != nil {
			t.Fatal(err)
		}
		mini.Set("easy_strm:task:"+id, string(raw))
	}
	service := NewTaskService(dao.NewTaskRedisDAO(client))
	tasks, err := service.GetUnifiedContext(context.Background())
	if err != nil || len(tasks) != 1100 || tasks[0]["task_id"] != "任务-1100" || tasks[1099]["task_id"] != "任务-0000" {
		t.Fatalf("分批读取不得截断或改变排序：数量=%d %v", len(tasks), err)
	}
	seen := make(map[string]bool)
	for _, task := range tasks {
		id := task["task_id"].(string)
		if seen[id] {
			t.Fatalf("重复任务：%s", id)
		}
		seen[id] = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.GetUnifiedContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未透传：%v", err)
	}
	if _, err = service.GetContext(ctx, "任务-0000"); !errors.Is(err, context.Canceled) {
		t.Fatalf("单任务取消未透传：%v", err)
	}
}

func TestOrganizePollingKeepsTerminalResultAndCancellation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	service := &OrganizeService{redisClient: client}
	mini.Set("organize:task:候选", `{"task_id":"候选","status":"completed","total":1,"result":[{"file_id":"文件"}]}`)
	mini.Set("organize:task:预览", `{"task_id":"预览","status":"completed","result":{"previews":[]}}`)
	candidate, err := service.GetCandidateTaskStatusContext(context.Background(), "候选")
	if err != nil || candidate.Status != "completed" || candidate.Total != 1 || len(candidate.Result) != 1 {
		t.Fatalf("终态候选结果丢失：%+v %v", candidate, err)
	}
	preview, err := service.GetPreviewTaskStatusContext(context.Background(), "预览")
	if err != nil || preview.Result == nil {
		t.Fatalf("终态预览结果丢失：%+v %v", preview, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.GetCandidateTaskStatusContext(ctx, "候选"); !errors.Is(err, context.Canceled) {
		t.Fatalf("候选取消未透传：%v", err)
	}
	if _, err = service.GetPreviewTaskStatusContext(ctx, "预览"); !errors.Is(err, context.Canceled) {
		t.Fatalf("预览取消未透传：%v", err)
	}
}
