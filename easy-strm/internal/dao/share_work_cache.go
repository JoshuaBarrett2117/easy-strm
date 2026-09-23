package dao

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"easy-strm/internal/domain"
	"github.com/go-redis/redis/v8"
)

// ShareWorkIdentity 缓存经数据源确认的作品身份，季集信息由每个文件独立解析。
type ShareWorkIdentity struct {
	Result       *domain.TmdbIdentifyResult `json:"result"`
	Seasons      []int                      `json:"seasons,omitempty"`
	SeasonsKnown bool                       `json:"seasons_known"`
}

// ShareWorkCacheDAO 使用独立版本键保存作品身份，不读取历史文件匹配缓存。
type ShareWorkCacheDAO struct{}

func shareWorkCacheKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "easy_strm:share:work:v1:" + hex.EncodeToString(sum[:])
}

// Get 获取有效的作品身份；未配置缓存时返回未命中。
func (ShareWorkCacheDAO) Get(ctx context.Context, key string) (*ShareWorkIdentity, error) {
	client := GetGlobalRedisClient()
	if client == nil {
		return nil, nil
	}
	data, err := client.Get(ctx, shareWorkCacheKey(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var value ShareWorkIdentity
	if err = json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if !validShareWorkIdentity(&value) {
		return nil, nil
	}
	return &value, nil
}

// Save 仅保存成功身份；无匹配和网络故障不能形成跨任务负缓存。
func (ShareWorkCacheDAO) Save(ctx context.Context, key string, value *ShareWorkIdentity) error {
	client := GetGlobalRedisClient()
	if client == nil || !validShareWorkIdentity(value) {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return client.Set(ctx, shareWorkCacheKey(key), data, 7*24*time.Hour).Err()
}

func validShareWorkIdentity(value *ShareWorkIdentity) bool {
	if value == nil || value.Result == nil || !value.Result.Success {
		return false
	}
	r := value.Result
	return (r.MediaType == "tv" || r.MediaType == "movie") && (r.TmdbID > 0 || r.MetadataID != "" && r.MetadataProvider != "")
}

// Delete 删除手动修正影响的作品缓存，避免后续新集复用旧身份。
func (ShareWorkCacheDAO) Delete(ctx context.Context, keys ...string) error {
	client := GetGlobalRedisClient()
	if client == nil || len(keys) == 0 {
		return nil
	}
	redisKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		redisKeys = append(redisKeys, shareWorkCacheKey(key))
	}
	return client.Del(ctx, redisKeys...).Err()
}
