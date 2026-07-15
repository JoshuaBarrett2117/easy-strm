package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// TmdbService TMDB 服务
// 负责与 TMDB API 交互，提供媒体信息识别功能
type TmdbService struct {
	apiKey       string
	baseURL      string
	imageBaseURL string
	language     string
	cacheDAO     *dao.TmdbCacheDAO
	httpClient   *http.Client
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

// SetLanguage 设置语言
// 参数:
//   - language: 语言代码（如 zh-CN, en）
func (s *TmdbService) SetLanguage(language string) {
	s.language = language
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
	resp, err := s.httpClient.Get(apiURL)
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
			TmdbID:        movie.ID,
			Title:         movie.Title,
			OriginalTitle: movie.OriginalTitle,
			Year:          year,
			PosterPath:    s.getImageURL(movie.PosterPath),
			Overview:      movie.Overview,
			VoteAverage:   movie.VoteAverage,
			MediaType:     "movie",
			ReleaseDate:   movie.ReleaseDate,
			GenreIDs:      movie.GenreIDs,
			Language:      movie.OriginalLanguage,
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
	resp, err := s.httpClient.Get(apiURL)
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
			TmdbID:        tv.ID,
			Title:         tv.Name,
			OriginalTitle: tv.OriginalName,
			Year:          year,
			PosterPath:    s.getImageURL(tv.PosterPath),
			Overview:      tv.Overview,
			VoteAverage:   tv.VoteAverage,
			MediaType:     "tv",
			FirstAirDate:  tv.FirstAirDate,
			GenreIDs:      tv.GenreIDs,
			Countries:     tv.OriginCountry,
			Language:      tv.OriginalLanguage,
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
	candidates, err := s.searchCandidatesWithFallback(parsed)

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
	result.Candidates = candidates

	logger.Infof("TmdbService[GetCandidates] 获取候选成功: title=%s, candidates=%d, type=%s",
		result.Title, len(candidates), result.MediaType)
	return result, nil
}

// GetCandidatesWithPath 基于文件路径及其目录上下文获取候选结果。
func (s *TmdbService) GetCandidatesWithPath(filePath string) (*domain.TmdbIdentifyResult, error) {
	logger.Infof("TmdbService[GetCandidatesWithPath] 开始获取候选: %s", filePath)
	return s.GetCandidates(filePath)
}

// IdentifyFile 识别文件，自动判断是电影还是剧集
// 参数:
//   - filename: 文件名
//
// 返回:
//   - *domain.TmdbIdentifyResult: 识别结果
//   - error: 错误信息
func (s *TmdbService) IdentifyFile(filename string) (*domain.TmdbIdentifyResult, error) {
	logger.Infof("TmdbService[IdentifyFile] 开始识别文件: %s", filename)

	// 解析文件名
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

	// 如果没有解析出标题，返回失败
	if parsed.Title == "" {
		result.Success = false
		result.Message = "无法从文件名中解析出标题"
		return result, nil
	}

	// 检查缓存
	cacheKey := s.buildCacheKey(filename, parsed.MediaType)
	if cache, err := s.cacheDAO.GetByQueryKey(cacheKey, parsed.MediaType); err == nil && cache != nil {
		result.Success = true
		result.TmdbID = cache.TmdbID
		result.Title = cache.Title
		result.OriginalTitle = cache.OriginalTitle
		result.Year = cache.Year
		if cache.SeasonNumber > 0 {
			result.SeasonNumber = cache.SeasonNumber
		}
		if cache.EpisodeNumber > 0 {
			result.EpisodeNumber = cache.EpisodeNumber
		}
		logger.Infof("TmdbService[IdentifyFile] 使用缓存: title=%s, tmdb_id=%d", result.Title, result.TmdbID)
		return result, nil
	}

	// 搜索 TMDB
	candidates, err := s.searchCandidatesWithFallback(parsed)

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
	result.Candidates = candidates

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

// searchCandidatesWithFallback 按多个标题变体搜索 TMDB，提升中文标题和标点差异的命中率。
func (s *TmdbService) searchCandidatesWithFallback(parsed *ParsedFilename) ([]domain.TmdbSearchResult, error) {
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
			candidates, err = s.SearchMovie(query, parsed.Year)
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
