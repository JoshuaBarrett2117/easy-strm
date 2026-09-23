package dao

import (
	"context"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestShareWorkCacheStoresOnlyConfirmedIdentity(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	InitTaskRedisDAO(client)
	t.Cleanup(func() { InitTaskRedisDAO(nil); client.Close() })
	cache := ShareWorkCacheDAO{}
	ctx := context.Background()
	failure := &ShareWorkIdentity{Result: &domain.TmdbIdentifyResult{Message: "无匹配"}}
	if err := cache.Save(ctx, "missing", failure); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Get(ctx, "missing"); err != nil || got != nil {
		t.Fatalf("失败结果不能持久缓存: %+v %v", got, err)
	}
	identity := &ShareWorkIdentity{Result: &domain.TmdbIdentifyResult{Success: true, MediaType: "tv", TmdbID: 20464}, Seasons: []int{1, 2}, SeasonsKnown: true}
	if err := cache.Save(ctx, "show", identity); err != nil {
		t.Fatal(err)
	}
	got, err := cache.Get(ctx, "show")
	if err != nil || got == nil || got.Result.TmdbID != 20464 || len(got.Seasons) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	mini.FastForward(8 * 24 * time.Hour)
	if got, err := cache.Get(ctx, "show"); err != nil || got != nil {
		t.Fatalf("缓存应过期: %+v %v", got, err)
	}
	if err := cache.Save(ctx, "show", identity); err != nil {
		t.Fatal(err)
	}
	if err := cache.Delete(ctx, "show"); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Get(ctx, "show"); err != nil || got != nil {
		t.Fatalf("手动修正未失效缓存: %+v %v", got, err)
	}
}
