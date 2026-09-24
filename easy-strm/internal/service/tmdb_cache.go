package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	"github.com/go-redis/redis/v8"
)

const (
	tmdbSearchRedisTTL  = 6 * time.Hour
	tmdbDetailRedisTTL  = 24 * time.Hour
	tmdbSearchKeyPrefix = "easy_strm:tmdb:search:"
	tmdbDetailKeyPrefix = "easy_strm:tmdb:detail:"
)

// buildCacheKey 构建缓存键
// 参数:
//   - filename: 文件名
//   - mediaType: 媒体类型
//
// 返回:
//   - string: 缓存键
func (s *TmdbService) buildCacheKey(filename, mediaType string) string {
	return strings.ToLower(filename)
}

func (s *TmdbService) buildCacheKeyForSource(filename, mediaType, metadataSource string) string {
	base := s.buildCacheKey(filename, mediaType)
	policy := normalizeMetadataSourcePolicy(metadataSource)
	if policy == domain.MetadataSourceAuto {
		return base
	}
	return policy + ":" + base
}

// CacheKeyForSource 返回指定元数据来源的缓存键。
func (s *TmdbService) CacheKeyForSource(filename, mediaType, metadataSource string) string {
	return s.buildCacheKeyForSource(filename, mediaType, metadataSource)
}

// cacheResult 缓存搜索结果
// 参数:
//   - queryKey: 查询键
//   - mediaType: 媒体类型
//   - result: 搜索结果
//   - season: 季数
//   - episode: 集数
func (s *TmdbService) cacheResult(queryKey, mediaType string, result domain.TmdbSearchResult, season, episode int) {
	if s.cacheDAO == nil {
		return
	}
	// 序列化原始数据
	rawData, _ := json.Marshal(result)

	cache := &dao.TmdbCache{
		QueryKey:      queryKey,
		MediaType:     mediaType,
		TmdbID:        result.TmdbID,
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		PosterPath:    result.PosterPath,
		Overview:      result.Overview,
		VoteAverage:   result.VoteAverage,
		ReleaseDate:   result.ReleaseDate,
		FirstAirDate:  result.FirstAirDate,
		SeasonNumber:  season,
		EpisodeNumber: episode,
		RawData:       rawData,
		ExpireAt:      time.Now().AddDate(0, 0, 7), // 7天过期
	}

	if err := s.cacheDAO.Create(cache); err != nil {
		logger.Warnf("TmdbService[cacheResult] 缓存失败: %v", err)
	}
}

