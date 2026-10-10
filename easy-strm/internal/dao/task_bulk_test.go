package dao

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestTaskGetAllBatchIncludesValidSortedTasksOnly(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	ctx := context.Background()
	client.RPush(ctx, taskListKey, "old", "missing", "invalid", "new")
	client.Set(ctx, taskKeyPrefix+"old", `{"task_id":"old","create_time":"2026-10-01 01:00:00"}`, 0)
	client.Set(ctx, taskKeyPrefix+"new", `{"task_id":"new","create_time":"2026-10-11 01:00:00"}`, 0)
	client.Set(ctx, taskKeyPrefix+"invalid", `{invalid`, 0)
	before := server.CommandCount()
	tasks, err := NewTaskRedisDAO(client).GetAll()
	if err != nil || len(tasks) != 2 || tasks[0]["task_id"] != "new" || server.CommandCount()-before != 2 {
		t.Fatalf("tasks=%v commands=%d err=%v", tasks, server.CommandCount()-before, err)
	}
	client.Del(ctx, taskListKey)
	tasks, err = NewTaskRedisDAO(client).GetAll()
	if err != nil || tasks == nil || len(tasks) != 0 {
		t.Fatalf("empty=%v err=%v", tasks, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = NewTaskRedisDAO(client).WithContext(cancelled).GetAll(); err == nil {
		t.Fatal("ignored cancellation")
	}
}
