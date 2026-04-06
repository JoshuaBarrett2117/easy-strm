package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
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
		apiKey:       apiKey,
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
	s.apiKey = apiKey
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
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
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
			Language:      strings.ToLower(movie.OriginalLanguage),
		})
	}

	logger.Infof("TmdbService[SearchMovie] 搜索完成: query=%s, results=%d", query, len(results))
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
	if s.apiKey == "" {
		return nil, fmt.Errorf("TMDB API Key 未配置")
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
			Countries:     normalizeCodes(tv.OriginCountry),
			Language:      strings.ToLower(tv.OriginalLanguage),
		})
	}

	logger.Infof("TmdbService[SearchTV] 搜索完成: query=%s, results=%d", query, len(results))
	return results, nil
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
		applyCachedMetadata(result, cache.RawData)
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

	// 搜索 TMDB
	var candidates []domain.TmdbSearchResult
	var err error

	if parsed.MediaType == "tv" {
		candidates, err = s.SearchTV(parsed.Title, parsed.Year)
	} else {
		candidates, err = s.SearchMovie(parsed.Title, parsed.Year)
	}

	// 智能分词降级搜索逻辑
	if (err == nil && len(candidates) == 0) && parsed.Title != "" {
		logger.Infof("TmdbService[IdentifyFile] 整体搜索无结果，尝试分词回退搜索: %s", parsed.Title)
		candidates, err = s.fallbackTokenizeSearch(parsed.Title, parsed.Year, parsed.MediaType)
	}

	if err != nil {
		result.Success = false
		result.Message = err.Error()
		return result, nil
	}

	if len(candidates) == 0 {
		result.Success = false
		result.Message = "未找到匹配的媒体"
		return result, nil
	}

	// 返回最佳匹配
	best := candidates[0]
	result.Success = true
	result.TmdbID = best.TmdbID
	result.Title = best.Title
	result.OriginalTitle = best.OriginalTitle
	result.Year = best.Year
	result.GenreIDs = best.GenreIDs
	result.Countries = best.Countries
	result.Language = best.Language
	result.Candidates = candidates

	s.enrichIdentifyMetadata(result, &best)

	// 缓存结果
	s.cacheResult(cacheKey, parsed.MediaType, best, parsed.Season, parsed.Episode)

	logger.Infof("TmdbService[IdentifyFile] 识别成功: title=%s, tmdb_id=%d, type=%s",
		result.Title, result.TmdbID, result.MediaType)
	return result, nil
}

