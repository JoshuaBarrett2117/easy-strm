package dao

import (
	"testing"

	"github.com/go-redis/redis/v8"
)

func TestGetGlobalRedisClientFallsBackToInitClient(t *testing.T) {
	originRedisClient := RedisClient
	originLegacyClient := redisClient
	t.Cleanup(func() {
		RedisClient = originRedisClient
		redisClient = originLegacyClient
	})

	expected := &redis.Client{}
	RedisClient = expected
	redisClient = nil

	if got := GetGlobalRedisClient(); got != expected {
		t.Fatalf("expected fallback to RedisClient, got %#v", got)
	}
}
