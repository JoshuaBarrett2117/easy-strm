package dao

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
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

	return cache, nil
}

func (d *TmdbCacheDAO) GetByTmdbID(tmdbID int, mediaType string) (*TmdbCache, error) {
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

// ============================================
// 更名预设 DAO
// ============================================

// RenamePresetDAO 更名预设数据访问层
type RenamePresetDAO struct{}

// NewRenamePresetDAO 创建更名预设DAO实例
func NewRenamePresetDAO() *RenamePresetDAO {
	return &RenamePresetDAO{}
}

// RenamePreset 更名预设模型
type RenamePreset struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	MediaType  string    `json:"media_type"`
	Template   string    `json:"template"`
	Enabled    bool      `json:"enabled"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// GetByID 根据ID获取更名预设
// 参数:
//   - id: 预设ID
//
// 返回:
//   - *RenamePreset: 预设数据
//   - error: 错误信息
func (d *RenamePresetDAO) GetByID(id int) (*RenamePreset, error) {
	preset := &RenamePreset{}
	err := DB.QueryRow(
		`SELECT id, name, media_type, template, enabled, create_time, update_time
		 FROM t_rename_preset WHERE id = $1`,
		id,
	).Scan(
		&preset.ID, &preset.Name, &preset.MediaType, &preset.Template,
		&preset.Enabled, &preset.CreateTime, &preset.UpdateTime,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("RenamePresetDAO[GetByID] 查询失败: %v", err)
	}
	return preset, nil
}

// GetByMediaType 根据媒体类型获取预设列表
// 参数:
//   - mediaType: 媒体类型（movie/tv）
//
// 返回:
//   - []*RenamePreset: 预设列表
//   - error: 错误信息
func (d *RenamePresetDAO) GetByMediaType(mediaType string) ([]*RenamePreset, error) {
	rows, err := DB.Query(
		`SELECT id, name, media_type, template, enabled, create_time, update_time
		 FROM t_rename_preset 
		 WHERE media_type = $1 AND enabled = true
		 ORDER BY id ASC`,
		mediaType,
	)
	if err != nil {
		return nil, fmt.Errorf("RenamePresetDAO[GetByMediaType] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*RenamePreset
	for rows.Next() {
		preset := &RenamePreset{}
		err := rows.Scan(
			&preset.ID, &preset.Name, &preset.MediaType, &preset.Template,
			&preset.Enabled, &preset.CreateTime, &preset.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("RenamePresetDAO[GetByMediaType] 扫描失败: %v", err)
		}
		list = append(list, preset)
	}
	return list, nil
}

// GetAll 获取所有预设
// 返回:
//   - []*RenamePreset: 预设列表
//   - error: 错误信息
func (d *RenamePresetDAO) GetAll() ([]*RenamePreset, error) {
	rows, err := DB.Query(
		`SELECT id, name, media_type, template, enabled, create_time, update_time
		 FROM t_rename_preset ORDER BY media_type, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("RenamePresetDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*RenamePreset
	for rows.Next() {
		preset := &RenamePreset{}
		err := rows.Scan(
			&preset.ID, &preset.Name, &preset.MediaType, &preset.Template,
			&preset.Enabled, &preset.CreateTime, &preset.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("RenamePresetDAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, preset)
	}
	return list, nil
}

// Create 创建预设
// 参数:
//   - preset: 预设数据
//
// 返回:
//   - error: 错误信息
func (d *RenamePresetDAO) Create(preset *RenamePreset) error {
	err := DB.QueryRow(
		`INSERT INTO t_rename_preset (name, media_type, template, enabled)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, create_time, update_time`,
		preset.Name, preset.MediaType, preset.Template, preset.Enabled,
	).Scan(&preset.ID, &preset.CreateTime, &preset.UpdateTime)

	if err != nil {
		return fmt.Errorf("RenamePresetDAO[Create] 创建失败: %v", err)
	}
	return nil
}

// Update 更新预设
// 参数:
//   - preset: 预设数据
//
// 返回:
//   - error: 错误信息
func (d *RenamePresetDAO) Update(preset *RenamePreset) error {
	_, err := DB.Exec(
		`UPDATE t_rename_preset SET name = $2, media_type = $3, template = $4, enabled = $5, update_time = NOW()
		 WHERE id = $1`,
		preset.ID, preset.Name, preset.MediaType, preset.Template, preset.Enabled,
	)

	if err != nil {
		return fmt.Errorf("RenamePresetDAO[Update] 更新失败: %v", err)
	}
	return nil
}

// Delete 删除预设
// 参数:
//   - id: 预设ID
//
// 返回:
//   - error: 错误信息
func (d *RenamePresetDAO) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM t_rename_preset WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("RenamePresetDAO[Delete] 删除失败: %v", err)
	}
	return nil
}

// ============================================
// 媒体文件缓存 DAO
// ============================================

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
// 参数:
//   - sourceID: 媒体源ID
//   - filePath: 文件路径
//
// 返回:
//   - *MediaFileCache: 缓存数据
//   - error: 错误信息
func (d *MediaFileCacheDAO) GetByPath(sourceID int, filePath string) (*MediaFileCache, error) {
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

	return cache, nil
}

// CreateOrUpdate 创建或更新文件缓存
// 参数:
//   - cache: 缓存数据
//
// 返回:
//   - error: 错误信息
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
	return nil
}

// DeleteBySourceID 删除指定媒体源的所有缓存
// 参数:
//   - sourceID: 媒体源ID
//
// 返回:
//   - error: 错误信息
func (d *MediaFileCacheDAO) DeleteBySourceID(sourceID int) error {
	_, err := DB.Exec("DELETE FROM t_media_file_cache WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("MediaFileCacheDAO[DeleteBySourceID] 删除失败: %v", err)
	}
	return nil
}

// 辅助函数：创建可空字符串
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// 辅助函数：创建可空整数
func nullInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

// 辅助函数：创建可空时间
func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
