package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	accountQuotaCacheKeyPrefix = "easy_strm:dashboard:account_quota:"
	accountQuotaCacheTTL       = 5 * time.Minute
)

type accountQuotaCacheValue struct {
	Used  int64 `json:"used"`
	Total int64 `json:"total"`
}

// AccountQuotaCache 封装仪表盘115账号容量的 Redis 缓存访问。
type AccountQuotaCache struct {
	client *redis.Client
}

// NewAccountQuotaCache 创建账号容量缓存。
func NewAccountQuotaCache(client *redis.Client) *AccountQuotaCache {
	if client == nil {
		client = GetGlobalRedisClient()
	}
	return &AccountQuotaCache{client: client}
}

// GetAccountStorage 读取账号容量缓存；未命中时 found 返回 false。
func (c *AccountQuotaCache) GetAccountStorage(accountID int) (used int64, total int64, found bool, err error) {
	if c == nil || c.client == nil {
		return 0, 0, false, nil
	}

	ctx := context.Background()
	key := accountQuotaCacheKey(accountID)
	value, getErr := c.client.Get(ctx, key).Result()
	if getErr == redis.Nil {
		return 0, 0, false, nil
	}
	if getErr != nil {
		return 0, 0, false, fmt.Errorf("读取账号容量缓存失败: %w", getErr)
	}

	var cached accountQuotaCacheValue
	if unmarshalErr := json.Unmarshal([]byte(value), &cached); unmarshalErr != nil {
		_ = c.client.Del(ctx, key).Err()
		return 0, 0, false, fmt.Errorf("解析账号容量缓存失败: %w", unmarshalErr)
	}
	return cached.Used, cached.Total, true, nil
}

// SetAccountStorage 写入账号容量缓存，固定五分钟过期。
func (c *AccountQuotaCache) SetAccountStorage(accountID int, used, total int64) error {
	if c == nil || c.client == nil {
		return nil
	}

	payload, err := json.Marshal(accountQuotaCacheValue{Used: used, Total: total})
	if err != nil {
		return fmt.Errorf("序列化账号容量缓存失败: %w", err)
	}
	if err := c.client.Set(context.Background(), accountQuotaCacheKey(accountID), payload, accountQuotaCacheTTL).Err(); err != nil {
		return fmt.Errorf("写入账号容量缓存失败: %w", err)
	}
	return nil
}

func accountQuotaCacheKey(accountID int) string {
	return accountQuotaCacheKeyPrefix + strconv.Itoa(accountID)
}
