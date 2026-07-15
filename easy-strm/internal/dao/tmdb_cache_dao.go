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

const (
	tmdbCacheRedisTTL       = 7 * 24 * time.Hour
	tmdbQueryCacheKeyPrefix = "easy_strm:tmdb:query:"
	tmdbIDCacheKeyPrefix    = "easy_strm:tmdb:id:"
)

// TmdbCacheDAO TMDB缓存数据访问层
type TmdbCacheDAO struct{}

// NewTmdbCacheDAO 创建TMDB缓存DAO实例
func NewTmdbCacheDAO() *TmdbCacheDAO {
	return &TmdbCacheDAO{}
}

// TmdbCache TMDB缓存模型
type TmdbCache struct {
	ID            int             `json:"id"`
	QueryKey      string          `json:"query_key"`
	MediaType     string          `json:"media_type"`
	TmdbID        int             `json:"tmdb_id"`
	Title         string          `json:"title"`
	OriginalTitle string          `json:"original_title"`
	Year          int             `json:"year"`
	PosterPath    string          `json:"poster_path"`
	Overview      string          `json:"overview"`
	VoteAverage   float64         `json:"vote_average"`
	ReleaseDate   string          `json:"release_date"`
	FirstAirDate  string          `json:"first_air_date"`
	SeasonNumber  int             `json:"season_number"`
	EpisodeNumber int             `json:"episode_number"`
	RawData       json.RawMessage `json:"raw_data"`
	ExpireAt      time.Time       `json:"expire_at"`
	CreateTime    time.Time       `json:"create_time"`
	UpdateTime    time.Time       `json:"update_time"`
}

// GetByQueryKey 根据查询键获取缓存
// 参数:
//   - queryKey: 查询键（文件名或搜索词）
//   - mediaType: 媒体类型（movie/tv）
//
// 返回:
//   - *TmdbCache: 缓存数据
//   - error: 错误信息
func (d *TmdbCacheDAO) GetByQueryKey(queryKey, mediaType string) (*TmdbCache, error) {
	if cache, ok := d.getFromRedis(tmdbQueryCacheKey(queryKey, mediaType)); ok {
		return cache, nil
	}

	cache := &TmdbCache{}
	var year sql.NullInt32
	var voteAverage sql.NullFloat64
	var seasonNumber, episodeNumber sql.NullInt32
	var posterPath, overview, releaseDate, firstAirDate sql.NullString
	var rawData []byte

	err := DB.QueryRow(
		`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path, 
		        overview, vote_average, release_date, first_air_date, season_number, episode_number, 
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache 
		 WHERE query_key = $1 AND media_type = $2 AND expire_at > NOW()`,
		queryKey, mediaType,
	).Scan(
		&cache.ID, &cache.QueryKey, &cache.MediaType, &cache.TmdbID, &cache.Title, &cache.OriginalTitle,
		&year, &posterPath, &overview, &voteAverage, &releaseDate, &firstAirDate,
		&seasonNumber, &episodeNumber, &rawData, &cache.ExpireAt, &cache.CreateTime, &cache.UpdateTime,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("TmdbCacheDAO[GetByQueryKey] 查询失败: %v", err)
	}

	// 处理可空字段
	if year.Valid {
		cache.Year = int(year.Int32)
	}
	if voteAverage.Valid {
		cache.VoteAverage = voteAverage.Float64
	}
	if seasonNumber.Valid {
		cache.SeasonNumber = int(seasonNumber.Int32)
	}
	if episodeNumber.Valid {
		cache.EpisodeNumber = int(episodeNumber.Int32)
	}
	if posterPath.Valid {
		cache.PosterPath = posterPath.String
	}
	if overview.Valid {
		cache.Overview = overview.String
	}
	if releaseDate.Valid {
		cache.ReleaseDate = releaseDate.String
	}
	if firstAirDate.Valid {
		cache.FirstAirDate = firstAirDate.String
	}
	if rawData != nil {
		cache.RawData = rawData
	}

	d.saveToRedis(cache)
	return cache, nil
}

