package dao

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestAccountQuotaCacheStoresValueForFiveMinutes(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewAccountQuotaCache(client)

	if err := cache.SetAccountStorage(2, 25, 100); err != nil {
		t.Fatalf("写入容量缓存失败: %v", err)
	}
	used, total, found, err := cache.GetAccountStorage(2)
	if err != nil || !found || used != 25 || total != 100 {
		t.Fatalf("容量缓存内容不正确: used=%d total=%d found=%v err=%v", used, total, found, err)
	}
	if ttl := server.TTL(accountQuotaCacheKey(2)); ttl != 5*time.Minute {
		t.Fatalf("容量缓存TTL应为5分钟，实际为%v", ttl)
	}

	server.FastForward(5 * time.Minute)
	_, _, found, err = cache.GetAccountStorage(2)
	if err != nil || found {
		t.Fatalf("容量缓存过期后应未命中: found=%v err=%v", found, err)
	}
}

func TestAccountQuotaCacheDeletesMalformedValue(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	key := accountQuotaCacheKey(3)
	if err := client.Set(context.Background(), key, "invalid-json", 0).Err(); err != nil {
		t.Fatal(err)
	}

	cache := NewAccountQuotaCache(client)
	_, _, found, err := cache.GetAccountStorage(3)
	if err == nil || found {
		t.Fatalf("损坏缓存应返回错误并视为未命中: found=%v err=%v", found, err)
	}
	if server.Exists(key) {
		t.Fatal("损坏缓存应被删除")
	}
}