// parseFilename 解析文件名，提取媒体信息
// 参数:
//   - filename: 文件名
//
// 返回:
//   - *ParsedFilename: 解析结果
func (s *TmdbService) parseFilename(filename string) *ParsedFilename {
	result := &ParsedFilename{
		MediaType: "movie", // 默认为电影
	}

	// 移除扩展名
	name := filename
	if ext := filepath.Ext(filename); ext != "" {
		name = strings.TrimSuffix(filename, ext)
	}

	// 替换常见分隔符为空格
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// 提取季集信息（剧集判断）
	seasonEpisodePatterns := []string{
		`(?i)S(\d{1,2})E(\d{1,2})`,                     // S01E02
		`(?i)Season\s*(\d{1,2})\s*Episode\s*(\d{1,2})`, // Season 1 Episode 2
		`(?i)(\d{1,2})x(\d{1,2})`,                      // 1x02
		`(?i)第(\d+)季第(\d+)集`,                           // 第1季第2集
		`(?i)EP?(\d{1,3})`,                             // EP02, E02
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
				result.Season = 1 // 默认第一季
			}
			name = re.ReplaceAllString(name, "")
			break
		}
	}

	// 提取年份
	yearPattern := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	if matches := yearPattern.FindStringSubmatch(name); len(matches) > 0 {
		result.Year, _ = strconv.Atoi(matches[1])
		name = strings.Replace(name, matches[0], "", 1)
	}

	// 提取视频质量
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

	// 提取来源
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

	// 提取编码
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

	// 清理标题
	// 移除常见标签
	tagsToRemove := []string{
		`(?i)\bBluRay\b`, `(?i)\bWEB-?DL\b`, `(?i)\bHDTV\b`,
		`(?i)\bBDRip\b`, `(?i)\bDVDRip\b`, `(?i)\bHDRip\b`,
		`(?i)\bx264\b`, `(?i)\bx265\b`, `(?i)\bH\.?264\b`, `(?i)\bH\.?265\b`,
		`(?i)\bHEVC\b`, `(?i)\bAVC\b`, `(?i)\bAAC\b`, `(?i)\bAC3\b`,
		`(?i)\bDTS\b`, `(?i)\bDD5\.?1\b`, `(?i)\b5\.?1\b`,
		`(?i)\b4K\b`, `(?i)\b2160p\b`, `(?i)\b1080p\b`, `(?i)\b720p\b`,
		`(?i)\bAMZN\b`, `(?i)\bNF\b`, `(?i)\bHMAX\b`,
	}
	for _, tag := range tagsToRemove {
		re := regexp.MustCompile(tag)
		name = re.ReplaceAllString(name, "")
	}

	// 移除括号及其包裹的附加信息（如压制组、分辨率说明）
	bracketPattern := regexp.MustCompile(`\[.*?\]|【.*?】|\(.*?\)|（.*?）|<.*?>`)
	name = bracketPattern.ReplaceAllString(name, " ")

	// 移除剩余非字母数字汉字的特殊符号
	nonWordPattern := regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	name = nonWordPattern.ReplaceAllString(name, " ")

	// 清理多余空格和特殊字符
	title := strings.TrimSpace(name)
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	result.Title = title

	return result
}

// fallbackTokenizeSearch 回退分词搜索：分离中文和英文，优先根据中文搜索
func (s *TmdbService) fallbackTokenizeSearch(title string, year int, mediaType string) ([]domain.TmdbSearchResult, error) {
	// 提取中文部分
	hzReg := regexp.MustCompile(`[\p{Han}]+`)
	hzMatches := hzReg.FindAllString(title, -1)
	chineseTitle := strings.Join(hzMatches, " ")

	// 提取英文部分
	enReg := regexp.MustCompile(`[a-zA-Z0-9]+`)
	enMatches := enReg.FindAllString(title, -1)
	englishTitle := strings.Join(enMatches, " ")

	var candidates []domain.TmdbSearchResult
	var err error

	// 1. 如果有中文，优先搜索中文部分
	if strings.TrimSpace(chineseTitle) != "" {
		if mediaType == "tv" {
			candidates, err = s.SearchTV(chineseTitle, year)
		} else {
			candidates, err = s.SearchMovie(chineseTitle, year)
		}
		if err == nil && len(candidates) > 0 {
			logger.Infof("TmdbService[fallbackTokenizeSearch] 中文分词搜索成功: %s", chineseTitle)
			return candidates, nil
		}
	}

	// 2. 如果中文无结果或无中文，且有英文，尝试搜索英文部分
	if strings.TrimSpace(englishTitle) != "" {
		if mediaType == "tv" {
			candidates, err = s.SearchTV(englishTitle, year)
		} else {
			candidates, err = s.SearchMovie(englishTitle, year)
		}
		if err == nil && len(candidates) > 0 {
			logger.Infof("TmdbService[fallbackTokenizeSearch] 英文分词搜索成功: %s", englishTitle)
			return candidates, nil
		}
	}

	return nil, err
}

