package main

import (
	"encoding/json"
	"strings"
	"testing"

	"easy-strm/internal/dao"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// TestD1LegacyTaskResponsesHidePassword 覆盖 auth.go 实际注入的旧任务列表/详情投影；旧结构不包含内部元数据。
func TestD1LegacyTaskResponsesHidePassword(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	defer client.Close()
	previous := redisClient
	redisClient = client
	defer func() { redisClient = previous }()
	store := dao.NewTaskRedisDAO(client)
	if err := store.Create("private-transfer", "share_transfer", "测试任务"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMetadata("private-transfer", map[string]interface{}{
		"share_transfer_password": "unit-test-password-D1", "share_code": "share-test",
	}); err != nil {
		t.Fatal(err)
	}
	assertPrivate := func(value interface{}, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), "unit-test-password-D1") || strings.Contains(string(payload), "share_transfer_password") {
			t.Fatal("旧任务接口泄露了内部密码")
		}
	}
	assertPrivate(GetTask("private-transfer"))
	assertPrivate(GetAllTasks())
}
