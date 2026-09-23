package service

import (
	"context"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	shareYearRE       = regexp.MustCompile(`(?:^|[ ._（(\[])(19[0-9]{2}|20[0-9]{2})(?:[ ._）)\]]|$)`)
	shareIDRE         = regexp.MustCompile(`(?i)[\[{]tmdb(?:id)?[ =:-]+([0-9]+)[\]}]`)
	shareParentYearRE = regexp.MustCompile(`[（(](19[0-9]{2}|20[0-9]{2})[）)]`)
	shareSeasonRE     = regexp.MustCompile(`(?i)\bS[0-9]{1,2}\b|\bSeason[ ._-]*[0-9]+\b|第[一二三四五六七八九十百0-9]+季`)
	shareSeriesRE     = regexp.MustCompile(`(?i)s[0-9]{1,2}e[0-9]{1,3}|\bS[0-9]{1,2}\b|\bSeason[ ._-]*[0-9]+\b|第[一二三四五六七八九十百0-9]+季|全[一二三四五六七八九十0-9]+季|[0-9]+集全|complete[ ._-]*series|tv[ ._-]*series`)
	shareDiscRE       = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:d|disc|disk|cd)[ ._-]*[0-9]{1,3}(?:[ ._-]|$)`)
	shareBracketRE    = regexp.MustCompile(`\[([^\]]+)\]|【([^】]+)】`)
	shareReleaseRE    = regexp.MustCompile(`(?i)\b(?:ULTRAHD|UHD|Blu[ .-]?ray|WEB[ .-]?DL|BDRip|DVD|REMUX|2160p|1080p|720p|HEVC|AVC|H26[45]|DTS|AAC|HDTV)\b|原盘DIY|蓝光原盘|DIY|简繁|国粤|国语|特效字幕|次世代|菜单修改`)
	shareIndexRE      = regexp.MustCompile(`^[0-9]{1,4}[.、][ ]*`)
	shareContainerRE  = regexp.MustCompile(`(?i)^(?:电影[ ]*)?iso[- _]?[0-9]*$|^(?:原盘电影合集|电影合集|原盘合集|整理好的圆盘|4k原盘|蓝光ISO原盘|IMDB[ ]*Top[ ]*250|CC版|BD[- ]ISO)(?:[ （(0-9]|$)`)
	shareEditionRE    = regexp.MustCompile(`[ ]*(?:法版|西班牙版|美国豪华版|蓝光|全集|全套|三碟|双碟).*$`)
)

// ShareMediaQuery 是原始路径派生的临时查询，绝不修改文件名或网盘数据。
type ShareMediaQuery struct {
	YearFromDirectory bool     `json:"year_from_directory"`
	TmdbID            int      `json:"tmdb_id"`
	Titles            []string `json:"titles"`
	Year              int      `json:"year"`
	MediaType         string   `json:"media_type"`
	Complex           bool     `json:"complex"`
	Container         bool     `json:"container"`
}

func shareContainerName(name string) bool {
	name = strings.TrimSpace(strings.Trim(name, "【】[]"))
	name = strings.NewReplacer("】【", " ", "][", " ").Replace(name)
	return shareContainerRE.MatchString(name) || strings.Contains(name, "整理好的圆盘") || name == "综艺" || name == "电影" || name == "电视剧"
}

func cleanShareTitles(name string) ([]string, int) {
	name = shareIDRE.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "_", " ")
	name = shareIndexRE.ReplaceAllString(name, "")
	year := 0
	yearRE := shareYearRE
	if shareParentYearRE.MatchString(name) {
		yearRE = shareParentYearRE
	}
	if m := yearRE.FindStringSubmatch(name); m != nil {
		fmt.Sscanf(m[1], "%d", &year)
	}
	titles := []string{}
	add := func(value string) {
		value = shareSeasonRE.ReplaceAllString(value, " ")
		value = shareDiscRE.ReplaceAllString(value, " ")
		value = strings.NewReplacer(".", " ", "_", " ").Replace(value)
		if idx := yearRE.FindStringIndex(value); idx != nil {
			value = value[:idx[0]]
		}
		value = strings.Trim(value, " .-[]【】()（）:：")
		value = shareEditionRE.ReplaceAllString(value, "")
		value = strings.Join(strings.Fields(value), " ")
		if value != "" && !shareContainerName(value) {
			titles = appendImportCandidate(titles, value)
		}
	}
	// 首个有片名意义的方括号通常是真正中文名，其他发行信息不参与查询。
	for _, m := range shareBracketRE.FindAllStringSubmatch(name, -1) {
		value := m[1]
		if value == "" {
			value = m[2]
		}
		if shareReleaseRE.MatchString(value) || strings.Contains(value, "GB") || strings.Contains(value, "TB") || strings.HasPrefix(value, "@") || !strings.ContainsFunc(value, unicode.IsLetter) {
			continue
		}
		if i := yearRE.FindStringIndex(value); i != nil {
			value = value[:i[0]]
		}
		add(value)
		for i, r := range value {
			if i > 0 && r < 128 && unicode.IsLetter(r) && strings.ContainsFunc(value[:i], func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
				add(value[:i])
				add(value[i:])
				break
			}
		}
		break
	}
	// 原始主体保留到年份或发行参数之前，支持残缺的方括号与下划线文件名。
	body := name
	if idx := strings.Index(body, "["); idx >= 0 && idx > 0 {
		body = body[:idx]
	}
	if strings.HasPrefix(body, "[") || strings.HasPrefix(body, "【") {
		body = shareBracketRE.ReplaceAllString(body, " ")
	}
	if idx := yearRE.FindStringIndex(body); idx != nil {
		body = body[:idx[0]]
	}
	if idx := shareSeriesRE.FindStringIndex(body); idx != nil {
		body = body[:idx[0]]
	}
	if idx := shareReleaseRE.FindStringIndex(body); idx != nil {
		body = body[:idx[0]]
	}
	body = shareDiscRE.ReplaceAllString(body, " ")
	body = strings.NewReplacer(".", " ", "[", " ", "]", " ").Replace(body)
	body = strings.TrimSpace(body)
	// 完整混合标题必须优先保留，再补充分语言查询。
	add(body)
	asciiStart := -1
	for i, r := range body {
		if unicode.IsLetter(r) && r < 128 {
			asciiStart = i
			break
		}
	}
	if asciiStart > 0 && strings.ContainsFunc(body[:asciiStart], func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		add(body[:asciiStart])
		add(body[asciiStart:])
	} else {
		add(body)
	}
	return titles, year
}

// AnalyzeShareFilename 提取单个媒体的查询和类型；祖先仅用于剧集上下文或无名光盘。
func AnalyzeShareFilename(filename string) ShareMediaQuery {
	segments := strings.Split(strings.ReplaceAll(filename, "\\", "/"), "/")
	name := segments[len(segments)-1]
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".iso", ".mkv", ".mp4", ".m2ts", ".ts", ".avi":
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	q := ShareMediaQuery{Titles: []string{}, MediaType: "unknown", Complex: strings.ContainsAny(name, "[]【】") || len([]rune(name)) > 80, Container: shareContainerName(name)}
	if q.Container {
		return q
	}
	if m := shareIDRE.FindStringSubmatch(name); m != nil {
		q.TmdbID, _ = strconv.Atoi(m[1])
	}
	q.Titles, q.Year = cleanShareTitles(name)
	if title, _, _, ok := parseRomanSeasonEpisode(name); ok {
		q.Titles, q.Year = cleanShareTitles(title)
		q.MediaType = "tv"
	}
	fileYear := q.Year
	for i := len(segments) - 1; i >= 0 && i >= len(segments)-3; i-- {
		if shareSeriesRE.MatchString(segments[i]) {
			q.MediaType = "tv"
		}
	}
	// 剧集分碟以及Season目录优先使用最近的剧集目录名称和首播年份。
	if q.MediaType == "tv" && (shareDiscRE.MatchString(name) || shareSeriesRE.MatchString(name)) && len(segments) > 1 {
		for i := len(segments) - 2; i >= 0 && i >= len(segments)-3; i-- {
			if shareContainerName(segments[i]) {
				continue
			}
			titles, year := cleanShareTitles(segments[i])
			if len(titles) > 0 {
				q.Titles = titles
				if year > 0 {
					q.Year = year
					q.YearFromDirectory = fileYear == 0 || fileYear != year
				}
				break
			}
		}
	}
	return q
}

// SetAIRecognitionService 在装配阶段注入AI辅助服务。
func (s *TmdbService) SetAIRecognitionService(ai *AIRecognitionService) { s.aiRecognition = ai }

func canonicalShareTitle(title string) string {
	return strings.ToLower(compactSearchTitle(title))
}

func selectVerifiedShareCandidate(q ShareMediaQuery, candidates []domain.TmdbSearchResult) *domain.TmdbSearchResult {
	var best *domain.TmdbSearchResult
	for i := range candidates {
		candidate := &candidates[i]
		if q.MediaType != "unknown" && candidate.MediaType != q.MediaType {
			continue
		}
		// 年份差异过大意味着同名重拍片，不能直接接受搜索首项。
		if q.Year > 0 && (candidate.Year < q.Year-1 || candidate.Year > q.Year+1) {
			continue
		}
		match := false
		for _, title := range q.Titles {
			normalized := canonicalShareTitle(title)
			if normalized != "" && (normalized == canonicalShareTitle(candidate.Title) || normalized == canonicalShareTitle(candidate.OriginalTitle)) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		if best != nil && (best.TmdbID != candidate.TmdbID || best.MediaType != candidate.MediaType || best.MetadataID != candidate.MetadataID || best.MetadataSource != candidate.MetadataSource || best.MetadataProvider != candidate.MetadataProvider) {
			return nil
		}
		best = candidate
	}
	return best
}

func (s *TmdbService) searchShareQuery(ctx context.Context, q ShareMediaQuery, source string) (*domain.TmdbSearchResult, error) {
	source = normalizeMetadataSourcePolicy(source)
	if source == domain.MetadataSourceAuto {
		best, err := s.searchShareQuery(ctx, q, domain.MetadataSourceTMDB)
		if err != nil || best != nil {
			return best, err
		}
		if q.MediaType == "tv" || !s.adultContentEnabled || !s.metatubeDefaultEnabled || !s.MetaTubeEnabled() {
			return nil, nil
		}
		movieQuery := q
		movieQuery.MediaType = "movie"
		return s.searchShareQuery(ctx, movieQuery, domain.MetadataSourceMetaTube)
	}
	candidates := []domain.TmdbSearchResult{}
	types := []string{q.MediaType}
	if q.MediaType == "unknown" {
		types = []string{"movie", "tv"}
	}
	for _, title := range q.Titles {
		for _, kind := range types {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			var found []domain.TmdbSearchResult
			var err error
			if kind == "tv" {
				found, err = s.searchTVContext(ctx, title, q.Year)
			} else if source == domain.MetadataSourceTMDB {
				found, err = s.searchMovieTMDBContext(ctx, title, q.Year)
			} else {
				if !s.adultContentEnabled || !s.MetaTubeEnabled() {
					return nil, fmt.Errorf("成人内容识别或MetaTube未启用")
				}
				found, err = s.searchMetaTubeContext(ctx, title, q.Year)
			}
			if err != nil {
				return nil, err
			}
			for i := range found {
				found[i].MediaType = kind
			}
			candidates = append(candidates, found...)
		}
	}
	if best := selectVerifiedShareCandidate(q, candidates); best != nil {
		return best, nil
	}
	// 仅在普通标题核验失败时检查TMDB官方别名，保持年份和唯一性要求。
	verified := []domain.TmdbSearchResult{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := fmt.Sprintf("%s:%d", candidate.MediaType, candidate.TmdbID)
		if candidate.MetadataSource == domain.MetadataSourceMetaTube || candidate.MetadataProvider != "" || candidate.TmdbID <= 0 || seen[key] {
			continue
		}
		seen[key] = true
		if q.Year > 0 && (candidate.Year < q.Year-1 || candidate.Year > q.Year+1) {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/%s/%d/alternative_titles?api_key=%s", s.baseURL, candidate.MediaType, candidate.TmdbID, s.apiKey), nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("获取TMDB别名失败")
		}
		var aliases struct {
			Results []struct {
				Title string `json:"title"`
			} `json:"results"`
			Titles []struct {
				Title string `json:"title"`
			} `json:"titles"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&aliases)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("获取TMDB别名失败: HTTP %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("解析TMDB别名失败: %w", decodeErr)
		}
		for _, alias := range append(aliases.Results, aliases.Titles...) {
			match := candidate
			match.Title = alias.Title
			if selectVerifiedShareCandidate(q, []domain.TmdbSearchResult{match}) != nil {
				verified = append(verified, candidate)
				break
			}
		}
	}
	if len(verified) == 1 {
		return &verified[0], nil
	}
	// 目录年份可能为整理年份或旧首播日期；只在唯一完整标题匹配时允许去掉该提示重查。
	if q.YearFromDirectory && q.Year > 0 && q.MediaType == "tv" {
		fallback := q
		fallback.Year = 0
		fallback.YearFromDirectory = false
		matches := []domain.TmdbSearchResult{}
		for _, title := range q.Titles {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			found, err := s.searchTVContext(ctx, title, 0)
			if err != nil {
				return nil, err
			}
			for i := range found {
				found[i].MediaType = "tv"
			}
			matches = append(matches, found...)
		}
		return selectVerifiedShareCandidate(fallback, matches), nil
	}
	return nil, nil
}

