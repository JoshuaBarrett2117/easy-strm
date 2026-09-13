package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func (s *OrganizeService) previewFile(source *domain.MediaSource, file domain.MediaFile, targetPath, template string, categories []*domain.MediaCategory, overrideMap map[string]domain.OrganizeManualOverride) (*OrganizePreview, error) {
	identifyResult, err := s.getPreferredIdentifyResult(file, s.matchOrganizeManualOverride(file, overrideMap), source)
	if err != nil {
		return nil, err
	}

	// 如果启用了分类，则动态匹配出 targetPath
	if len(categories) > 0 {
		matchedPath := s.matchCategoryPath(identifyResult, categories)
		if matchedPath != "" {
			targetPath = s.prependCategoryTargetPath(targetPath, matchedPath)
		} else {
			logger.Warnf("OrganizeService[previewFile] 未命中任何分类策略，沿用请求目标目录: %s", targetPath)
		}
	}

	// 生成更名预览
	renameReq := &domain.RenamePreviewRequest{
		SourceID:  source.ID,
		FileID:    file.ID,
		TmdbID:    identifyResult.TmdbID,
		MediaType: identifyResult.MediaType,
		Title:     identifyResult.Title,
		Year:      identifyResult.Year,
		Season:    identifyResult.SeasonNumber,
		Episode:   identifyResult.EpisodeNumber,
		Template:  template,
	}

	renameResult, err := s.renameService.PreviewRename(renameReq)
	if err != nil {
		return nil, fmt.Errorf("生成更名预览失败: %v", err)
	}

	finalTargetPath, newName, newPath := s.buildOrganizeTargetPath(targetPath, renameResult.NewName)
	if source.SourceType == domain.SourceTypeCloud115 {
		finalTargetPath, newName, newPath = s.buildCloud115OrganizeTargetPath(targetPath, renameResult.NewName)
	}
	renameResult.NewName = newName
	conflict := false
	if source.SourceType == domain.SourceTypeLocal {
		_, statErr := os.Stat(newPath)
		conflict = statErr == nil
	}

	return &OrganizePreview{
		FileID:     file.ID,
		CloudID:    file.CID, // 透传 115 内部 ID
		FileName:   file.Name,
		FilePath:   file.Path,
		MediaType:  identifyResult.MediaType,
		TmdbID:     identifyResult.TmdbID,
		Title:      identifyResult.Title,
		Year:       identifyResult.Year,
		Season:     identifyResult.SeasonNumber,
		Episode:    identifyResult.EpisodeNumber,
		NewName:    renameResult.NewName,
		NewPath:    newPath,
		TargetPath: finalTargetPath, // 使用包含 Title 的路径作为实际目标
		Conflict:   conflict,
		ConflictPath: func() string {
			if conflict {
				return newPath
			}
			return ""
		}(),
	}, nil
}

func (s *OrganizeService) getPreferredIdentifyResult(file domain.MediaFile, manualOverride *domain.OrganizeManualOverride, source *domain.MediaSource) (*domain.TmdbIdentifyResult, error) {
	sourceID, metadataSource := 0, domain.MetadataSourceAuto
	if source != nil {
		sourceID, metadataSource = source.ID, source.MetadataSource
	}
	if manualOverride != nil {
		result := s.buildManualIdentifyResult(file, *manualOverride)
		if s.tmdbService != nil {
			s.tmdbService.EnsureIdentifyMetadata(result)
		}
		s.saveIdentifyResultToCache(file, result, sourceID, metadataSource, true)
		return result, nil
	}

	if cached := s.getCachedIdentifyResult(file, metadataSource); cached != nil {
		if s.tmdbService != nil {
			s.tmdbService.EnsureIdentifyMetadata(cached)
		}
		return cached, nil
	}

	identifyResult, err := s.tmdbService.IdentifyFileWithPathBySource(s.identifyInputForFile(file), metadataSource)
	if err != nil {
		return nil, fmt.Errorf("TMDB 识别失败: %v", err)
	}
	if !identifyResult.Success {
		return nil, fmt.Errorf("识别失败: %s", identifyResult.Message)
	}

	if s.tmdbService != nil {
		s.tmdbService.EnsureIdentifyMetadata(identifyResult)
	}
	s.saveIdentifyResultToCache(file, identifyResult, sourceID, metadataSource, false)

	return identifyResult, nil
}

func (s *OrganizeService) identifyInputForFile(file domain.MediaFile) string {
	if path := strings.TrimSpace(file.Path); path != "" {
		return path
	}
	if id := strings.TrimSpace(file.ID); id != "" {
		return id
	}
	return file.Name
}

func (s *OrganizeService) buildOrganizeManualOverrideMap(items []domain.OrganizeManualOverride) map[string]domain.OrganizeManualOverride {
	if len(items) == 0 {
		return nil
	}

	result := make(map[string]domain.OrganizeManualOverride, len(items)*2)
	for _, item := range items {
		if fileID := strings.TrimSpace(item.FileID); fileID != "" {
			result["file:"+s.normalizePath(fileID)] = item
		}
		if cloudID := strings.TrimSpace(item.CloudID); cloudID != "" {
			result["cloud:"+s.normalizePath(cloudID)] = item
		}
	}
	return result
}

