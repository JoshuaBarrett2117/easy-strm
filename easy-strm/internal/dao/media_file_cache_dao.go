package dao

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const mediaFileCacheKeyPrefix = "easy_strm:media_file:cache:"

// MediaFileCacheDAO 媒体文件缓存数据访问层
type MediaFileCacheDAO struct{}

// NewMediaFileCacheDAO 创建媒体文件缓存DAO实例
func NewMediaFileCacheDAO() *MediaFileCacheDAO {
	return &MediaFileCacheDAO{}
}

// MediaFileCache 媒体文件缓存模型
type MediaFileCache struct {
	ID            int             `json:"id"`
	SourceID      int             `json:"source_id"`
	FilePath      string          `json:"file_path"`
	FileName      string          `json:"file_name"`
	FileSize      int64           `json:"file_size"`
	SHA1          string          `json:"sha1"`
	TmdbID        int             `json:"tmdb_id"`
	MediaType     string          `json:"media_type"`
	SeasonNumber  int             `json:"season_number"`
	EpisodeNumber int             `json:"episode_number"`
	TmdbData      json.RawMessage `json:"tmdb_data"`
	IdentifiedAt  *time.Time      `json:"identified_at"`
	CreateTime    time.Time       `json:"create_time"`
	UpdateTime    time.Time       `json:"update_time"`
}

// GetByPath 根据路径获取文件缓存
func (d *MediaFileCacheDAO) GetByPath(sourceID int, filePath string) (*MediaFileCache, error) {
	if cache, ok := d.getFromRedis(sourceID, filePath); ok {
		return cache, nil
	}

	cache := &MediaFileCache{}
	var tmdbID sql.NullInt64
	var mediaType sql.NullString
	var seasonNumber, episodeNumber sql.NullInt32
	var tmdbData []byte
	var identifiedAt sql.NullTime

	err := DB.QueryRow(
		`SELECT id, source_id, file_path, file_name, file_size, sha1, tmdb_id, media_type,
		        season_number, episode_number, tmdb_data, identified_at, create_time, update_time
		 FROM t_media_file_cache
		 WHERE source_id = $1 AND file_path = $2`,
		sourceID, filePath,
	).Scan(
		&cache.ID, &cache.SourceID, &cache.FilePath, &cache.FileName, &cache.FileSize, &cache.SHA1,
		&tmdbID, &mediaType, &seasonNumber, &episodeNumber, &tmdbData, &identifiedAt,
		&cache.CreateTime, &cache.UpdateTime,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("MediaFileCacheDAO[GetByPath] 查询失败: %v", err)
	}

	if tmdbID.Valid {
		cache.TmdbID = int(tmdbID.Int64)
	}
	if mediaType.Valid {
		cache.MediaType = mediaType.String
	}
	if seasonNumber.Valid {
		cache.SeasonNumber = int(seasonNumber.Int32)
	}
	if episodeNumber.Valid {
		cache.EpisodeNumber = int(episodeNumber.Int32)
	}
	if tmdbData != nil {
		cache.TmdbData = tmdbData
	}
	if identifiedAt.Valid {
		cache.IdentifiedAt = &identifiedAt.Time
	}

	d.saveToRedis(cache)
	return cache, nil
}

// CreateOrUpdate 创建或更新文件缓存
func (d *MediaFileCacheDAO) CreateOrUpdate(cache *MediaFileCache) error {
	err := DB.QueryRow(
		`INSERT INTO t_media_file_cache (source_id, file_path, file_name, file_size, sha1, tmdb_id,
		        media_type, season_number, episode_number, tmdb_data, identified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (source_id, file_path) DO UPDATE SET
		        file_name = EXCLUDED.file_name,
		        file_size = EXCLUDED.file_size,
		        sha1 = EXCLUDED.sha1,
		        tmdb_id = EXCLUDED.tmdb_id,
		        media_type = EXCLUDED.media_type,
		        season_number = EXCLUDED.season_number,
		        episode_number = EXCLUDED.episode_number,
		        tmdb_data = EXCLUDED.tmdb_data,
		        identified_at = EXCLUDED.identified_at,
		        update_time = NOW()
		 RETURNING id, create_time, update_time`,
		cache.SourceID, cache.FilePath, cache.FileName, cache.FileSize, nullString(cache.SHA1),
		nullInt(cache.TmdbID), nullString(cache.MediaType), nullInt(cache.SeasonNumber),
		nullInt(cache.EpisodeNumber), cache.TmdbData, cache.IdentifiedAt,
	).Scan(&cache.ID, &cache.CreateTime, &cache.UpdateTime)

	if err != nil {
		return fmt.Errorf("MediaFileCacheDAO[CreateOrUpdate] 创建/更新失败: %v", err)
	}
	d.saveToRedis(cache)
	return nil
}

// DeleteBySourceID 删除指定媒体源的所有缓存
func (d *MediaFileCacheDAO) DeleteBySourceID(sourceID int) error {
	_, err := DB.Exec("DELETE FROM t_media_file_cache WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("MediaFileCacheDAO[DeleteBySourceID] 删除失败: %v", err)
	}
	return nil
}

func (d *MediaFileCacheDAO) getFromRedis(sourceID int, filePath string) (*MediaFileCache, bool) {
	client := GetGlobalRedisClient()
	if client == nil {
		return nil, false
	}

	value, err := client.Get(context.Background(), mediaFileCacheKey(sourceID, filePath)).Result()
	if err == redis.Nil || value == "" {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	var cache MediaFileCache
	if err := json.Unmarshal([]byte(value), &cache); err != nil {
		return nil, false
	}
	return &cache, true
}

func (d *MediaFileCacheDAO) saveToRedis(cache *MediaFileCache) {
	client := GetGlobalRedisClient()
	if client == nil || cache == nil {
		return
	}

	payload, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = client.Set(context.Background(), mediaFileCacheKey(cache.SourceID, cache.FilePath), payload, tmdbCacheRedisTTL).Err()
}

func mediaFileCacheKey(sourceID int, filePath string) string {
	normalizedPath := strings.ToLower(strings.TrimSpace(filepathToSlash(filePath)))
	return mediaFileCacheKeyPrefix + strconv.Itoa(sourceID) + ":" + normalizedPath
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
