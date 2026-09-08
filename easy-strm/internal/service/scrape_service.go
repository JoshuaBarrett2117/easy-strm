package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

const tmdbImageBaseURL = "https://image.tmdb.org/t/p/"

// ScrapeResult 单个文件的刮削结果
type ScrapeResult struct {
	FilePath       string   `json:"file_path"`
	NfoPath        string   `json:"nfo_path"`
	GeneratedFiles []string `json:"generated_files,omitempty"`
	ImagePaths     []string `json:"image_paths,omitempty"`
	Success        bool     `json:"success"`
	Message        string   `json:"message"`
}

// ScrapeService NFO刮削服务
// 负责从TMDB缓存数据生成Kodi/Emby兼容的NFO和本地侧车图片
type ScrapeService struct {
	tmdbCacheDAO      *dao.TmdbCacheDAO
	mediaFileCacheDAO *dao.MediaFileCacheDAO
	mediaSourceDAO    *dao.MediaSourceDAO
	systemConfigDAO   SystemConfigReader
	tmdbService       *TmdbService
	imageBaseURL      string
}

// NewScrapeService 创建NFO刮削服务实例
func NewScrapeService(
	tmdbCacheDAO *dao.TmdbCacheDAO,
	mediaFileCacheDAO *dao.MediaFileCacheDAO,
	mediaSourceDAO *dao.MediaSourceDAO,
	systemConfigDAO SystemConfigReader,
	tmdbService *TmdbService,
) *ScrapeService {
	return &ScrapeService{
		tmdbCacheDAO:      tmdbCacheDAO,
		mediaFileCacheDAO: mediaFileCacheDAO,
		mediaSourceDAO:    mediaSourceDAO,
		systemConfigDAO:   systemConfigDAO,
		tmdbService:       tmdbService,
		imageBaseURL:      "https://image.tmdb.org/t/p/original",
	}
}

type scrapeOutputOptions struct {
	WriteNFO    bool
	WritePoster bool
	WriteFanart bool
	WriteThumb  bool
}

// ScrapeFile 为单个文件刮削NFO和图片侧车
func (s *ScrapeService) ScrapeFile(sourceID int, filePath string) (string, string, []string, error) {
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil {
		return "", "", nil, fmt.Errorf("ScrapeService[ScrapeFile] failed to load media source: %v", err)
	}
	if source == nil {
		return "", "", nil, fmt.Errorf("ScrapeService[ScrapeFile] media source not found: %d", sourceID)
	}
	if source.SourceType == domain.SourceTypeCloud115 {
		return "", "", nil, fmt.Errorf("only local media sources support scraping")
	}

	mediaType, rawData, season, episode, err := s.resolveTmdbData(sourceID, filePath, source.MetadataSource)
	if err != nil {
		return "", "", nil, err
	}

	absMediaPath, displayPath := s.resolveMediaPaths(source.Path, filePath)
	return s.scrapeResolvedFile(absMediaPath, displayPath, mediaType, rawData, season, episode)
}

func (s *ScrapeService) ScrapeAbsoluteFile(sourceID int, filePath string) (string, string, []string, error) {
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil {
		return "", "", nil, fmt.Errorf("ScrapeService[ScrapeAbsoluteFile] failed to load media source: %v", err)
	}
	if source == nil {
		return "", "", nil, fmt.Errorf("ScrapeService[ScrapeAbsoluteFile] media source not found: %d", sourceID)
	}
	if source.SourceType == domain.SourceTypeCloud115 {
		return "", "", nil, fmt.Errorf("only local media sources support scraping")
	}

	relativePath := filePath
	if filepath.IsAbs(filePath) {
		if rel, relErr := filepath.Rel(source.Path, filePath); relErr == nil && !strings.HasPrefix(rel, "..") {
			relativePath = filepath.ToSlash(rel)
		}
	}

	mediaType, rawData, season, episode, err := s.resolveTmdbData(sourceID, relativePath, source.MetadataSource)
	if err != nil {
		return "", "", nil, err
	}

	absMediaPath, displayPath := s.resolveMediaPaths(source.Path, filePath)
	return s.scrapeResolvedFile(absMediaPath, displayPath, mediaType, rawData, season, episode)
}

