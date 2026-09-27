package service

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"easy-strm/internal/domain"
)

var shareStandaloneYear = regexp.MustCompile(`(?:19|20)[0-9]{2}`)

// shareTitleYear 优先括号年份，否则取技术参数前最后一个独立年份，保留片名中的1900、1917等数字。
func shareTitleYear(name string) (int, int) {
	if m := shareParentYearRE.FindStringSubmatchIndex(name); m != nil {
		year, _ := strconv.Atoi(name[m[2]:m[3]])
		return year, m[2]
	}
	// 发行标记可能出现在年份之前（例如 [UHD原盘]...2018），不能提前截断。
	positions := shareStandaloneYear.FindAllStringIndex(name, -1)
	for i := len(positions) - 1; i >= 0; i-- {
		p := positions[i]
		if p[0] > 0 && !strings.ContainsRune(" ._([（", rune(name[p[0]-1])) {
			continue
		}
		if p[1] < len(name) && !strings.ContainsRune(" ._)]）", rune(name[p[1]])) {
			continue
		}
		if p[0] == 0 {
			continue
		}
		year, _ := strconv.Atoi(name[p[0]:p[1]])
		return year, p[0]
	}
	return 0, -1
}

func shareNoiseTitle(value string) bool {
	lower := strings.ToLower(value)
	for _, token := range []string{"hdr10", "dolby", "atmos", "truehd", "内封", "內封", "中字", "字幕", "原盘", "无字"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// addShareMovieContext 只使用最近的带作品年份目录，不把上级合集名称当作片名；明确剧集不走电影上下文。
func addShareMovieContext(q *ShareMediaQuery, filename string) {
	if q.MediaType == "tv" || q.Container {
		return
	}
	parent := path.Base(path.Dir(strings.ReplaceAll(filename, "\\", "/")))
	if shareContainerName(parent) || strings.Contains(parent, "合集") || strings.Contains(parent, "系列") {
		return
	}
	index := shareParentYearRE.FindStringIndex(parent)
	if index == nil {
		return
	}
	title := strings.TrimSpace(parent[:index[0]])
	if title == "" {
		return
	}
	titles, year := cleanShareTitles(parent)
	for _, name := range titles {
		q.Titles = appendImportCandidate(q.Titles, name)
	}
	// “流媒体版”明确表明文件年份描述发行版本，此时目录括号年份才是作品年份。
	if strings.Contains(parent, "流媒体") && year > 0 {
		q.Year = year
		q.YearSource = "work_directory"
		q.YearFromDirectory = true
	}
	if year > 0 && (q.Year == 0 || q.Year == 1900 || q.Year == 1917) {
		q.Year = year
		q.YearSource = "work_directory"
		q.YearFromDirectory = true
	}
	// 有明确电影目录且无季集标记时限定电影，防止同名电视剧抢占身份。
	for _, segment := range strings.Split(strings.ReplaceAll(filename, "\\", "/"), "/") {
		if regexp.MustCompile(`^(?:[0-9]{4}年)?电影$`).MatchString(segment) {
			q.MediaType = "movie"
			break
		}
	}
}

type shareExpandedYearKey struct{}

func shareYearSearchExpanded(ctx context.Context) bool {
	v, _ := ctx.Value(shareExpandedYearKey{}).(bool)
	return v
}

func shareCandidateIdentity(c domain.TmdbSearchResult) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d", c.MetadataSource, c.MetadataProvider, c.MetadataID, c.MediaType, c.TmdbID)
}
func shareCandidateYearMatches(q ShareMediaQuery, c domain.TmdbSearchResult) bool {
	return q.Year == 0 || c.Year > 0 && c.Year >= q.Year-1 && c.Year <= q.Year+1
}
func shareCandidateTitleScore(q ShareMediaQuery, c domain.TmdbSearchResult, aliases []string) int {
	names := append([]string{c.Title, c.OriginalTitle}, aliases...)
	matches := map[string]bool{}
	for _, title := range q.Titles {
		key := canonicalShareTitle(title)
		if key == "" {
			continue
		}
		for _, name := range names {
			if key == canonicalShareTitle(name) {
				matches[key] = true
			}
		}
	}
	// 不给人气或候选顺序加分。不同语言的完整名称共同匹配才增加身份支持。
	han, other := false, false
	for name := range matches {
		if strings.ContainsFunc(name, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			han = true
		} else {
			other = true
		}
	}
	if han && other {
		return 2
	}
	if len(matches) > 0 {
		return 1
	}
	return 0
}
func selectShareEvidenceCandidate(q ShareMediaQuery, candidates []domain.TmdbSearchResult, aliases map[string][]string) *domain.TmdbSearchResult {
	bestScore := 0
	var best *domain.TmdbSearchResult
	ambiguous := false
	for i := range candidates {
		c := &candidates[i]
		if q.MediaType != "unknown" && q.MediaType != "" && c.MediaType != q.MediaType || !shareCandidateYearMatches(q, *c) {
			continue
		}
		score := shareCandidateTitleScore(q, *c, aliases[shareCandidateIdentity(*c)])
		if score == 0 {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = c
			ambiguous = false
		} else if score == bestScore && best != nil && shareCandidateIdentity(*best) != shareCandidateIdentity(*c) {
			ambiguous = true
		}
	}
	if ambiguous {
		return nil
	}
	return best
}

type shareRecognitionEvidence struct {
	candidates                           []domain.TmdbSearchResult
	missingYear, yearConflict, ambiguous bool
}

func (e *shareRecognitionEvidence) record(q ShareMediaQuery, candidates []domain.TmdbSearchResult) {
	seen := map[string]bool{}
	for _, c := range e.candidates {
		seen[shareCandidateIdentity(c)] = true
	}
	for _, c := range candidates {
		if q.MediaType != "unknown" && q.MediaType != "" && c.MediaType != q.MediaType {
			continue
		}
		if !seen[shareCandidateIdentity(c)] && len(e.candidates) < 20 {
			e.candidates = append(e.candidates, c)
			seen[shareCandidateIdentity(c)] = true
		}
		if shareCandidateTitleScore(q, c, nil) > 0 && q.Year > 0 {
			if c.Year == 0 {
				e.missingYear = true
			} else if !shareCandidateYearMatches(q, c) {
				e.yearConflict = true
			}
		}
	}
}
func (e *shareRecognitionEvidence) recordMatches(q ShareMediaQuery, candidates []domain.TmdbSearchResult, aliases map[string][]string) {
	matches := map[string]bool{}
	for _, c := range candidates {
		if (q.MediaType == "unknown" || q.MediaType == "" || c.MediaType == q.MediaType) && shareCandidateYearMatches(q, c) && shareCandidateTitleScore(q, c, aliases[shareCandidateIdentity(c)]) > 0 {
			matches[shareCandidateIdentity(c)] = true
		}
	}
	e.ambiguous = e.ambiguous || len(matches) > 1
}
func (e *shareRecognitionEvidence) reason() string {
	switch {
	case e.ambiguous:
		return "多个同名候选通过标题和年份核验，身份仍有冲突，请手动核对"
	case e.missingYear:
		return "候选缺少上映年份，无法核验文件年份，请手动核对"
	case e.yearConflict:
		return "片名已匹配，但文件年份与作品上映年份冲突，请手动核对"
	case len(e.candidates) > 0:
		return "找到候选，但完整片名或官方译名未通过核验，请手动核对"
	default:
		return "未找到匹配候选，请核对完整片名、年份及数据源收录情况"
	}
}