func (s *OrganizeService) matchOrganizeManualOverride(file domain.MediaFile, overrideMap map[string]domain.OrganizeManualOverride) *domain.OrganizeManualOverride {
	if len(overrideMap) == 0 {
		return nil
	}

	if fileID := strings.TrimSpace(file.ID); fileID != "" {
		if item, ok := overrideMap["file:"+s.normalizePath(fileID)]; ok {
			return &item
		}
	}
	if cloudID := strings.TrimSpace(file.CID); cloudID != "" {
		if item, ok := overrideMap["cloud:"+s.normalizePath(cloudID)]; ok {
			return &item
		}
	}
	return nil
}

func (s *OrganizeService) buildManualIdentifyResult(file domain.MediaFile, item domain.OrganizeManualOverride) *domain.TmdbIdentifyResult {
	mediaType := strings.TrimSpace(item.MediaType)
	if mediaType == "" {
		mediaType = "movie"
	}

	return &domain.TmdbIdentifyResult{
		Success:          true,
		Message:          "使用手动修改的识别结果",
		Filename:         file.Name,
		MediaType:        mediaType,
		TmdbID:           item.TmdbID,
		Title:            strings.TrimSpace(item.Title),
		OriginalTitle:    strings.TrimSpace(item.OriginalTitle),
		Year:             item.Year,
		SeasonNumber:     item.Season,
		EpisodeNumber:    item.Episode,
		MetadataSource:   item.MetadataSource,
		MetadataID:       item.MetadataID,
		MetadataProvider: item.MetadataProvider,
	}
}

func (s *OrganizeService) getCachedIdentifyResult(file domain.MediaFile, metadataSource string) *domain.TmdbIdentifyResult {
	if s.identifyCacheDAO == nil && (s.tmdbCacheDAO == nil || s.tmdbService == nil) {
		return nil
	}

	fileHash := identifyCacheHash(file.Name, metadataSource)

	if cached := s.getIdentifyCacheFromRedis(fileHash); cached != nil && identifyResultMatchesPolicy(cached, metadataSource) {
		logger.Debugf("OrganizeService[getCachedIdentifyResult] Redis命中: %s", file.Name)
		return cached
	}

	if cached := s.getIdentifyCacheFromDB(fileHash); cached != nil && identifyResultMatchesPolicy(cached, metadataSource) {
		s.saveIdentifyCacheToRedis(fileHash, cached)
		logger.Debugf("OrganizeService[getCachedIdentifyResult] 数据库命中: %s", file.Name)
		return cached
	}

	if s.tmdbCacheDAO != nil && s.tmdbService != nil {
		keys := make([]string, 0, 3)
		for _, key := range []string{file.ID, file.CID, strings.ToLower(strings.TrimSpace(file.Name))} {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			seen := false
			for _, existing := range keys {
				if existing == key {
					seen = true
					break
				}
			}
			if !seen {
				keys = append(keys, key)
			}
		}

		for _, key := range keys {
			for _, mediaType := range []string{"movie", "tv"} {
				cacheKey := s.tmdbService.buildCacheKeyForSource(key, mediaType, metadataSource)
				cache, err := s.tmdbCacheDAO.GetByQueryKey(cacheKey, mediaType)
				if err != nil || cache == nil {
					continue
				}

				result := &domain.TmdbIdentifyResult{
					Success:       true,
					Message:       "使用已有识别结果",
					Filename:      file.Name,
					MediaType:     cache.MediaType,
					TmdbID:        cache.TmdbID,
					Title:         cache.Title,
					OriginalTitle: cache.OriginalTitle,
					Year:          cache.Year,
					SeasonNumber:  cache.SeasonNumber,
					EpisodeNumber: cache.EpisodeNumber,
				}
				applyCachedMetadata(result, cache.RawData)
				s.tmdbService.enrichCachedIdentifyMetadata(result, cache)
				return result
			}
		}
	}

	return nil
}