func (s *ScrapeService) scrapeResolvedFile(absMediaPath, displayPath, mediaType string, rawData json.RawMessage, season, episode int) (string, string, []string, error) {
	options := s.getScrapeOutputOptions()

	var (
		nfoContent     string
		generatedFiles []string
		err            error
	)
	if mediaType == "tv" {
		nfoContent, generatedFiles, err = s.GenerateEpisodeNFO(rawData, filepath.Dir(absMediaPath), filepath.Base(absMediaPath), season, episode, options)
	} else {
		nfoContent, generatedFiles, err = s.GenerateMovieNFO(rawData, filepath.Dir(absMediaPath), filepath.Base(absMediaPath), options)
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("ScrapeService[ScrapeFile] failed to generate NFO: %v", err)
	}

	nfoPath := nfoPathFromMedia(displayPath)
	if options.WriteNFO {
		if err := s.writeNFOFile(absMediaPath, nfoContent); err != nil {
			return "", "", nil, fmt.Errorf("ScrapeService[ScrapeFile] failed to write NFO file: %v", err)
		}
	} else {
		nfoPath = ""
	}

	logger.Infof("[ScrapeService] scrape completed: file=%s, nfo=%s", displayPath, nfoPath)
	return nfoContent, nfoPath, generatedFiles, nil
}

// ScrapeFiles 为多个文件刮削NFO
func (s *ScrapeService) ScrapeFiles(sourceID int, filePaths []string) ([]ScrapeResult, error) {
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("ScrapeService[ScrapeFiles] 获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("ScrapeService[ScrapeFiles] 媒体源不存在: %d", sourceID)
	}
	if source.SourceType == domain.SourceTypeCloud115 {
		return nil, fmt.Errorf("仅本地媒体源支持NFO刮削")
	}

	results := make([]ScrapeResult, 0, len(filePaths))
	for _, fp := range filePaths {
		_, nfoPath, generatedFiles, err := s.ScrapeFile(sourceID, fp)
		if err != nil {
			logger.Warnf("[ScrapeService] 刮削失败: file=%s, err=%v", fp, err)
			results = append(results, ScrapeResult{
				FilePath: fp,
				Success:  false,
				Message:  err.Error(),
			})
			continue
		}

		results = append(results, ScrapeResult{
			FilePath:       fp,
			NfoPath:        nfoPath,
			GeneratedFiles: generatedFiles,
			ImagePaths:     generatedFiles,
			Success:        true,
			Message:        "刮削成功",
		})
	}
	return results, nil
}

// resolveTmdbData 解析文件的TMDB数据，优先使用文件缓存
func (s *ScrapeService) resolveTmdbData(sourceID int, filePath, metadataSource string) (mediaType string, rawData json.RawMessage, season, episode int, err error) {
	fileCache, _ := s.mediaFileCacheDAO.GetByPath(sourceID, filePath)
	if fileCache != nil && len(fileCache.TmdbData) > 0 && metadataPayloadMatchesPolicy(fileCache.TmdbData, metadataSource) {
		return fileCache.MediaType, fileCache.TmdbData, fileCache.SeasonNumber, fileCache.EpisodeNumber, nil
	}

	filename := filepath.Base(filePath)
	identifyResult, identifyErr := s.tmdbService.IdentifyFileWithPathBySource(filePath, metadataSource)
	if identifyErr != nil {
		return "", nil, 0, 0, fmt.Errorf("文件未识别且自动识别失败: %v", identifyErr)
	}
	if !identifyResult.Success {
		return "", nil, 0, 0, fmt.Errorf("文件未识别: %s", identifyResult.Message)
	}

	cacheKey := s.tmdbService.buildCacheKeyForSource(filename, identifyResult.MediaType, metadataSource)
	cache, cacheErr := s.tmdbCacheDAO.GetByQueryKey(cacheKey, identifyResult.MediaType)
	if cacheErr == nil && cache != nil && len(cache.RawData) > 0 {
		return identifyResult.MediaType, cache.RawData, identifyResult.SeasonNumber, identifyResult.EpisodeNumber, nil
	}

	fallbackRawData, buildErr := buildFallbackRawData(identifyResult)
	if buildErr != nil || len(fallbackRawData) == 0 {
		return "", nil, 0, 0, fmt.Errorf("TMDB缓存数据为空，且无法基于识别结果生成刮削数据")
	}

	return identifyResult.MediaType, fallbackRawData, identifyResult.SeasonNumber, identifyResult.EpisodeNumber, nil
}