// ParsedFilename 解析后的文件名信息
type ParsedFilename struct {
	Title     string
	Year      int
	MediaType string // movie | tv
	Season    int
	Episode   int
	Quality   string
	Source    string
	Codec     string
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

func applyCachedMetadata(result *domain.TmdbIdentifyResult, rawData json.RawMessage) {
	if len(rawData) == 0 {
		return
	}

	var cached domain.TmdbSearchResult
	if err := json.Unmarshal(rawData, &cached); err != nil {
		return
	}
	result.GenreIDs = cached.GenreIDs
	result.Countries = cached.Countries
	result.Language = cached.Language
}

func (s *TmdbService) enrichCachedIdentifyMetadata(result *domain.TmdbIdentifyResult, cache *dao.TmdbCache) {
	if result.TmdbID == 0 || len(result.GenreIDs) > 0 {
		return
	}

	best := domain.TmdbSearchResult{
		TmdbID:        cache.TmdbID,
		Title:         cache.Title,
		OriginalTitle: cache.OriginalTitle,
		Year:          cache.Year,
		PosterPath:    cache.PosterPath,
		Overview:      cache.Overview,
		VoteAverage:   cache.VoteAverage,
		MediaType:     cache.MediaType,
		ReleaseDate:   cache.ReleaseDate,
		FirstAirDate:  cache.FirstAirDate,
		GenreIDs:      result.GenreIDs,
		Countries:     result.Countries,
		Language:      result.Language,
	}
	s.enrichIdentifyMetadata(result, &best)
	if len(best.GenreIDs) == 0 {
		return
	}

	rawData, err := json.Marshal(best)
	if err != nil {
		logger.Warnf("TmdbService[enrichCachedIdentifyMetadata] 缓存元数据序列化失败: %v", err)
		return
	}
	cache.RawData = rawData
	cache.ExpireAt = time.Now().AddDate(0, 0, 7)
	if err := s.cacheDAO.Update(cache); err != nil {
		logger.Warnf("TmdbService[enrichCachedIdentifyMetadata] 缓存元数据回写失败: %v", err)
	}
}

func (s *TmdbService) enrichIdentifyMetadata(result *domain.TmdbIdentifyResult, best *domain.TmdbSearchResult) {
	var detail map[string]interface{}
	var err error
	if result.MediaType == "tv" {
		detail, err = s.GetTVDetail(result.TmdbID)
	} else {
		detail, err = s.GetMovieDetail(result.TmdbID)
	}
	if err != nil {
		logger.Warnf("TmdbService[enrichIdentifyMetadata] 获取详情失败，使用搜索结果元数据: %v", err)
		return
	}

	if genreIDs := parseGenreIDs(detail["genres"]); len(genreIDs) > 0 {
		result.GenreIDs = genreIDs
		best.GenreIDs = genreIDs
	}
	if countries := parseCountries(detail); len(countries) > 0 {
		result.Countries = countries
		best.Countries = countries
	}
	if language, ok := detail["original_language"].(string); ok && language != "" {
		result.Language = strings.ToLower(language)
		best.Language = result.Language
	}
}

func parseGenreIDs(raw interface{}) []int {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	ids := make([]int, 0, len(items))
	for _, item := range items {
		genre, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		switch id := genre["id"].(type) {
		case float64:
			ids = append(ids, int(id))
		case int:
			ids = append(ids, id)
		}
	}
	return ids
}

func parseCountries(detail map[string]interface{}) []string {
	if raw, ok := detail["origin_country"].([]interface{}); ok {
		values := make([]string, 0, len(raw))
		for _, item := range raw {
			if country, ok := item.(string); ok {
				values = append(values, country)
			}
		}
		return normalizeCodes(values)
	}

	if raw, ok := detail["production_countries"].([]interface{}); ok {
		values := make([]string, 0, len(raw))
		for _, item := range raw {
			country, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if code, ok := country["iso_3166_1"].(string); ok {
				values = append(values, code)
			}
		}
		return normalizeCodes(values)
	}

	return nil
}

func normalizeCodes(codes []string) []string {
	normalized := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(strings.ToUpper(code))
		if code != "" {
			normalized = append(normalized, code)
		}
	}
	return normalized
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
	// 移除路径中的空格
	path = strings.ReplaceAll(path, " ", "")
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

	return result, nil
}