func (s *OrganizeService) getIdentifyCacheFromRedis(fileHash string) *domain.TmdbIdentifyResult {
	if s.redisClient == nil {
		return nil
	}
	key := fmt.Sprintf("identify:cache:%s", fileHash)
	ctx := context.Background()
	value, err := s.redisClient.Get(ctx, key).Result()
	if err != nil || value == "" {
		return nil
	}
	var cache struct {
		MediaType        string `json:"media_type"`
		TmdbID           int    `json:"tmdb_id"`
		Title            string `json:"title"`
		OriginalTitle    string `json:"original_title"`
		Year             int    `json:"year"`
		SeasonNumber     int    `json:"season_number"`
		EpisodeNumber    int    `json:"episode_number"`
		PosterURL        string `json:"poster_path"`
		IsManual         bool   `json:"is_manual"`
		MetadataSource   string `json:"metadata_source"`
		MetadataID       string `json:"metadata_id"`
		MetadataProvider string `json:"metadata_provider"`
	}
	if err := json.Unmarshal([]byte(value), &cache); err != nil {
		logger.Warnf("OrganizeService[getIdentifyCacheFromRedis] JSON解析失败: %v", err)
		return nil
	}
	return &domain.TmdbIdentifyResult{
		Success:          true,
		Message:          "使用已有识别结果",
		Filename:         "",
		MediaType:        cache.MediaType,
		TmdbID:           cache.TmdbID,
		Title:            cache.Title,
		OriginalTitle:    cache.OriginalTitle,
		Year:             cache.Year,
		SeasonNumber:     cache.SeasonNumber,
		EpisodeNumber:    cache.EpisodeNumber,
		MetadataSource:   cache.MetadataSource,
		MetadataID:       cache.MetadataID,
		MetadataProvider: cache.MetadataProvider,
	}
}

func (s *OrganizeService) getIdentifyCacheFromDB(fileHash string) *domain.TmdbIdentifyResult {
	cache, err := s.identifyCacheDAO.GetByFileHash(fileHash)
	if err != nil || cache == nil {
		return nil
	}
	return &domain.TmdbIdentifyResult{
		Success:       true,
		Message:       "使用已有识别结果",
		Filename:      cache.FileName,
		MediaType:     cache.MediaType,
		TmdbID:        cache.TmdbID,
		Title:         cache.Title,
		OriginalTitle: cache.OriginalTitle,
		Year:          cache.Year,
		SeasonNumber:  cache.SeasonNumber,
		EpisodeNumber: cache.EpisodeNumber,
	}
}

func (s *OrganizeService) saveIdentifyCacheToRedis(fileHash string, result *domain.TmdbIdentifyResult) {
	if s.redisClient == nil {
		return
	}
	key := fmt.Sprintf("identify:cache:%s", fileHash)
	data := map[string]interface{}{
		"media_type":        result.MediaType,
		"tmdb_id":           result.TmdbID,
		"title":             result.Title,
		"original_title":    result.OriginalTitle,
		"year":              result.Year,
		"season_number":     result.SeasonNumber,
		"episode_number":    result.EpisodeNumber,
		"metadata_source":   result.MetadataSource,
		"metadata_id":       result.MetadataID,
		"metadata_provider": result.MetadataProvider,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Warnf("OrganizeService[saveIdentifyCacheToRedis] JSON序列化失败: %v", err)
		return
	}
	ctx := context.Background()
	if err := s.redisClient.Set(ctx, key, string(jsonData), identifyCacheRedisTTL).Err(); err != nil {
		logger.Warnf("OrganizeService[saveIdentifyCacheToRedis] 保存Redis失败: %v", err)
	}
}

func (s *OrganizeService) saveIdentifyCacheToDB(fileHash, fileName string, result *domain.TmdbIdentifyResult, sourceID int, isManual bool) {
	cache := buildIdentifyCache(fileHash, fileName, result, sourceID, isManual)
	if err := s.identifyCacheDAO.CreateOrUpdate(cache); err != nil {
		logger.Warnf("OrganizeService[saveIdentifyCacheToDB] 保存数据库失败: %v", err)
	}
}

func buildIdentifyCache(fileHash, fileName string, result *domain.TmdbIdentifyResult, sourceID int, isManual bool) *dao.IdentifyCache {
	return &dao.IdentifyCache{
		FileHash:      fileHash,
		FileName:      fileName,
		MediaType:     result.MediaType,
		TmdbID:        result.TmdbID,
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		SeasonNumber:  result.SeasonNumber,
		EpisodeNumber: result.EpisodeNumber,
		PosterPath:    result.PosterPath,
		IsManual:      isManual,
		SourceID:      sourceID,
	}
}

func (s *OrganizeService) saveIdentifyResultToCache(file domain.MediaFile, result *domain.TmdbIdentifyResult, sourceID int, metadataSource string, isManual bool) {
	if s.identifyCacheDAO == nil {
		return
	}
	fileHash := identifyCacheHash(file.Name, metadataSource)
	s.saveIdentifyCacheToRedis(fileHash, result)
	s.saveIdentifyCacheToDB(fileHash, file.Name, result, sourceID, isManual)
}

func identifyCacheHash(fileName, metadataSource string) string {
	policy := normalizeMetadataSourcePolicy(metadataSource)
	if policy == domain.MetadataSourceAuto {
		return dao.FileHash(fileName)
	}
	return dao.FileHash(policy + ":" + fileName)
}

func identifyResultMatchesPolicy(result *domain.TmdbIdentifyResult, metadataSource string) bool {
	if result == nil {
		return false
	}
	policy := normalizeMetadataSourcePolicy(metadataSource)
	if policy == domain.MetadataSourceAuto {
		return true
	}
	if policy == domain.MetadataSourceMetaTube {
		return result.MetadataSource == domain.MetadataSourceMetaTube
	}
	return result.MetadataSource != domain.MetadataSourceMetaTube
}