func metadataPayloadMatchesPolicy(rawData json.RawMessage, metadataSource string) bool {
	policy := normalizeMetadataSourcePolicy(metadataSource)
	if policy == domain.MetadataSourceAuto {
		return true
	}
	var metadata struct {
		Source string `json:"metadata_source"`
	}
	_ = json.Unmarshal(rawData, &metadata)
	if policy == domain.MetadataSourceMetaTube {
		return metadata.Source == domain.MetadataSourceMetaTube
	}
	return metadata.Source != domain.MetadataSourceMetaTube
}

func buildFallbackRawData(result *domain.TmdbIdentifyResult) (json.RawMessage, error) {
	if result == nil || result.TmdbID <= 0 {
		return nil, fmt.Errorf("识别结果不完整")
	}

	payload := map[string]interface{}{
		"id": result.TmdbID,
	}

	if result.MediaType == "tv" {
		if title := strings.TrimSpace(result.Title); title != "" {
			payload["name"] = title
		}
		if originalTitle := strings.TrimSpace(result.OriginalTitle); originalTitle != "" {
			payload["original_name"] = originalTitle
		}
		if result.Year > 0 {
			payload["first_air_date"] = fmt.Sprintf("%04d-01-01", result.Year)
		}
	} else {
		if title := strings.TrimSpace(result.Title); title != "" {
			payload["title"] = title
		}
		if originalTitle := strings.TrimSpace(result.OriginalTitle); originalTitle != "" {
			payload["original_title"] = originalTitle
		}
		if result.Year > 0 {
			payload["release_date"] = fmt.Sprintf("%04d-01-01", result.Year)
		}
	}

	rawData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return rawData, nil
}

// GenerateMovieNFO generates movie NFO content and optional local sidecar artwork.
func (s *ScrapeService) GenerateMovieNFO(rawData json.RawMessage, mediaRoot, mediaPath string, options scrapeOutputOptions) (string, []string, error) {
	var detail tmdbMovieDetail
	if err := json.Unmarshal(rawData, &detail); err != nil {
		return "", nil, fmt.Errorf("ScrapeService[GenerateMovieNFO] failed to parse TMDB payload: %v", err)
	}
	if err := s.enrichMovieDetail(rawData, &detail); err != nil {
		logger.Warnf("[ScrapeService] failed to enrich movie detail: file=%s, err=%v", mediaPath, err)
	}

	title := strings.TrimSpace(detail.Title)
	if title == "" {
		title = strings.TrimSpace(detail.OriginalTitle)
	}
	if title == "" {
		title = "Unknown"
	}

	sortTitle := strings.TrimSpace(detail.OriginalTitle)
	if sortTitle == "" {
		sortTitle = title
	}

	movie := nfoMovie{
		Title:         title,
		OriginalTitle: strings.TrimSpace(detail.OriginalTitle),
		SortTitle:     sortTitle,
		Plot:          detail.Overview,
		Tagline:       detail.Tagline,
		TmdbID:        fmt.Sprintf("%d", detail.ID),
		Rating:        fmt.Sprintf("%.1f", detail.VoteAverage),
		UniqueID:      &nfoUniqueID{Type: "tmdb", Default: "true", Value: fmt.Sprintf("%d", detail.ID)},
	}
	var metadata struct {
		Source   string `json:"metadata_source"`
		ID       string `json:"metadata_id"`
		Provider string `json:"metadata_provider"`
	}
	if json.Unmarshal(rawData, &metadata) == nil && metadata.Source == "metatube" && metadata.ID != "" {
		movie.UniqueID = &nfoUniqueID{Type: "metatube", Default: "true", Value: metadata.ID}
		movie.TmdbID = ""
	}

	if detail.ReleaseDate != "" && len(detail.ReleaseDate) >= 4 {
		movie.Year = detail.ReleaseDate[:4]
		movie.Premiered = detail.ReleaseDate
	}
	if detail.Runtime > 0 {
		movie.Runtime = fmt.Sprintf("%d", detail.Runtime)
	}
	if detail.BelongsToCollection != nil && detail.BelongsToCollection.Name != "" {
		movie.Set = detail.BelongsToCollection.Name
	}

	for _, g := range detail.Genres {
		movie.Genres = append(movie.Genres, g.Name)
	}
	for _, c := range detail.ProductionCountries {
		movie.Countries = append(movie.Countries, c.Name)
	}
	for _, l := range detail.SpokenLanguages {
		movie.Languages = append(movie.Languages, l.Name)
	}
	for _, studio := range detail.ProductionCompanies {
		movie.Studios = append(movie.Studios, studio.Name)
	}
	if detail.Credits != nil {
		movie.Directors = extractDirectors(detail.Credits.Crew)
		movie.Writers = extractWriters(detail.Credits.Crew)
		movie.Actors = extractActors(detail.Credits.Cast)
	}

	var generatedFiles []string
	if options.WritePoster {
		if posters, err := s.downloadArtwork(mediaRoot, mediaPath, "-poster.jpg", detail.PosterPath); err != nil {
			logger.Warnf("[ScrapeService] failed to download movie poster: file=%s, err=%v", mediaPath, err)
		} else if len(posters) > 0 {
			movie.Thumbs = append(movie.Thumbs, nfoThumb{
				Aspect:  "poster",
				Preview: artworkPreviewURL("w500", detail.PosterPath),
				Value:   posters[0],
			})
			generatedFiles = append(generatedFiles, posters...)
		}
	}
	if options.WriteFanart {
		if fanarts, err := s.downloadArtwork(mediaRoot, mediaPath, "-fanart.jpg", detail.BackdropPath); err != nil {
			logger.Warnf("[ScrapeService] failed to download movie fanart: file=%s, err=%v", mediaPath, err)
		} else if len(fanarts) > 0 {
			movie.Fanart = &nfoFanart{
				Thumbs: []nfoFanartThumb{{
					Preview: artworkPreviewURL("w780", detail.BackdropPath),
					Value:   fanarts[0],
				}},
			}
			generatedFiles = append(generatedFiles, fanarts...)
		}
	}

	nfoContent, err := marshalNFO(movie)
	if err != nil {
		return "", nil, err
	}
	return nfoContent, generatedFiles, nil
}