func (d *TmdbCacheDAO) GetByTmdbID(tmdbID int, mediaType string) (*TmdbCache, error) {
	if cache, ok := d.getFromRedis(tmdbIDCacheKey(tmdbID, mediaType)); ok {
		return cache, nil
	}

	cache := &TmdbCache{}
	var year sql.NullInt32
	var voteAverage sql.NullFloat64
	var seasonNumber, episodeNumber sql.NullInt32
	var posterPath, overview, releaseDate, firstAirDate sql.NullString
	var rawData []byte

	err := DB.QueryRow(
		`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path,
		        overview, vote_average, release_date, first_air_date, season_number, episode_number,
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache
		 WHERE tmdb_id = $1 AND media_type = $2 AND expire_at > NOW()
		 ORDER BY update_time DESC, id DESC
		 LIMIT 1`,
		tmdbID, mediaType,
	).Scan(
		&cache.ID, &cache.QueryKey, &cache.MediaType, &cache.TmdbID, &cache.Title, &cache.OriginalTitle,
		&year, &posterPath, &overview, &voteAverage, &releaseDate, &firstAirDate,
		&seasonNumber, &episodeNumber, &rawData, &cache.ExpireAt, &cache.CreateTime, &cache.UpdateTime,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("TmdbCacheDAO[GetByTmdbID] query failed: %v", err)
	}

	if year.Valid {
		cache.Year = int(year.Int32)
	}
	if voteAverage.Valid {
		cache.VoteAverage = voteAverage.Float64
	}
	if seasonNumber.Valid {
		cache.SeasonNumber = int(seasonNumber.Int32)
	}
	if episodeNumber.Valid {
		cache.EpisodeNumber = int(episodeNumber.Int32)
	}
	if posterPath.Valid {
		cache.PosterPath = posterPath.String
	}
	if overview.Valid {
		cache.Overview = overview.String
	}
	if releaseDate.Valid {
		cache.ReleaseDate = releaseDate.String
	}
	if firstAirDate.Valid {
		cache.FirstAirDate = firstAirDate.String
	}
	if rawData != nil {
		cache.RawData = rawData
	}

	d.saveToRedis(cache)
	return cache, nil
}

// Create 创建缓存
// 参数:
//   - cache: 缓存数据
//
// 返回:
//   - error: 错误信息
func (d *TmdbCacheDAO) Create(cache *TmdbCache) error {
	err := DB.QueryRow(
		`INSERT INTO t_tmdb_cache (query_key, media_type, tmdb_id, title, original_title, year, 
		        poster_path, overview, vote_average, release_date, first_air_date, season_number, 
		        episode_number, raw_data, expire_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING id, create_time, update_time`,
		cache.QueryKey, cache.MediaType, cache.TmdbID, cache.Title, cache.OriginalTitle, cache.Year,
		nullString(cache.PosterPath), nullString(cache.Overview), cache.VoteAverage,
		nullString(cache.ReleaseDate), nullString(cache.FirstAirDate),
		nullInt(cache.SeasonNumber), nullInt(cache.EpisodeNumber),
		cache.RawData, cache.ExpireAt,
	).Scan(&cache.ID, &cache.CreateTime, &cache.UpdateTime)

	if err != nil {
		return fmt.Errorf("TmdbCacheDAO[Create] 创建失败: %v", err)
	}
	d.saveToRedis(cache)
	return nil
}

// Update 更新缓存
// 参数:
//   - cache: 缓存数据
//
// 返回:
//   - error: 错误信息
func (d *TmdbCacheDAO) Update(cache *TmdbCache) error {
	_, err := DB.Exec(
		`UPDATE t_tmdb_cache SET 
		        tmdb_id = $3, title = $4, original_title = $5, year = $6, poster_path = $7,
		        overview = $8, vote_average = $9, release_date = $10, first_air_date = $11,
		        season_number = $12, episode_number = $13, raw_data = $14, expire_at = $15,
		        update_time = NOW()
		 WHERE query_key = $1 AND media_type = $2`,
		cache.QueryKey, cache.MediaType, cache.TmdbID, cache.Title, cache.OriginalTitle, cache.Year,
		nullString(cache.PosterPath), nullString(cache.Overview), cache.VoteAverage,
		nullString(cache.ReleaseDate), nullString(cache.FirstAirDate),
		nullInt(cache.SeasonNumber), nullInt(cache.EpisodeNumber),
		cache.RawData, cache.ExpireAt,
	)

	if err != nil {
		return fmt.Errorf("TmdbCacheDAO[Update] 更新失败: %v", err)
	}
	d.saveToRedis(cache)
	return nil
}

// DeleteExpired 删除过期缓存
// 返回:
//   - int64: 删除数量
//   - error: 错误信息
func (d *TmdbCacheDAO) DeleteExpired() (int64, error) {
	result, err := DB.Exec("DELETE FROM t_tmdb_cache WHERE expire_at < NOW()")
	if err != nil {
		return 0, fmt.Errorf("TmdbCacheDAO[DeleteExpired] 删除失败: %v", err)
	}
	return result.RowsAffected()
}

