package service

import (
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

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
	rules := s.compiledRulesForFilenameParsing()
	result := parseMediaNameSegment(lastPathSegment(segments), true, rules)
	searchTitles := make([]string, 0, 3)
	searchTitles = appendUniqueNonEmpty(searchTitles, result.Title)

	for i := len(segments) - 2; i >= 0 && len(segments)-i <= 3; i-- {
		contextParsed := parseMediaNameSegment(segments[i], false, rules)
		if contextParsed.Year > 0 && result.Year == 0 {
			result.Year = contextParsed.Year
		}
		if result.MediaType == "movie" && contextParsed.MediaType == "tv" && result.Season == 0 && result.Episode == 0 {
			result.MediaType = contextParsed.MediaType
			result.Season = contextParsed.Season
			result.Episode = contextParsed.Episode
			result.Episodes = append([]int(nil), contextParsed.Episodes...)
		}
		searchTitles = appendUniqueNonEmpty(searchTitles, contextParsed.Title)
	}

	if result.Title == "" && len(searchTitles) > 0 {
		result.Title = searchTitles[0]
	}
	result.SearchTitles = searchTitles
	return result
}

// ParseFilename 使用当前生效规则解析文件名，不发起 TMDB 网络请求。
func (s *TmdbService) ParseFilename(filename string) *ParsedFilename {
	return s.parseFilename(filename)
}

func parseMediaNameSegment(input string, trimExt bool, rules []compiledFilenameRecognitionRule) *ParsedFilename {
	result := &ParsedFilename{
		MediaType: "movie",
	}

	name := normalizeFilenameRecognitionInput(input, trimExt)
	metadataName := name
	if matched, ok := matchFilenameRecognitionRule(name, rules); ok {
		result = matched
		name = matched.Title
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
		if matched, _ := regexp.MatchString(pattern, metadataName); matched {
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
		if matched, _ := regexp.MatchString(pattern, metadataName); matched {
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
		if matched, _ := regexp.MatchString(pattern, metadataName); matched {
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
	if result.MatchedRuleID == "" {
		title = trimReleaseGroupSuffix(title)
	}
	result.Title = title
	return result
}

// ParsedFilename 解析后的文件名信息
type ParsedFilename struct {
	Title           string   `json:"title"`
	Year            int      `json:"year"`
	MediaType       string   `json:"media_type"` // movie | tv
	Season          int      `json:"season"`
	Episode         int      `json:"episode"`
	Episodes        []int    `json:"episodes,omitempty"` // 同一文件包含的明确集号，首集仍由episode表示
	Quality         string   `json:"quality"`
	Source          string   `json:"source"`
	Codec           string   `json:"codec"`
	SearchTitles    []string `json:"search_titles"`
	MatchedRuleID   string   `json:"matched_rule_id"`
	MatchedRuleName string   `json:"matched_rule_name"`
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