// GenerateEpisodeNFO generates episode NFO content and optional local sidecar artwork.
func (s *ScrapeService) GenerateEpisodeNFO(rawData json.RawMessage, mediaRoot, mediaPath string, season, episode int, options scrapeOutputOptions) (string, []string, error) {
	var tvDetail tmdbTVDetail
	if err := json.Unmarshal(rawData, &tvDetail); err != nil {
		return "", nil, fmt.Errorf("ScrapeService[GenerateEpisodeNFO] failed to parse TMDB payload: %v", err)
	}
	if err := s.enrichTVDetail(rawData, &tvDetail); err != nil {
		logger.Warnf("[ScrapeService] failed to enrich tv detail: file=%s, err=%v", mediaPath, err)
	}

	ep := nfoEpisode{
		ShowTitle: tvDetail.Name,
		Season:    fmt.Sprintf("%d", season),
		Episode:   fmt.Sprintf("%d", episode),
		Plot:      tvDetail.Overview,
		Rating:    fmt.Sprintf("%.1f", tvDetail.VoteAverage),
		TmdbID:    fmt.Sprintf("%d", tvDetail.ID),
		UniqueID:  &nfoUniqueID{Type: "tmdb", Default: "true", Value: fmt.Sprintf("%d", tvDetail.ID)},
	}
	if strings.TrimSpace(tvDetail.Name) == "" {
		ep.ShowTitle = "Unknown"
	}

	episodeDetail, detailErr := s.loadEpisodeDetail(tvDetail.ID, season, episode)
	if detailErr == nil && episodeDetail != nil {
		if title := strings.TrimSpace(episodeDetail.Name); title != "" {
			ep.Title = title
		}
		if plot := strings.TrimSpace(episodeDetail.Overview); plot != "" {
			ep.Plot = plot
		}
		if episodeDetail.AirDate != "" {
			ep.Aired = episodeDetail.AirDate
		}
		ep.Rating = fmt.Sprintf("%.1f", episodeDetail.VoteAverage)
		if episodeDetail.Credits != nil {
			ep.Directors = extractDirectors(episodeDetail.Credits.Crew)
			ep.Writers = extractWriters(episodeDetail.Credits.Crew)
			ep.Actors = extractActors(episodeDetail.Credits.Cast)
		}
	} else {
		if detailErr != nil {
			logger.Warnf("[ScrapeService] failed to load episode detail, fallback to series metadata: tmdb_id=%d s%d e%d err=%v", tvDetail.ID, season, episode, detailErr)
		}
		ep.Title = fmt.Sprintf("S%02dE%02d", season, episode)
		if tvDetail.Credits != nil {
			ep.Directors = extractDirectors(tvDetail.Credits.Crew)
			ep.Writers = extractWriters(tvDetail.Credits.Crew)
			ep.Actors = extractActors(tvDetail.Credits.Cast)
		}
	}

	var generatedFiles []string
	if options.WritePoster {
		if posters, err := s.downloadArtwork(mediaRoot, mediaPath, "-poster.jpg", tvDetail.PosterPath); err != nil {
			logger.Warnf("[ScrapeService] failed to download episode poster: file=%s, err=%v", mediaPath, err)
		} else if len(posters) > 0 {
			ep.Thumbs = append(ep.Thumbs, nfoThumb{
				Aspect:  "poster",
				Preview: artworkPreviewURL("w500", tvDetail.PosterPath),
				Value:   posters[0],
			})
			generatedFiles = append(generatedFiles, posters...)
		}
	}
	if options.WriteFanart {
		if fanarts, err := s.downloadArtwork(mediaRoot, mediaPath, "-fanart.jpg", tvDetail.BackdropPath); err != nil {
			logger.Warnf("[ScrapeService] failed to download episode fanart: file=%s, err=%v", mediaPath, err)
		} else if len(fanarts) > 0 {
			ep.Thumbs = append(ep.Thumbs, nfoThumb{
				Aspect:  "fanart",
				Preview: artworkPreviewURL("w780", tvDetail.BackdropPath),
				Value:   fanarts[0],
			})
			generatedFiles = append(generatedFiles, fanarts...)
		}
	}
	if options.WriteThumb && episodeDetail != nil && episodeDetail.StillPath != "" {
		if thumbs, err := s.downloadArtwork(mediaRoot, mediaPath, "-thumb.jpg", episodeDetail.StillPath); err != nil {
			logger.Warnf("[ScrapeService] failed to download episode thumb: file=%s, err=%v", mediaPath, err)
		} else if len(thumbs) > 0 {
			ep.Thumbs = append(ep.Thumbs, nfoThumb{
				Aspect:  "thumb",
				Preview: artworkPreviewURL("w500", episodeDetail.StillPath),
				Value:   thumbs[0],
			})
			generatedFiles = append(generatedFiles, thumbs...)
		}
	}

	nfoContent, err := marshalNFO(ep)
	if err != nil {
		return "", nil, err
	}
	return nfoContent, generatedFiles, nil
}