// enrichCachedIdentifyMetadata 从缓存补充识别结果的额外元数据
// 参数:
//   - result: 识别结果（会被原地修改）
//   - cache: TMDB 缓存记录
func (s *TmdbService) enrichCachedIdentifyMetadata(result *domain.TmdbIdentifyResult, cache *dao.TmdbCache) {
	if cache == nil || result == nil {
		return
	}
	result.PosterPath = cache.PosterPath
	// 从 RawData 中解析额外字段补充到结果
	if len(cache.RawData) == 0 {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(cache.RawData, &raw); err != nil {
		return
	}
	if poster, ok := raw["poster_path"].(string); ok && poster != "" {
		result.PosterPath = poster
	}
	// 提取 genre_ids
	if genreIDs := extractGenreIDsFromDetail(raw); len(genreIDs) > 0 {
		result.GenreIDs = genreIDs
	}
	if rating, ok := raw["vote_average"].(float64); ok && rating >= 0 && rating <= 10 {
		result.VoteAverage = &rating
	}
	// 提取 origin_country 作为 Countries
	if countries := extractCountriesFromDetail(raw, result.MediaType); len(countries) > 0 {
		result.Countries = countries
	}
	// 提取 original_language
	if lang, ok := raw["original_language"].(string); ok {
		result.Language = lang
	}
	result.MetadataSource, _ = raw["metadata_source"].(string)
	result.MetadataID, _ = raw["metadata_id"].(string)
	result.MetadataProvider, _ = raw["metadata_provider"].(string)
	if result.MetadataSource == "metatube" && result.MetadataID != "" {
		s.metatubeRefs.Store(result.TmdbID, metaTubeRef{Provider: result.MetadataProvider, ID: result.MetadataID})
	}
}

// applyCachedMetadata 从缓存的 RawData 中提取元数据应用到识别结果
// 参数:
//   - result: 识别结果（会被原地修改）
//   - rawData: 缓存的原始 JSON 数据
func applyCachedMetadata(result *domain.TmdbIdentifyResult, rawData json.RawMessage) {
	if len(rawData) == 0 || result == nil {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(rawData, &raw); err != nil {
		return
	}
	if poster, ok := raw["poster_path"].(string); ok && poster != "" {
		result.PosterPath = poster
	}
	if genreIDs := extractGenreIDsFromDetail(raw); len(genreIDs) > 0 {
		result.GenreIDs = genreIDs
	}
	if countries := extractCountriesFromDetail(raw, result.MediaType); len(countries) > 0 {
		result.Countries = countries
	}
	if lang, ok := raw["original_language"].(string); ok {
		result.Language = lang
	}
	result.MetadataSource, _ = raw["metadata_source"].(string)
	result.MetadataID, _ = raw["metadata_id"].(string)
	result.MetadataProvider, _ = raw["metadata_provider"].(string)
}

func (s *TmdbService) loadSearchCache(mediaType, query string, year int) ([]domain.TmdbSearchResult, bool) {
	client := dao.GetGlobalRedisClient()
	if client == nil {
		return nil, false
	}

	value, err := client.Get(context.Background(), s.searchCacheKey(mediaType, query, year)).Result()
	if err == redis.Nil || value == "" {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	var results []domain.TmdbSearchResult
	if err := json.Unmarshal([]byte(value), &results); err != nil {
		return nil, false
	}
	return results, true
}

func (s *TmdbService) saveSearchCache(mediaType, query string, year int, results []domain.TmdbSearchResult) {
	client := dao.GetGlobalRedisClient()
	if client == nil {
		return
	}

	payload, err := json.Marshal(results)
	if err != nil {
		return
	}
	_ = client.Set(context.Background(), s.searchCacheKey(mediaType, query, year), payload, tmdbSearchRedisTTL).Err()
}

func (s *TmdbService) loadDetailCache(kind string, tmdbID, season, episode int) (map[string]interface{}, bool) {
	client := dao.GetGlobalRedisClient()
	if client == nil {
		return nil, false
	}

	value, err := client.Get(context.Background(), s.detailCacheKey(kind, tmdbID, season, episode)).Result()
	if err == redis.Nil || value == "" {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	var detail map[string]interface{}
	if err := json.Unmarshal([]byte(value), &detail); err != nil {
		return nil, false
	}
	return detail, true
}

func (s *TmdbService) saveDetailCache(kind string, tmdbID, season, episode int, detail map[string]interface{}) {
	client := dao.GetGlobalRedisClient()
	if client == nil || detail == nil {
		return
	}

	payload, err := json.Marshal(detail)
	if err != nil {
		return
	}
	_ = client.Set(context.Background(), s.detailCacheKey(kind, tmdbID, season, episode), payload, tmdbDetailRedisTTL).Err()
}

func (s *TmdbService) searchCacheKey(mediaType, query string, year int) string {
	return fmt.Sprintf("%s%x:%s:%s:%d:%s", tmdbSearchKeyPrefix, sha256.Sum256([]byte(s.baseURL)), strings.ToLower(strings.TrimSpace(mediaType)), s.language, year, strings.ToLower(strings.TrimSpace(query)))
}

func (s *TmdbService) detailCacheKey(kind string, tmdbID, season, episode int) string {
	return fmt.Sprintf("%s%x:%s:%s:%d:%d:%d", tmdbDetailKeyPrefix, sha256.Sum256([]byte(s.baseURL)), strings.ToLower(strings.TrimSpace(kind)), s.language, tmdbID, season, episode)
}
