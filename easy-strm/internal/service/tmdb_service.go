package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

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

// buildSearchQueryVariants 生成搜索变体，兼容中文标点、空格和紧凑片名。
func looksLikeMaskedAPIKey(apiKey string) bool {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return true
	}
	return strings.Contains(trimmed, "****")
}

func buildSearchQueryVariants(title string) []string {
	seen := make(map[string]struct{})
	var queries []string

	add := func(value string) {
		value = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(value, " "))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		queries = append(queries, value)
	}

	add(title)

	normalized := normalizeSearchTitle(title)
	add(normalized)
	add(compactSearchTitle(normalized))
	add(compactSearchTitle(title))

	return queries
}

// normalizeSearchTitle 将标点统一为单空格，方便 TMDB 搜索做词级匹配。
func normalizeSearchTitle(title string) string {
	var builder strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsSpace(r):
			builder.WriteRune(' ')
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			builder.WriteRune(' ')
		default:
			builder.WriteRune(r)
		}
	}

	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(builder.String(), " "))
}

// compactSearchTitle 去掉空格和标点，适合中文片名的紧凑搜索回退。
func compactSearchTitle(title string) string {
	var builder strings.Builder
	for _, r := range title {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// parseFilename 解析文件名，提取媒体信息
// 参数:
//   - filename: 文件名
//
// 返回:
//   - *ParsedFilename: 解析结果
func (s *TmdbService) parseFilename(filename string) *ParsedFilename {
	segments := splitPathSegments(filename)
	result := parseMediaNameSegment(lastPathSegment(segments), true)
	searchTitles := make([]string, 0, 3)
	searchTitles = appendUniqueNonEmpty(searchTitles, result.Title)

	for i := len(segments) - 2; i >= 0 && len(segments)-i <= 3; i-- {
		contextParsed := parseMediaNameSegment(segments[i], false)
		if contextParsed.Year > 0 && result.Year == 0 {
			result.Year = contextParsed.Year
		}
		if result.MediaType == "movie" && contextParsed.MediaType == "tv" && result.Season == 0 && result.Episode == 0 {
			result.MediaType = contextParsed.MediaType
			result.Season = contextParsed.Season
			result.Episode = contextParsed.Episode
		}
		searchTitles = appendUniqueNonEmpty(searchTitles, contextParsed.Title)
	}

	if result.Title == "" && len(searchTitles) > 0 {
		result.Title = searchTitles[0]
	}
	result.SearchTitles = searchTitles
	return result
}

func parseMediaNameSegment(input string, trimExt bool) *ParsedFilename {
	result := &ParsedFilename{
		MediaType: "movie",
	}

	name := strings.TrimSpace(input)
	if trimExt {
		if ext := filepath.Ext(name); ext != "" {
			name = strings.TrimSuffix(name, ext)
		}
	}

	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	seasonEpisodePatterns := []string{
		`(?i)S(\d{1,2})E(\d{1,2})`,
		`(?i)Season\s*(\d{1,2})\s*Episode\s*(\d{1,2})`,
		`(?i)(\d{1,2})x(\d{1,2})`,
		`(?i)第(\d+)季第(\d+)集`,
		`(?i)EP?(\d{1,3})`,
	}

	for _, pattern := range seasonEpisodePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(name); len(matches) >= 2 {
			result.MediaType = "tv"
			if len(matches) >= 3 {
				result.Season, _ = strconv.Atoi(matches[1])
				result.Episode, _ = strconv.Atoi(matches[2])
			} else {
				result.Episode, _ = strconv.Atoi(matches[1])
				result.Season = 1
			}
			name = re.ReplaceAllString(name, "")
			break
		}
	}

	yearPattern := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	if matches := yearPattern.FindStringSubmatch(name); len(matches) > 0 {
		result.Year, _ = strconv.Atoi(matches[1])
		name = strings.Replace(name, matches[0], "", 1)
	}

	qualityPatterns := map[string]string{
		`(?i)\b4K\b`:      "4K",
		`(?i)\b2160p\b`:   "2160p",
		`(?i)\b1080p\b`:   "1080p",
		`(?i)\b720p\b`:    "720p",
		`(?i)\b480p\b`:    "480p",
		`(?i)\bHDTV\b`:    "HDTV",
		`(?i)\bBluRay\b`:  "BluRay",
		`(?i)\bWEB-?DL\b`: "WEB-DL",
		`(?i)\bBDRip\b`:   "BDRip",
	}
	for pattern, quality := range qualityPatterns {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			result.Quality = quality
			break
		}
	}

	sourcePatterns := map[string]string{
		`(?i)\bAMZN\b`: "AMZN",
		`(?i)\bNF\b`:   "NF",
		`(?i)\bHMAX\b`: "HMAX",
		`(?i)\bDSNP\b`: "DSNP",
		`(?i)\bATVP\b`: "ATVP",
	}
	for pattern, source := range sourcePatterns {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			result.Source = source
			break
		}
	}

	codecPatterns := map[string]string{
		`(?i)\bx264\b`:    "x264",
		`(?i)\bx265\b`:    "x265",
		`(?i)\bH\.?264\b`: "H.264",
		`(?i)\bH\.?265\b`: "H.265",
		`(?i)\bHEVC\b`:    "HEVC",
		`(?i)\bAVC\b`:     "AVC",
	}
	for pattern, codec := range codecPatterns {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			result.Codec = codec
			break
		}
	}

	tagsToRemove := []string{
		`(?i)\bREMUX\b`,
		`(?i)\bBluRay\b`, `(?i)\bWEB-?DL\b`, `(?i)\bWEB\s*DL\b`, `(?i)\bHDTV\b`,
		`(?i)\bBDRip\b`, `(?i)\bDVDRip\b`, `(?i)\bHDRip\b`,
		`(?i)\bx264\b`, `(?i)\bx265\b`, `(?i)\bH\.?264\b`, `(?i)\bH\.?265\b`,
		`(?i)\bHEVC\b`, `(?i)\bAVC\b`, `(?i)\bAAC\b`, `(?i)\bAC3\b`,
		`(?i)\bDTS\s*HD\b`, `(?i)\bDTSHD\b`, `(?i)\bTRUEHD\b`, `(?i)\bATMOS\b`, `(?i)\bDTS\b`, `(?i)\bHD\b`,
		`(?i)\bDD5\.?1\b`, `(?i)\bDDP?5\.?1\b`, `(?i)\bDDP?\s*5\s*1\b`, `(?i)\b7\.?1\b`, `(?i)\b7\s*1\b`, `(?i)\b5\.?1\b`,
		`(?i)\bMA\b`,
		`(?i)\b4K\b`, `(?i)\b2160p\b`, `(?i)\b1080p\b`, `(?i)\b720p\b`,
		`(?i)\bAMZN\b`, `(?i)\bNF\b`, `(?i)\bHMAX\b`, `(?i)\bDSNP\b`, `(?i)\bATVP\b`,
		`(?i)\bHQ\b`, `高码率`, `低码率`, `非60帧版`, `60帧版`, `高帧率`,
		`杜比视界`, `杜比`, `国语`, `国粤`, `中字`, `双字`, `内封`, `特效字幕`, `收藏版`,
	}
	for _, tag := range tagsToRemove {
		re := regexp.MustCompile(tag)
		name = re.ReplaceAllString(name, "")
	}

	title := strings.TrimSpace(name)
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	title = trimReleaseGroupSuffix(title)
	result.Title = title
	return result
}