func (s *ScrapeService) loadEpisodeDetail(tmdbID, season, episode int) (*tmdbEpisodeDetail, error) {
	if s.tmdbService == nil || tmdbID <= 0 {
		return nil, fmt.Errorf("TMDB服务未初始化")
	}
	detail, err := s.tmdbService.GetTVEpisodeDetail(tmdbID, season, episode)
	if err != nil {
		return nil, err
	}

	ep := &tmdbEpisodeDetail{}
	raw, err := json.Marshal(detail)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, ep); err != nil {
		return nil, err
	}
	if ep.Name == "" && ep.Overview == "" {
		return nil, fmt.Errorf("剧集详情为空")
	}
	return ep, nil
}

func (s *ScrapeService) downloadArtwork(mediaRoot, mediaPath, suffix, imagePath string) ([]string, error) {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return nil, nil
	}

	mediaFullPath := filepath.Join(mediaRoot, mediaPath)
	destNames := sidecarNames(mediaPath, suffix)
	destPath := filepath.Join(filepath.Dir(mediaFullPath), destNames[0])
	if err := s.downloadImageToPath(s.artworkSourceURL(imagePath), destPath); err != nil {
		return nil, err
	}
	for _, name := range destNames[1:] {
		aliasPath := filepath.Join(filepath.Dir(mediaFullPath), name)
		if err := copyFile(destPath, aliasPath); err != nil {
			return nil, err
		}
	}
	return destNames, nil
}

