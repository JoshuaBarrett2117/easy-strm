package dao

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	IdentifyCacheExpireDays = 30
)

// IdentifyCacheDAO 文件识别缓存数据访问层
type IdentifyCacheDAO struct{}

// NewIdentifyCacheDAO 创建文件识别缓存DAO实例
func NewIdentifyCacheDAO() *IdentifyCacheDAO {
	return &IdentifyCacheDAO{}
}

// IdentifyCache 文件识别缓存模型
type IdentifyCache struct {
	ID            int        `json:"id"`
	FileHash     string     `json:"file_hash"`
	FileName     string     `json:"file_name"`
	MediaType    string     `json:"media_type"`
	TmdbID       int        `json:"tmdb_id"`
	Title        string     `json:"title"`
	OriginalTitle string     `json:"original_title"`
	Year         int        `json:"year"`
	SeasonNumber int        `json:"season_number"`
	EpisodeNumber int       `json:"episode_number"`
	PosterPath   string     `json:"poster_path"`
	IsManual     bool       `json:"is_manual"`
	SourceID     int        `json:"source_id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// FileHash 生成文件名的MD5哈希
func FileHash(fileName string) string {
	hash := md5.Sum([]byte(strings.ToLower(fileName)))
	return hex.EncodeToString(hash[:])
}

// GetByFileHash 根据文件hash获取缓存
// 参数:
//   - fileHash: 文件名hash
// 返回:
//   - *IdentifyCache: 缓存数据
//   - error: 错误信息
func (d *IdentifyCacheDAO) GetByFileHash(fileHash string) (*IdentifyCache, error) {
	cache := &IdentifyCache{}
	var tmdbID sql.NullInt64
	var year, seasonNumber, episodeNumber sql.NullInt32
	var posterPath, originalTitle sql.NullString
	var sourceID sql.NullInt64

	err := DB.QueryRow(
		`SELECT id, file_hash, file_name, media_type, tmdb_id, title, original_title, year,
		        season_number, episode_number, poster_path, is_manual, source_id, created_at, updated_at
		 FROM t_identify_cache
		 WHERE file_hash = $1`,
		fileHash,
	).Scan(
		&cache.ID, &cache.FileHash, &cache.FileName, &cache.MediaType, &tmdbID,
		&cache.Title, &originalTitle, &year, &seasonNumber, &episodeNumber,
		&posterPath, &cache.IsManual, &sourceID, &cache.CreatedAt, &cache.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("IdentifyCacheDAO[GetByFileHash] 查询失败: %v", err)
	}

	if tmdbID.Valid {
		cache.TmdbID = int(tmdbID.Int64)
	}
	if year.Valid {
		cache.Year = int(year.Int32)
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
	if originalTitle.Valid {
		cache.OriginalTitle = originalTitle.String
	}
	if sourceID.Valid {
		cache.SourceID = int(sourceID.Int64)
	}

	return cache, nil
}

// GetByFileHashes 批量根据文件hash获取缓存
// 参数:
//   - fileHashes: 文件名hash列表
// 返回:
//   - map[string]*IdentifyCache: 以file_hash为键的缓存映射
//   - error: 错误信息
func (d *IdentifyCacheDAO) GetByFileHashes(fileHashes []string) (map[string]*IdentifyCache, error) {
	if len(fileHashes) == 0 {
		return make(map[string]*IdentifyCache), nil
	}

	placeholders := ""
	args := make([]interface{}, len(fileHashes))
	for i, hash := range fileHashes {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += fmt.Sprintf("$%d", i+1)
		args[i] = hash
	}

	query := fmt.Sprintf(
		`SELECT id, file_hash, file_name, media_type, tmdb_id, title, original_title, year,
		        season_number, episode_number, poster_path, is_manual, source_id, created_at, updated_at
		 FROM t_identify_cache
		 WHERE file_hash IN (%s)`,
		placeholders,
	)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("IdentifyCacheDAO[GetByFileHashes] 查询失败: %v", err)
	}
	defer rows.Close()

	result := make(map[string]*IdentifyCache)
	for rows.Next() {
		cache := &IdentifyCache{}
		var tmdbID sql.NullInt64
		var year, seasonNumber, episodeNumber sql.NullInt32
		var posterPath, originalTitle sql.NullString
		var sourceID sql.NullInt64

		err := rows.Scan(
			&cache.ID, &cache.FileHash, &cache.FileName, &cache.MediaType, &tmdbID,
			&cache.Title, &originalTitle, &year, &seasonNumber, &episodeNumber,
			&posterPath, &cache.IsManual, &sourceID, &cache.CreatedAt, &cache.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("IdentifyCacheDAO[GetByFileHashes] 扫描失败: %v", err)
		}

		if tmdbID.Valid {
			cache.TmdbID = int(tmdbID.Int64)
		}
		if year.Valid {
			cache.Year = int(year.Int32)
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
		if originalTitle.Valid {
			cache.OriginalTitle = originalTitle.String
		}
		if sourceID.Valid {
			cache.SourceID = int(sourceID.Int64)
		}

		result[cache.FileHash] = cache
	}

	return result, nil
}

// Create 创建缓存
// 参数:
//   - cache: 缓存数据
// 返回:
//   - error: 错误信息
func (d *IdentifyCacheDAO) Create(cache *IdentifyCache) error {
	err := DB.QueryRow(
		`INSERT INTO t_identify_cache (file_hash, file_name, media_type, tmdb_id, title, original_title, year,
		        season_number, episode_number, poster_path, is_manual, source_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id, created_at, updated_at`,
		cache.FileHash, cache.FileName, cache.MediaType, nullInt(cache.TmdbID),
		cache.Title, nullString(cache.OriginalTitle), nullInt(cache.Year),
		nullInt(cache.SeasonNumber), nullInt(cache.EpisodeNumber), nullString(cache.PosterPath),
		cache.IsManual, nullInt(cache.SourceID),
	).Scan(&cache.ID, &cache.CreatedAt, &cache.UpdatedAt)

	if err != nil {
		return fmt.Errorf("IdentifyCacheDAO[Create] 创建失败: %v", err)
	}
	return nil
}

// CreateOrUpdate 创建或更新缓存
// 参数:
//   - cache: 缓存数据
// 返回:
//   - error: 错误信息
func (d *IdentifyCacheDAO) CreateOrUpdate(cache *IdentifyCache) error {
	err := DB.QueryRow(
		`INSERT INTO t_identify_cache (file_hash, file_name, media_type, tmdb_id, title, original_title, year,
		        season_number, episode_number, poster_path, is_manual, source_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (file_hash) DO UPDATE SET
		        file_name = EXCLUDED.file_name,
		        media_type = EXCLUDED.media_type,
		        tmdb_id = EXCLUDED.tmdb_id,
		        title = EXCLUDED.title,
		        original_title = EXCLUDED.original_title,
		        year = EXCLUDED.year,
		        season_number = EXCLUDED.season_number,
		        episode_number = EXCLUDED.episode_number,
		        poster_path = EXCLUDED.poster_path,
		        is_manual = EXCLUDED.is_manual,
		        source_id = COALESCE(t_identify_cache.source_id, EXCLUDED.source_id),
		        updated_at = NOW()
		 RETURNING id, created_at, updated_at`,
		cache.FileHash, cache.FileName, cache.MediaType, nullInt(cache.TmdbID),
		cache.Title, nullString(cache.OriginalTitle), nullInt(cache.Year),
		nullInt(cache.SeasonNumber), nullInt(cache.EpisodeNumber), nullString(cache.PosterPath),
		cache.IsManual, nullInt(cache.SourceID),
	).Scan(&cache.ID, &cache.CreatedAt, &cache.UpdatedAt)

	if err != nil {
		return fmt.Errorf("IdentifyCacheDAO[CreateOrUpdate] 创建/更新失败: %v", err)
	}
	return nil
}

// DeleteOlderThan 删除超过指定天数的非手动识别记录
// 参数:
//   - days: 天数
// 返回:
//   - int64: 删除数量
//   - error: 错误信息
func (d *IdentifyCacheDAO) DeleteOlderThan(days int) (int64, error) {
	result, err := DB.Exec(
		`DELETE FROM t_identify_cache
		 WHERE is_manual = false AND created_at < NOW() - INTERVAL '1 day' * $1`,
		days,
	)
	if err != nil {
		return 0, fmt.Errorf("IdentifyCacheDAO[DeleteOlderThan] 删除失败: %v", err)
	}
	return result.RowsAffected()
}

// GetCount 获取缓存总数
// 返回:
//   - int64: 缓存数量
//   - error: 错误信息
func (d *IdentifyCacheDAO) GetCount() (int64, error) {
	var count int64
	err := DB.QueryRow("SELECT COUNT(*) FROM t_identify_cache").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("IdentifyCacheDAO[GetCount] 查询失败: %v", err)
	}
	return count, nil
}

// GetCountByType 按类型统计缓存数量
// 参数:
//   - isManual: 是否手动识别
// 返回:
//   - int64: 缓存数量
//   - error: 错误信息
func (d *IdentifyCacheDAO) GetCountByType(isManual bool) (int64, error) {
	var count int64
	err := DB.QueryRow(
		"SELECT COUNT(*) FROM t_identify_cache WHERE is_manual = $1",
		isManual,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("IdentifyCacheDAO[GetCountByType] 查询失败: %v", err)
	}
	return count, nil
}