// ParsedFilename 解析后的文件名信息
type ParsedFilename struct {
	Title        string
	Year         int
	MediaType    string // movie | tv
	Season       int
	Episode      int
	Quality      string
	Source       string
	Codec        string
	SearchTitles []string
}

func splitPathSegments(input string) []string {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return nil
	}
	normalized = filepath.ToSlash(normalized)
	normalized = pathpkg.Clean(normalized)
	parts := strings.Split(normalized, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func lastPathSegment(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

func appendUniqueNonEmpty(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func trimReleaseGroupSuffix(title string) string {
	tokens := strings.Fields(strings.TrimSpace(title))
	if len(tokens) <= 1 {
		return strings.TrimSpace(title)
	}

	last := tokens[len(tokens)-1]
	if looksLikeReleaseGroupToken(last) {
		tokens = tokens[:len(tokens)-1]
	}
	return strings.Join(tokens, " ")
}

func looksLikeReleaseGroupToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	if regexp.MustCompile(`[\p{Han}]`).MatchString(token) {
		return true
	}

	if regexp.MustCompile(`^[A-Z0-9]{2,8}$`).MatchString(token) {
		return true
	}

	return false
}

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

// cacheResult 缓存搜索结果
// 参数:
//   - queryKey: 查询键
//   - mediaType: 媒体类型
//   - result: 搜索结果
//   - season: 季数
//   - episode: 集数
func (s *TmdbService) cacheResult(queryKey, mediaType string, result domain.TmdbSearchResult, season, episode int) {
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
	// 从 RawData 中解析额外字段补充到结果
	if len(cache.RawData) == 0 {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(cache.RawData, &raw); err != nil {
		return
	}
	// 提取 genre_ids
	if genreIDs := extractGenreIDsFromDetail(raw); len(genreIDs) > 0 {
		result.GenreIDs = genreIDs
	}
	// 提取 origin_country 作为 Countries
	if countries := extractCountriesFromDetail(raw, result.MediaType); len(countries) > 0 {
		result.Countries = countries
	}
	// 提取 original_language
	if lang, ok := raw["original_language"].(string); ok {
		result.Language = lang
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
	if genreIDs := extractGenreIDsFromDetail(raw); len(genreIDs) > 0 {
		result.GenreIDs = genreIDs
	}
	if countries := extractCountriesFromDetail(raw, result.MediaType); len(countries) > 0 {
		result.Countries = countries
	}
	if lang, ok := raw["original_language"].(string); ok {
		result.Language = lang
	}
}

// getImageURL 获取完整图片 URL
// 参数:
//   - path: 图片路径
//
// 返回:
//   - string: 完整 URL
func (s *TmdbService) getImageURL(path string) string {
	if path == "" {
		return ""
	}
	return s.imageBaseURL + path
}

// GetMovieDetail 获取电影详情
// 参数:
//   - tmdbID: TMDB ID
//
// 返回:
//   - map[string]interface{}: 电影详情
//   - error: 错误信息
func (s *TmdbService) GetMovieDetail(tmdbID int) (map[string]interface{}, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("movie", tmdbID, 0, 0); ok {
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/movie/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, s.apiKey, s.language)

	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	s.saveDetailCache("movie", tmdbID, 0, 0, result)
	return result, nil
}

// GetTVDetail 获取剧集详情
// 参数:
//   - tmdbID: TMDB ID
//
// 返回:
//   - map[string]interface{}: 剧集详情
//   - error: 错误信息
func (s *TmdbService) GetTVDetail(tmdbID int) (map[string]interface{}, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("tv", tmdbID, 0, 0); ok {
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/tv/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, s.apiKey, s.language)

	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	s.saveDetailCache("tv", tmdbID, 0, 0, result)
	return result, nil
}

// GetTVEpisodeDetail 获取剧集某一集的详情
func (s *TmdbService) GetTVEpisodeDetail(tmdbID, season, episode int) (map[string]interface{}, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
	}
	if detail, ok := s.loadDetailCache("tv_episode", tmdbID, season, episode); ok {
		return detail, nil
	}

	apiURL := fmt.Sprintf("%s/tv/%d/season/%d/episode/%d?api_key=%s&language=%s",
		s.baseURL, tmdbID, season, episode, s.apiKey, s.language)

	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("TMDB API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API 返回错误: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 TMDB 响应失败: %v", err)
	}

	s.saveDetailCache("tv_episode", tmdbID, season, episode, result)
	return result, nil
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
	return fmt.Sprintf("%s%s:%s:%d:%s", tmdbSearchKeyPrefix, strings.ToLower(strings.TrimSpace(mediaType)), s.language, year, strings.ToLower(strings.TrimSpace(query)))
}

func (s *TmdbService) detailCacheKey(kind string, tmdbID, season, episode int) string {
	return fmt.Sprintf("%s%s:%s:%d:%d:%d", tmdbDetailKeyPrefix, strings.ToLower(strings.TrimSpace(kind)), s.language, tmdbID, season, episode)
}