func (s *ScrapeService) resolveMediaPaths(sourceRoot, filePath string) (string, string) {
	if filepath.IsAbs(filePath) {
		return filePath, filepath.Base(filePath)
	}
	return filepath.Join(sourceRoot, filePath), filePath
}

func (s *ScrapeService) getScrapeOutputOptions() scrapeOutputOptions {
	return scrapeOutputOptions{
		WriteNFO:    s.getBoolConfig("scrape_write_nfo", true),
		WritePoster: s.getBoolConfig("scrape_write_poster", true),
		WriteFanart: s.getBoolConfig("scrape_write_fanart", true),
		WriteThumb:  s.getBoolConfig("scrape_write_thumb", true),
	}
}

func (s *ScrapeService) getBoolConfig(key string, defaultValue bool) bool {
	if s.systemConfigDAO == nil {
		return defaultValue
	}
	config, err := s.systemConfigDAO.GetByKey(key)
	if err != nil || config == nil {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(config.ConfigVal)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func (s *ScrapeService) enrichMovieDetail(rawData json.RawMessage, detail *tmdbMovieDetail) error {
	if detail == nil {
		return nil
	}
	if s.tmdbService != nil && s.tmdbService.MetaTubeEnabled() {
		var metadata struct {
			Source   string `json:"metadata_source"`
			ID       string `json:"metadata_id"`
			Provider string `json:"metadata_provider"`
		}
		if json.Unmarshal(rawData, &metadata) == nil && metadata.Source == "metatube" && metadata.ID != "" {
			fullDetail, err := s.tmdbService.metaTubeDetail(metaTubeRef{Provider: metadata.Provider, ID: metadata.ID})
			if err != nil {
				return err
			}
			refreshed, err := json.Marshal(fullDetail)
			if err != nil {
				return err
			}
			return json.Unmarshal(refreshed, detail)
		}
	}
	tmdbID := detail.ID
	if tmdbID <= 0 {
		tmdbID = extractTmdbID(rawData)
	}
	if s.tmdbService == nil || tmdbID <= 0 || !needsMovieDetailRefresh(detail) {
		detail.ID = tmdbID
		return nil
	}
	fullDetail, err := s.tmdbService.GetMovieDetail(tmdbID)
	if err != nil {
		return err
	}
	refreshed, err := json.Marshal(fullDetail)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(refreshed, detail); err != nil {
		return err
	}
	return nil
}

func (s *ScrapeService) enrichTVDetail(rawData json.RawMessage, detail *tmdbTVDetail) error {
	if detail == nil {
		return nil
	}
	tmdbID := detail.ID
	if tmdbID <= 0 {
		tmdbID = extractTmdbID(rawData)
	}
	if s.tmdbService == nil || tmdbID <= 0 || !needsTVDetailRefresh(detail) {
		detail.ID = tmdbID
		return nil
	}
	fullDetail, err := s.tmdbService.GetTVDetail(tmdbID)
	if err != nil {
		return err
	}
	refreshed, err := json.Marshal(fullDetail)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(refreshed, detail); err != nil {
		return err
	}
	return nil
}

func (s *ScrapeService) artworkSourceURL(path string) string {
	if path == "" {
		return ""
	}
	if isRemoteURL(path) {
		return path
	}
	if s.imageBaseURL != "" {
		return s.imageBaseURL + path
	}
	return tmdbImageURL("original", path)
}

func artworkPreviewURL(size, path string) string {
	if path == "" {
		return ""
	}
	if isRemoteURL(path) {
		return path
	}
	return tmdbImageURL(size, path)
}

func (s *ScrapeService) downloadImageToPath(imageURL, destPath string) error {
	if imageURL == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建图片目录失败: %v", err)
	}

	client := http.DefaultClient
	if s.tmdbService != nil && s.tmdbService.httpClient != nil {
		client = s.tmdbService.httpClient
	}

	resp, err := client.Get(imageURL)
	if err != nil {
		return fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载图片失败: HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建图片文件失败: %v", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("写入图片文件失败: %v", err)
	}
	return nil
}
