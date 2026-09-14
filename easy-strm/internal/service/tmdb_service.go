package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// TmdbService TMDB 服务
// 负责与 TMDB API 交互，提供媒体信息识别功能
type TmdbService struct {
	aiRecognition          *AIRecognitionService
	apiKey                 string
	baseURL                string
	imageBaseURL           string
	language               string
	cacheDAO               *dao.TmdbCacheDAO
	httpClient             *http.Client
	metatubeURL            string
	metatubeToken          string
	metatubeDefaultEnabled bool
	adultContentEnabled    bool
	metatubeRefs           sync.Map

	filenameRuleMu        sync.RWMutex
	filenameRuleStore     FilenameRecognitionRuleStore
	filenameRulesLoaded   bool
	filenameRules         []FilenameRecognitionRule
	compiledFilenameRules []compiledFilenameRecognitionRule
}

// NewTmdbService 创建 TMDB 服务实例
// 参数:
//   - apiKey: TMDB API Key
//   - cacheDAO: TMDB 缓存 DAO
//
// 返回:
//   - *TmdbService: TMDB 服务实例
func NewTmdbService(apiKey string, cacheDAO *dao.TmdbCacheDAO) *TmdbService {
	return &TmdbService{
		apiKey:       strings.TrimSpace(apiKey),
		baseURL:      "https://api.themoviedb.org/3",
		imageBaseURL: "https://image.tmdb.org/t/p/w500",
		language:     "zh-CN",
		cacheDAO:     cacheDAO,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetAPIKey 设置 API Key
// 参数:
//   - apiKey: TMDB API Key
func (s *TmdbService) SetAPIKey(apiKey string) {
	s.apiKey = strings.TrimSpace(apiKey)
}

// SetMetaTubeConfig 设置本地 MetaTube 服务地址及可选访问令牌。
func (s *TmdbService) SetMetaTubeConfig(baseURL, token string) {
	s.metatubeURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s.metatubeToken = strings.TrimSpace(token)
}

// MetaTubeEnabled 返回是否已配置本地 MetaTube 地址。
func (s *TmdbService) MetaTubeEnabled() bool { return s.metatubeURL != "" }

// SetMetaTubeDefaultEnabled 设置自动策略是否默认选择 MetaTube。
func (s *TmdbService) SetMetaTubeDefaultEnabled(enabled bool) { s.metatubeDefaultEnabled = enabled }

// SetAdultContentEnabled 设置是否允许成人内容识别及 MetaTube 数据源。
func (s *TmdbService) SetAdultContentEnabled(enabled bool) {
	s.adultContentEnabled = enabled
	if !enabled {
		s.metatubeDefaultEnabled = false
	}
}

// AdultContentEnabled 返回成人内容能力是否已显式启用。
func (s *TmdbService) AdultContentEnabled() bool { return s.adultContentEnabled }

// GetMetaTubeConfig 返回当前进程中的 MetaTube 配置，供设置热更新保留未填写的令牌。
func (s *TmdbService) GetMetaTubeConfig() (string, string) { return s.metatubeURL, s.metatubeToken }

// SetLanguage 设置语言
// 参数:
//   - language: 语言代码（如 zh-CN, en）
func (s *TmdbService) SetLanguage(language string) {
	s.language = language
}

// SetHTTPClient 设置 TMDB 请求使用的 HTTP 客户端。
// 传入 nil 时保留当前客户端，避免运行期意外切回无超时的默认客户端。
func (s *TmdbService) SetHTTPClient(httpClient *http.Client) {
	if httpClient != nil {
		s.httpClient = httpClient
	}
}

// GetAPIKey 获取 API Key
// 返回:
//   - string: API Key
func (s *TmdbService) GetAPIKey() string {
	return s.apiKey
}

// HasUsableAPIKey 判断当前 API Key 是否为可用的真实值
func (s *TmdbService) HasUsableAPIKey() bool {
	return !looksLikeMaskedAPIKey(s.apiKey)
}

// GetLanguage 获取语言
// 返回:
//   - string: 语言代码
func (s *TmdbService) GetLanguage() string {
	return s.language
}

// SearchMovie 搜索电影，返回 Top 3 候选
// 参数:
//   - query: 搜索关键词
//   - year: 年份（可选）
//
// 返回:
//   - []domain.TmdbSearchResult: 搜索结果列表
//   - error: 错误信息
func (s *TmdbService) SearchMovie(query string, year int) ([]domain.TmdbSearchResult, error) {
	return s.SearchMovieBySource(query, year, domain.MetadataSourceAuto)
}

// SearchMovieBySource 按指定元数据来源搜索电影。
func (s *TmdbService) SearchMovieBySource(query string, year int, metadataSource string) ([]domain.TmdbSearchResult, error) {
	metadataSource = normalizeMetadataSourcePolicy(metadataSource)
	if metadataSource == domain.MetadataSourceMetaTube {
		if !s.adultContentEnabled {
			return nil, fmt.Errorf("成人内容识别未启用，请先在系统设置中二次确认开启")
		}
		if !s.MetaTubeEnabled() {
			return nil, fmt.Errorf("MetaTube 未启用或服务地址未配置")
		}
		return s.searchMetaTube(query, year, "movie")
	}

	results, tmdbErr := s.searchMovieTMDB(query, year)
	if tmdbErr != nil {
		return nil, tmdbErr
	}
	if metadataSource == domain.MetadataSourceTMDB || len(results) > 0 {
		return results, nil
	}

	// 自动策略固定优先 TMDB；仅在 TMDB 没有可用结果且成人模式与 MetaTube 均启用时回退。
	if !s.adultContentEnabled || !s.metatubeDefaultEnabled || !s.MetaTubeEnabled() {
		return results, nil
	}
	metaTubeResults, metaTubeErr := s.searchMetaTube(query, year, "movie")
	if metaTubeErr != nil {
		return nil, metaTubeErr
	}
	return metaTubeResults, nil
}

// GetMovieDetailBySource 按识别结果携带的来源获取电影详情。
// MetaTube 使用 provider/id 作为稳定标识；缺少原始标识时回退到搜索阶段缓存的合成 ID 映射。
func (s *TmdbService) GetMovieDetailBySource(tmdbID int, metadataSource, metadataID, metadataProvider string) (map[string]interface{}, error) {
	if normalizeMetadataSourcePolicy(metadataSource) == domain.MetadataSourceMetaTube || metadataID != "" || metadataProvider != "" {
		if metadataID == "" || metadataProvider == "" {
			if value, ok := s.metatubeRefs.Load(tmdbID); ok {
				ref := value.(metaTubeRef)
				metadataProvider, metadataID = ref.Provider, ref.ID
			}
		}
		if metadataID == "" || metadataProvider == "" {
			return nil, fmt.Errorf("MetaTube 识别结果缺少 provider 或 id")
		}
		return s.metaTubeDetail(metaTubeRef{Provider: metadataProvider, ID: metadataID})
	}
	return s.GetMovieDetail(tmdbID)
}

func (s *TmdbService) searchMovieTMDB(query string, year int) ([]domain.TmdbSearchResult, error) {
	return s.searchMovieTMDBContext(context.Background(), query, year)
}

func (s *TmdbService) searchMovieTMDBContext(ctx context.Context, query string, year int) ([]domain.TmdbSearchResult, error) {
	if !s.HasUsableAPIKey() {
		return nil, fmt.Errorf("TMDB API Key 未配置或已失效，请重新填写真实的 API Key")
	}
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if results, ok := s.loadSearchCache("movie", query, year); ok {
		return results, nil
	}

	// 构建请求 URL
	apiURL := fmt.Sprintf("%s/search/movie?api_key=%s&language=%s&query=%s",
		s.baseURL, s.apiKey, s.language, url.QueryEscape(query))

	if year > 0 {
		apiURL += fmt.Sprintf("&year=%d", year)
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.Errorf("TmdbService[SearchMovie] 请求失败: %v", err)
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Errorf("TmdbService[SearchMovie] API 返回错误: %s", string(body))
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Results []struct {
			ID               int     `json:"id"`
			Title            string  `json:"title"`
			OriginalTitle    string  `json:"original_title"`
			ReleaseDate      string  `json:"release_date"`
			PosterPath       string  `json:"poster_path"`
			Overview         string  `json:"overview"`
			VoteAverage      float64 `json:"vote_average"`
			GenreIDs         []int   `json:"genre_ids"`
			OriginalLanguage string  `json:"original_language"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Errorf("TmdbService[SearchMovie] 解析响应失败: %v", err)
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	// 转换结果，最多返回 3 个
	var results []domain.TmdbSearchResult
	for i, movie := range result.Results {
		if i >= 3 {
			break
		}
		year := 0
		if movie.ReleaseDate != "" && len(movie.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(movie.ReleaseDate[:4])
		}
		results = append(results, domain.TmdbSearchResult{
			TmdbID:         movie.ID,
			Title:          movie.Title,
			OriginalTitle:  movie.OriginalTitle,
			Year:           year,
			PosterPath:     s.getImageURL(movie.PosterPath),
			Overview:       movie.Overview,
			VoteAverage:    movie.VoteAverage,
			MediaType:      "movie",
			ReleaseDate:    movie.ReleaseDate,
			GenreIDs:       movie.GenreIDs,
			Language:       movie.OriginalLanguage,
			MetadataSource: domain.MetadataSourceTMDB,
		})
	}

	logger.Infof("TmdbService[SearchMovie] 搜索完成: query=%s, results=%d", query, len(results))
	s.saveSearchCache("movie", query, year, results)
	return results, nil
}

// SearchTV 搜索剧集，返回 Top 3 候选
// 参数:
//   - query: 搜索关键词
//   - year: 年份（可选）
//
// 返回:
//   - []domain.TmdbSearchResult: 搜索结果列表
//   - error: 错误信息
func (s *TmdbService) SearchTV(query string, year int) ([]domain.TmdbSearchResult, error) {
	return s.searchTVContext(context.Background(), query, year)
}

func (s *TmdbService) searchTVContext(ctx context.Context, query string, year int) ([]domain.TmdbSearchResult, error) {
	if !s.HasUsableAPIKey() {
		return nil, fmt.Errorf("TMDB API Key 未配置或已失效，请重新填写真实的 API Key")
	}
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if results, ok := s.loadSearchCache("tv", query, year); ok {
		return results, nil
	}

	// 构建请求 URL
	apiURL := fmt.Sprintf("%s/search/tv?api_key=%s&language=%s&query=%s",
		s.baseURL, s.apiKey, s.language, url.QueryEscape(query))

	if year > 0 {
		apiURL += fmt.Sprintf("&first_air_date_year=%d", year)
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.Errorf("TmdbService[SearchTV] 请求失败: %v", err)
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Errorf("TmdbService[SearchTV] API 返回错误: %s", string(body))
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Results []struct {
			ID               int      `json:"id"`
			Name             string   `json:"name"`
			OriginalName     string   `json:"original_name"`
			FirstAirDate     string   `json:"first_air_date"`
			PosterPath       string   `json:"poster_path"`
			Overview         string   `json:"overview"`
			VoteAverage      float64  `json:"vote_average"`
			GenreIDs         []int    `json:"genre_ids"`
			OriginCountry    []string `json:"origin_country"`
			OriginalLanguage string   `json:"original_language"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Errorf("TmdbService[SearchTV] 解析响应失败: %v", err)
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	// 转换结果，最多返回 3 个
	var results []domain.TmdbSearchResult
	for i, tv := range result.Results {
		if i >= 3 {
			break
		}
		year := 0
		if tv.FirstAirDate != "" && len(tv.FirstAirDate) >= 4 {
			year, _ = strconv.Atoi(tv.FirstAirDate[:4])
		}
		results = append(results, domain.TmdbSearchResult{
			TmdbID:         tv.ID,
			Title:          tv.Name,
			OriginalTitle:  tv.OriginalName,
			Year:           year,
			PosterPath:     s.getImageURL(tv.PosterPath),
			Overview:       tv.Overview,
			VoteAverage:    tv.VoteAverage,
			MediaType:      "tv",
			FirstAirDate:   tv.FirstAirDate,
			GenreIDs:       tv.GenreIDs,
			Countries:      tv.OriginCountry,
			Language:       tv.OriginalLanguage,
			MetadataSource: domain.MetadataSourceTMDB,
		})
	}

	logger.Infof("TmdbService[SearchTV] 搜索完成: query=%s, results=%d", query, len(results))
	s.saveSearchCache("tv", query, year, results)
	return results, nil
}

// GetCandidates 获取识别候选结果（不自动缓存）
// 与 IdentifyFile 不同，此方法始终搜索 TMDB 并返回 Top 3 候选，
// 不会写入缓存，由用户选择后再通过 Identify 端点绑定。
// 参数:
//   - filename: 文件名
//
// 返回:
//   - *domain.TmdbIdentifyResult: 识别结果（含 Candidates）
//   - error: 错误信息
func (s *TmdbService) GetCandidates(filename string) (*domain.TmdbIdentifyResult, error) {
	return s.getCandidates(filename, domain.MetadataSourceAuto)
}

func (s *TmdbService) getCandidates(filename, metadataSource string) (*domain.TmdbIdentifyResult, error) {
	logger.Infof("TmdbService[GetCandidates] 开始获取候选: %s", filename)

	parsed := s.parseFilename(filename)

	result := &domain.TmdbIdentifyResult{
		Filename:      filename,
		MediaType:     parsed.MediaType,
		Quality:       parsed.Quality,
		Source:        parsed.Source,
		Codec:         parsed.Codec,
		SeasonNumber:  parsed.Season,
		EpisodeNumber: parsed.Episode,
	}

	if parsed.Title == "" {
		result.Success = false
		result.Message = "无法从文件名中解析出标题"
		return result, nil
	}

	// 始终搜索 TMDB，不读缓存，确保返回最新候选
	candidates, err := s.searchCandidatesWithFallback(parsed, metadataSource)

	if err != nil {
		result.Success = false
		result.Message = err.Error()
		return result, nil
	}

	if len(candidates) == 0 {
		result.Success = false
		result.Message = "未找到匹配的媒体信息"
		return result, nil
	}

	// 填充最佳匹配信息（第一个候选）和完整候选列表
	best := candidates[0]
	result.Success = true
	result.TmdbID = best.TmdbID
	result.Title = best.Title
	result.OriginalTitle = best.OriginalTitle
	result.Year = best.Year
	result.PosterPath = best.PosterPath
	result.Candidates = candidates
	result.MetadataSource = best.MetadataSource
	result.MetadataID = best.MetadataID
	result.MetadataProvider = best.MetadataProvider

	logger.Infof("TmdbService[GetCandidates] 获取候选成功: title=%s, candidates=%d, type=%s",
		result.Title, len(candidates), result.MediaType)
	return result, nil
}

// GetCandidatesWithPath 基于文件路径及其目录上下文获取候选结果。
func (s *TmdbService) GetCandidatesWithPath(filePath string) (*domain.TmdbIdentifyResult, error) {
	logger.Infof("TmdbService[GetCandidatesWithPath] 开始获取候选: %s", filePath)
	return s.GetCandidates(filePath)
}

// GetCandidatesWithPathBySource 按元数据来源获取候选结果。
func (s *TmdbService) GetCandidatesWithPathBySource(filePath, metadataSource string) (*domain.TmdbIdentifyResult, error) {
	return s.getCandidates(filePath, metadataSource)
}

// GetCandidatesWithPathBySourceAndType 按调用方明确的媒体类型解析并查询候选，不写识别缓存。
func (s *TmdbService) GetCandidatesWithPathBySourceAndType(filePath, metadataSource, mediaType string) (*domain.TmdbIdentifyResult, error) {
	parsed := s.parseFilenameForMediaType(filePath, mediaType)
	result := &domain.TmdbIdentifyResult{
		Filename: filePath, MediaType: parsed.MediaType, Quality: parsed.Quality, Source: parsed.Source,
		Codec: parsed.Codec, SeasonNumber: parsed.Season, EpisodeNumber: parsed.Episode,
	}
	if parsed.Title == "" {
		result.Message = "无法从文件名中解析出标题"
		return result, nil
	}
	candidates, err := s.searchCandidatesWithFallback(parsed, metadataSource)
	if err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if len(candidates) == 0 {
		result.Message = "未找到匹配的媒体信息"
		return result, nil
	}
	best := candidates[0]
	result.Success, result.Candidates = true, candidates
	result.TmdbID, result.Title, result.OriginalTitle, result.Year = best.TmdbID, best.Title, best.OriginalTitle, best.Year
	result.PosterPath = best.PosterPath
	result.MetadataSource, result.MetadataID, result.MetadataProvider = best.MetadataSource, best.MetadataID, best.MetadataProvider
	return result, nil
}

// IdentifyFile 识别文件，自动判断是电影还是剧集
// 参数:
//   - filename: 文件名
//
// 返回:
//   - *domain.TmdbIdentifyResult: 识别结果
//   - error: 错误信息
func (s *TmdbService) IdentifyFile(filename string) (*domain.TmdbIdentifyResult, error) {
	return s.identifyFile(filename, domain.MetadataSourceAuto)
}

func (s *TmdbService) identifyFile(filename, metadataSource string, mediaTypes ...string) (*domain.TmdbIdentifyResult, error) {
	logger.Infof("TmdbService[IdentifyFile] 开始识别文件: %s", filename)
	metadataSource = normalizeMetadataSourcePolicy(metadataSource)

	// 解析文件名
	parsed := s.parseFilename(filename)
	if len(mediaTypes) > 0 && (mediaTypes[0] == "movie" || mediaTypes[0] == "tv") {
		parsed = s.parseFilenameForMediaType(filename, mediaTypes[0])
	}

	result := &domain.TmdbIdentifyResult{
		Filename:      filename,
		MediaType:     parsed.MediaType,
		Quality:       parsed.Quality,
		Source:        parsed.Source,
		Codec:         parsed.Codec,
		SeasonNumber:  parsed.Season,
		EpisodeNumber: parsed.Episode,
	}

	// 如果没有解析出标题，返回失败
	if parsed.Title == "" {
		result.Success = false
		result.Message = "无法从文件名中解析出标题"
		return result, nil
	}

	// 检查缓存
	cacheKey := s.buildCacheKeyForSource(filename, parsed.MediaType, metadataSource)
	if s.cacheDAO != nil {
		if cache, err := s.cacheDAO.GetByQueryKey(cacheKey, parsed.MediaType); err == nil && cache != nil {
			result.Success = true
			result.TmdbID = cache.TmdbID
			result.Title = cache.Title
			result.OriginalTitle = cache.OriginalTitle
			result.Year = cache.Year
			result.PosterPath = cache.PosterPath
			if cache.SeasonNumber > 0 {
				result.SeasonNumber = cache.SeasonNumber
			}
			if cache.EpisodeNumber > 0 {
				result.EpisodeNumber = cache.EpisodeNumber
			}
			s.enrichCachedIdentifyMetadata(result, cache)
			logger.Infof("TmdbService[IdentifyFile] 使用缓存: title=%s, tmdb_id=%d", result.Title, result.TmdbID)
			return result, nil
		}
	}

	// 搜索 TMDB
	candidates, err := s.searchCandidatesWithFallback(parsed, metadataSource)

	if err != nil {
		result.Success = false
		result.Message = err.Error()
		return result, nil
	}

	if len(candidates) == 0 {
		result.Success = false
		result.Message = "未找到匹配的媒体信息"
		return result, nil
	}

	// 返回最佳匹配
	best := candidates[0]
	result.Success = true
	result.TmdbID = best.TmdbID
	result.Title = best.Title
	result.OriginalTitle = best.OriginalTitle
	result.Year = best.Year
	result.PosterPath = best.PosterPath
	result.Candidates = candidates
	result.MetadataSource = best.MetadataSource
	result.MetadataID = best.MetadataID
	result.MetadataProvider = best.MetadataProvider

	// 缓存结果
	s.cacheResult(cacheKey, parsed.MediaType, best, parsed.Season, parsed.Episode)

	logger.Infof("TmdbService[IdentifyFile] 识别成功: title=%s, tmdb_id=%d, type=%s",
		result.Title, result.TmdbID, result.MediaType)
	return result, nil
}

// IdentifyFileWithPath 基于文件路径及其目录上下文识别媒体信息。
func (s *TmdbService) IdentifyFileWithPath(filePath string) (*domain.TmdbIdentifyResult, error) {
	logger.Infof("TmdbService[IdentifyFileWithPath] 开始识别文件: %s", filePath)
	return s.IdentifyFile(filePath)
}

// IdentifyFileWithPathBySource 按媒体源策略识别文件。
func (s *TmdbService) IdentifyFileWithPathBySource(filePath, metadataSource string) (*domain.TmdbIdentifyResult, error) {
	return s.identifyFile(filePath, metadataSource)
}

// IdentifyFileWithPathBySourceAndType 按元数据策略及调用方明确的媒体类型识别文件。
func (s *TmdbService) IdentifyFileWithPathBySourceAndType(filePath, metadataSource, mediaType string) (*domain.TmdbIdentifyResult, error) {
	return s.identifyFile(filePath, metadataSource, mediaType)
}

// searchCandidatesWithFallback 按多个标题变体搜索 TMDB，提升中文标题和标点差异的命中率。
func (s *TmdbService) searchCandidatesWithFallback(parsed *ParsedFilename, metadataSource string) ([]domain.TmdbSearchResult, error) {
	if parsed == nil || parsed.Title == "" {
		return nil, nil
	}

	queries := make([]string, 0)
	for _, title := range parsed.SearchTitles {
		queries = append(queries, buildSearchQueryVariants(title)...)
	}
	if len(queries) == 0 {
		queries = buildSearchQueryVariants(parsed.Title)
	}

	seen := make(map[string]struct{}, len(queries))
	orderedQueries := make([]string, 0, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		if _, ok := seen[query]; ok {
			continue
		}
		seen[query] = struct{}{}
		orderedQueries = append(orderedQueries, query)
	}

	for i, query := range orderedQueries {
		if i > 0 {
			logger.Infof("TmdbService[searchCandidatesWithFallback] 原始查询无结果，尝试回退查询: %s", query)
		}

		var (
			candidates []domain.TmdbSearchResult
			err        error
		)
		if parsed.MediaType == "tv" {
			candidates, err = s.SearchTV(query, parsed.Year)
		} else {
			candidates, err = s.SearchMovieBySource(query, parsed.Year, metadataSource)
		}
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 {
			return candidates, nil
		}
	}

	return nil, nil
}

func normalizeMetadataSourcePolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case domain.MetadataSourceTMDB:
		return domain.MetadataSourceTMDB
	case domain.MetadataSourceMetaTube:
		return domain.MetadataSourceMetaTube
	default:
		return domain.MetadataSourceAuto
	}
}