func (d *TmdbCacheDAO) getFromRedis(key string) (*TmdbCache, bool) {
	client := GetGlobalRedisClient()
	if client == nil {
		return nil, false
	}

	value, err := client.Get(context.Background(), key).Result()
	if err == redis.Nil || value == "" {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	var cache TmdbCache
	if err := json.Unmarshal([]byte(value), &cache); err != nil {
		return nil, false
	}
	if !cache.ExpireAt.IsZero() && cache.ExpireAt.Before(time.Now()) {
		_ = client.Del(context.Background(), key).Err()
		return nil, false
	}

	return &cache, true
}

func (d *TmdbCacheDAO) saveToRedis(cache *TmdbCache) {
	client := GetGlobalRedisClient()
	if client == nil || cache == nil {
		return
	}

	payload, err := json.Marshal(cache)
	if err != nil {
		return
	}

	ttl := time.Until(cache.ExpireAt)
	if ttl <= 0 {
		ttl = tmdbCacheRedisTTL
	}
	ctx := context.Background()
	_ = client.Set(ctx, tmdbQueryCacheKey(cache.QueryKey, cache.MediaType), payload, ttl).Err()
	if cache.TmdbID > 0 {
		_ = client.Set(ctx, tmdbIDCacheKey(cache.TmdbID, cache.MediaType), payload, ttl).Err()
	}
}

func tmdbQueryCacheKey(queryKey, mediaType string) string {
	return tmdbQueryCacheKeyPrefix + strings.ToLower(strings.TrimSpace(mediaType)) + ":" + strings.ToLower(strings.TrimSpace(queryKey))
}

func tmdbIDCacheKey(tmdbID int, mediaType string) string {
	return tmdbIDCacheKeyPrefix + strings.ToLower(strings.TrimSpace(mediaType)) + ":" + strconv.Itoa(tmdbID)
}

// GetByQueryKeys 批量根据查询键获取缓存
// 用于文件列表批量获取TMDB识别结果
// 参数:
//   - queryKeys: 查询键列表（文件ID或文件名）
//
// 返回:
//   - map[string]*TmdbCache: 以query_key为键的缓存映射
//   - error: 错误信息
func (d *TmdbCacheDAO) GetByQueryKeys(queryKeys []string) (map[string]*TmdbCache, error) {
	if len(queryKeys) == 0 {
		return make(map[string]*TmdbCache), nil
	}

	// 构建IN查询参数
	placeholders := ""
	args := make([]interface{}, len(queryKeys))
	for i, key := range queryKeys {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args[i] = key
	}

	query := fmt.Sprintf(
		`SELECT id, query_key, media_type, tmdb_id, title, original_title, year, poster_path, 
		        overview, vote_average, release_date, first_air_date, season_number, episode_number, 
		        raw_data, expire_at, create_time, update_time
		 FROM t_tmdb_cache 
		 WHERE query_key IN (%s) AND expire_at > NOW()`,
		placeholders,
	)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("TmdbCacheDAO[GetByQueryKeys] 查询失败: %v", err)
	}
	defer rows.Close()

	result := make(map[string]*TmdbCache)
	for rows.Next() {
		cache := &TmdbCache{}
		var year sql.NullInt32
		var voteAverage sql.NullFloat64
		var seasonNumber, episodeNumber sql.NullInt32
		var posterPath, overview, releaseDate, firstAirDate sql.NullString
		var rawData []byte

		err := rows.Scan(
			&cache.ID, &cache.QueryKey, &cache.MediaType, &cache.TmdbID, &cache.Title, &cache.OriginalTitle,
			&year, &posterPath, &overview, &voteAverage, &releaseDate, &firstAirDate,
			&seasonNumber, &episodeNumber, &rawData, &cache.ExpireAt, &cache.CreateTime, &cache.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("TmdbCacheDAO[GetByQueryKeys] 扫描失败: %v", err)
		}

		// 处理可空字段
		if year.Valid {
			cache.Year = int(year.Int32)
		}
		if voteAverage.Valid {
			cache.VoteAverage = voteAverage.Float64
		}
		if seasonNumber.Valid {
			cache.SeasonNumber = int(seasonNumber.Int32)
		}
		if episodeNumber.Valid {
			cache.EpisodeNumber = int(episodeNumber.Int32)
		}
		if posterPath.Valid {
			cache.PosterPath = posterPath.String
		}
		if overview.Valid {
			cache.Overview = overview.String
		}
		if releaseDate.Valid {
			cache.ReleaseDate = releaseDate.String
		}
		if firstAirDate.Valid {
			cache.FirstAirDate = firstAirDate.String
		}
		if rawData != nil {
			cache.RawData = rawData
		}

		result[cache.QueryKey] = cache
	}

	return result, nil
}
