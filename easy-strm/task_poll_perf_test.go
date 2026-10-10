package main

import (
	"context"
	"errors"
	"testing"

	"easy-strm/internal/dao"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestLegacyTaskContextPointReadAndCancellation(t *testing.T) {
	previous := redisClient
	defer func() { redisClient = previous }()
	redisClient = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetTaskContext(ctx, "cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消读取不能访问Redis：%v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	defer client.Close()
	redisClient = client
	store := dao.NewTaskRedisDAO(client)
	if err := store.Create("fixture", "strm", "测试任务"); err != nil {
		t.Fatal(err)
	}
	if task, err := GetTaskContext(context.Background(), "fixture"); err != nil || task == nil || task.TaskID != "fixture" {
		t.Fatalf("单任务响应改变：%+v %v", task, err)
	}
	if task, err := GetTaskContext(context.Background(), "missing"); err != nil || task != nil {
		t.Fatalf("缺失任务响应改变：%+v %v", task, err)
	}
	client.Close()
	if _, err := GetTaskContext(context.Background(), "fixture"); err == nil {
		t.Fatal("Redis故障不得返回成功")
	}
}
