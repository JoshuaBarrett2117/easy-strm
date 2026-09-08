package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

var redisClient *redis.Client
var ctx = context.Background()

func InitRedis(config *Config) error {
	Debug("Connecting to Redis at %s:%d", config.Redis.Host, config.Redis.Port)
	// 创建Redis客户端
	redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Redis.Host, config.Redis.Port),
		Password: config.Redis.Password,
		DB:       0, // 使用默认DB
	})

	// 测试连接
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		Error("Failed to connect to Redis: %v", err)
		return err
	}

	Debug("Redis connected successfully")
	return nil
}

// SetRedisKey 设置Redis键值对
func SetRedisKey(key string, value interface{}, expiration int) error {
	Debug("Setting Redis key: %s = %v (expiration: %d)", key, value, expiration)
	err := redisClient.Set(ctx, key, value, time.Duration(expiration)*time.Second).Err()
	if err != nil {
		Error("Failed to set Redis key %s: %v", key, err)
	}
	return err
}

// GetRedisKey 获取Redis键值
func GetRedisKey(key string) (string, error) {
	Debug("Getting Redis key: %s", key)
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		Debug("Redis key %s not found or error: %v", key, err)
	}
	return value, err
}

// DeleteRedisKey 删除Redis键
func DeleteRedisKey(key string) error {
	Debug("Deleting Redis key: %s", key)
	err := redisClient.Del(ctx, key).Err()
	if err != nil {
		Error("Failed to delete Redis key %s: %v", key, err)
	}
	return err
}

// SetToken 设置JWT token到Redis
func SetToken(userID int, token string) error {
	key := fmt.Sprintf("easy_strm:token:%d", userID)
	Debug("Setting token for user %d in Redis", userID)
	// 设置token过期时间为24小时
	err := redisClient.Set(ctx, key, token, 24*time.Hour).Err()
	if err != nil {
		Error("Failed to set token for user %d: %v", userID, err)
	}
	return err
}

// GetToken 从Redis获取JWT token
func GetToken(userID int) (string, error) {
	key := fmt.Sprintf("easy_strm:token:%d", userID)
	Debug("Getting token for user %d from Redis", userID)
	token, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		Debug("Token for user %d not found: %v", userID, err)
	}
	return token, err
}

// DeleteToken 从Redis删除JWT token
func DeleteToken(userID int) error {
	key := fmt.Sprintf("easy_strm:token:%d", userID)
	Debug("Deleting token for user %d from Redis", userID)
	err := redisClient.Del(ctx, key).Err()
	if err != nil {
		Error("Failed to delete token for user %d: %v", userID, err)
	}
	return err
}

// MigrateRedisKeys 迁移旧的Redis key到新的key格式（添加easy_strm前缀）
func MigrateRedisKeys() error {
	Info("Starting Redis key migration...")

	migratedCount := 0

	oldNewKeyPairs := []struct {
		oldPattern string
		newPrefix  string
	}{
		{"token:*", "easy_strm:token:"},
		{"strm:task:*", "easy_strm:task:"},
	}

	for _, pair := range oldNewKeyPairs {
		keys, err := redisClient.Keys(ctx, pair.oldPattern).Result()
		if err != nil {
			Debug("No keys found for pattern %s: %v", pair.oldPattern, err)
			continue
		}

		for _, oldKey := range keys {
			var newKey string
			if pair.oldPattern == "token:*" {
				newKey = "easy_strm:" + oldKey
			} else if pair.oldPattern == "strm:task:*" {
				newKey = "easy_strm:task:" + oldKey[len("strm:task:"):]
			}

			exists, err := redisClient.Exists(ctx, newKey).Result()
			if err != nil {
				Warn("Failed to check if new key %s exists: %v", newKey, err)
				continue
			}

			if exists > 0 {
				Debug("New key %s already exists, skipping migration of %s", newKey, oldKey)
				redisClient.Del(ctx, oldKey)
				continue
			}

			ttl, err := redisClient.TTL(ctx, oldKey).Result()
			if err != nil {
				Warn("Failed to get TTL for key %s: %v", oldKey, err)
				ttl = -1
			}

			value, err := redisClient.Get(ctx, oldKey).Result()
			if err != nil {
				Warn("Failed to get value for key %s: %v", oldKey, err)
				continue
			}

			if ttl > 0 {
				err = redisClient.Set(ctx, newKey, value, ttl).Err()
			} else {
				err = redisClient.Set(ctx, newKey, value, 0).Err()
			}

			if err != nil {
				Error("Failed to set new key %s: %v", newKey, err)
				continue
			}

			redisClient.Del(ctx, oldKey)
			Debug("Migrated key: %s -> %s", oldKey, newKey)
			migratedCount++
		}
	}

	if migratedCount > 0 {
		Info("Redis key migration completed, %d keys migrated", migratedCount)
	} else {
		Info("No Redis keys needed migration")
	}

	return nil
}

// getRedisClientInstance 获取全局Redis客户端实例
func getRedisClientInstance() *redis.Client {
	return redisClient
}

// GetRedisClientForService 获取全局Redis客户端实例（供Service层使用）
func GetRedisClientForService() *redis.Client {
	return redisClient
}