// IdentifyShareFile 使用单条媒体的清洗查询，绕过旧文件匹配缓存，不回退到合集目录。
// AI最多调用一次，AI建议仍必须经过实际元数据检索和标题/年份核验。
func (s *TmdbService) IdentifyShareFile(ctx context.Context, filename, source, forcedType string) (*domain.TmdbIdentifyResult, error) {
	input := shareEpisodeInput(ctx, filename, forcedType)
	result, err := s.IdentifyWithAssist(ctx, input, IdentifyAssistOptions{
		MediaType: forcedType, MetadataSource: source, AllowAI: true, UseCache: false, ShareMode: true,
	})
	if result != nil {
		result.Filename = filename
	}
	return result, err
}

func (s *TmdbService) identifyShareWithAssistUncached(ctx context.Context, filename, source, forcedType string) (*domain.TmdbIdentifyResult, error) {
	q := s.analyzeShareQuery(filename, forcedType)
	result := &domain.TmdbIdentifyResult{Filename: filename, MediaType: q.MediaType, RecognitionMethod: "rule"}
	if q.Container {
		result.Message = "集合目录不是单部媒体，已跳过"
		return result, nil
	}
	if forcedType == "movie" || forcedType == "tv" {
		q.MediaType = forcedType
	}
	if q.TmdbID > 0 && (q.MediaType == "tv" || q.MediaType == "movie") {
		var detail map[string]interface{}
		var err error
		if q.MediaType == "tv" {
			detail, err = s.shareTVDetail(ctx, q.TmdbID)
		} else {
			detail, err = s.GetMovieDetail(q.TmdbID)
		}
		if err != nil {
			return nil, err
		}
		id, _ := detail["id"].(float64)
		if int(id) != q.TmdbID {
			return nil, fmt.Errorf("TMDB详情ID与文件名标记不一致")
		}
		titleKey, originalKey, dateKey := "title", "original_title", "release_date"
		if q.MediaType == "tv" {
			titleKey, originalKey, dateKey = "name", "original_name", "first_air_date"
		}
		result.Title, _ = detail[titleKey].(string)
		if result.Title == "" {
			return nil, fmt.Errorf("TMDB详情缺少标题")
		}
		result.OriginalTitle, _ = detail[originalKey].(string)
		date, _ := detail[dateKey].(string)
		if len(date) >= 4 {
			result.Year, _ = strconv.Atoi(date[:4])
		}
		poster, _ := detail["poster_path"].(string)
		result.PosterPath = s.getImageURL(poster)
		result.TmdbID = q.TmdbID
		result.MediaType = q.MediaType
		result.MetadataSource = "tmdb"
		result.Success = true
		result.RecognitionMethod = "source"
		result.Message = "通过文件名中的TMDB ID获取详情"
		return result, nil
	}
	usedAI := false
	aiScene := ""
	aiMessage := ""
	assist := func(scene string) {
		if s.aiRecognition == nil || usedAI {
			return
		}
		hint, called, err := s.aiRecognition.Assist(ctx, filename, scene)
		usedAI = usedAI || called
		if called {
			aiScene = scene
		}
		if err != nil {
			aiMessage = "；ai_request_failed：" + err.Error()
			return
		}
		if hint != nil {
			result.QueryBeforeAI = append([]string(nil), q.Titles...)
			result.AIHint = cloneRecognitionHint(hint)
			q.Titles = []string{}
			q.Titles = appendImportCandidate(q.Titles, hint.Title)
			q.Titles = appendImportCandidate(q.Titles, hint.OriginalTitle)
			result.QueryAfterAI = append([]string(nil), q.Titles...)
			if q.Year == 0 && hint.Year > 0 {
				q.Year = hint.Year
			}
			if q.MediaType == "unknown" && forcedType != "movie" && forcedType != "tv" {
				q.MediaType = hint.MediaType
			}
		}
	}
	// 先完成规则、双类型检索和官方别名核验，成功时不触发AI。
	best, err := s.searchShareQuery(ctx, q, source)
	if err != nil {
		return nil, err
	}
	if best == nil && !usedAI {
		// 场景配置仍然生效，但统一在常规识别失败后触发，单次最多请求一次。
		if q.Complex {
			assist("complex_title")
		}
		if q.MediaType == "unknown" {
			assist("uncertain_type")
		}
		assist("no_match")
		if usedAI && aiMessage == "" {
			best, err = s.searchShareQuery(ctx, q, source)
			if err != nil {
				return nil, err
			}
		}
	}
	if best == nil {
		result.Message = "未找到可确认的媒体匹配，请手动核对" + aiMessage
		result.AIUsed = usedAI
		result.AIScene = aiScene
		result.FailureReason = result.Message
		return result, nil
	}
	result.Success = true
	result.MediaType = best.MediaType
	result.TmdbID = best.TmdbID
	result.Title = best.Title
	result.OriginalTitle = best.OriginalTitle
	result.Year = best.Year
	result.PosterPath = best.PosterPath
	result.MetadataSource = best.MetadataSource
	result.MetadataID = best.MetadataID
	result.MetadataProvider = best.MetadataProvider
	result.Message = "标题和年份核验通过"
	result.RecognitionMethod = "source"
	result.AIUsed = usedAI
	result.AIScene = aiScene
	if usedAI {
		result.Message += "（AI辅助）"
		result.RecognitionMethod = "ai"
	}
	return result, nil
}
