package service

import (
	"context"
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
)

var (
	shareOnlyEpisode   = regexp.MustCompile(`^(?:第([0-9]{1,3})[集话話]|([0-9]{1,3}))(?:[（(][0-9]+[）)])?$`)
	shareOnlySxe       = regexp.MustCompile(`(?i)^S([0-9]{1,3})E([0-9]{1,3})$`)
	shareJoinedQuality = regexp.MustCompile(`(?i)(S[0-9]{1,3}E[0-9]{1,3})(2160P|1080P|720P)$`)
	shareNumericTitle  = regexp.MustCompile(`^[0-9]+$|^第[0-9]+[集话話]$|(?i)^S[0-9]+$`)
	shareSeasonToken   = regexp.MustCompile(`(?i)(?:^|[ ._（(])S([0-9]{1,3})(?:[ ._（）)(]|$)|(?:^|[ ._])Season[ ._]*([0-9]{1,3})(?:[ ._（）)(]|$)|第([一二三四五六七八九十0-9]+)季`)
)

func shareContextSeason(name string) (int, bool) {
	m := shareSeasonToken.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	for _, v := range m[1:] {
		if v != "" {
			if n, e := strconv.Atoi(v); e == nil {
				return n, true
			}
			n := 0
			for _, r := range v {
				if r == '十' {
					if n == 0 {
						n = 1
					}
					n *= 10
				} else {
					for index, digit := range []rune("一二三四五六七八九") {
						if digit == r {
							n += index + 1
							break
						}
					}
				}
			}
			return n, n > 0
		}
	}
	return 0, false
}

// shareTVDirectoryContext 只在明确剧集路径中提取最近的作品目录，季目录单独返回年份。
func shareTVDirectoryContext(filename, forcedType string) (title string, season, workYear, seasonYear int, ok bool) {
	if forcedType == "movie" {
		return
	}
	segments := strings.Split(strings.ReplaceAll(filename, "\\", "/"), "/")
	ok = forcedType == "tv"
	for _, v := range segments[:len(segments)-1] {
		if strings.Contains(v, "剧集") || strings.Contains(v, "电视剧") || shareSeriesRE.MatchString(v) {
			ok = true
		}
	}
	if !ok {
		return
	}
	season = 1
	seasonFound := false
	for i := len(segments) - 2; i >= 0; i-- {
		v := segments[i]
		n, isSeason := shareContextSeason(v)
		y, _ := shareTitleYear(shareYearRangeRE.ReplaceAllString(v, " "))
		if isSeason && !seasonFound {
			season = n
			seasonYear = y
			seasonFound = true
		}
		names, _ := cleanShareTitles(v)
		if len(names) == 0 {
			continue
		}
		candidate := names[0]
		// 清除季目录的画质，不让“4K”等标签成为作品名。
		candidate = regexp.MustCompile(`(?i)(?:[ ._-]|^)(?:4K|HDR|Dolby|Atmos).*$`).ReplaceAllString(candidate, "")
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || shareNumericTitle.MatchString(candidate) || shareContainerName(candidate) || strings.Contains(candidate, "合集") || strings.Contains(candidate, "系列") || strings.Contains(candidate, "剧集") || candidate == "Season" {
			continue
		}
		if title == "" {
			title = candidate
		}
		if !isSeason && y > 0 {
			workYear = y
			break
		}
		if !isSeason {
			break
		}
	}
	ok = ok && title != ""
	return
}

func contextualShareTVInput(filename, forcedType string) string {
	title, season, _, _, ok := shareTVDirectoryContext(filename, forcedType)
	if !ok {
		return filename
	}
	normalized := strings.ReplaceAll(filename, "\\", "/")
	base := path.Base(normalized)
	ext := path.Ext(base)
	name := strings.TrimSuffix(base, ext)
	episode := 0
	if m := shareOnlyEpisode.FindStringSubmatch(name); m != nil {
		value := m[1]
		if value == "" {
			value = m[2]
		}
		episode, _ = strconv.Atoi(value)
	}
	if m := shareOnlySxe.FindStringSubmatch(name); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
	}
	if episode > 0 {
		return path.Join(path.Dir(normalized), fmtShareEpisode(title, season, episode)+ext)
	}
	// 仅拆分明确的画质后缀，避免把任意连续数字截成集号。
	if shareJoinedQuality.MatchString(name) {
		return path.Join(path.Dir(normalized), shareJoinedQuality.ReplaceAllString(name, "${1}.${2}")+ext)
	}
	brackets := shareBracketRE.FindAllStringSubmatch(name, -1)
	for i, m := range brackets {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		if canonicalShareTitle(v) != canonicalShareTitle(title) || i+1 >= len(brackets) {
			continue
		}
		v = brackets[i+1][1]
		if v == "" {
			v = brackets[i+1][2]
		}
		if len(v) > 3 || !shareNumericTitle.MatchString(v) {
			continue
		}
		episode, _ = strconv.Atoi(v)
		if episode > 0 {
			return path.Join(path.Dir(normalized), fmtShareEpisode(title, season, episode)+ext)
		}
	}
	return filename
}

func fmtShareEpisode(title string, season, episode int) string {
	return title + " S" + fmtShareNumber(season) + "E" + fmtShareNumber(episode)
}
func fmtShareNumber(n int) string {
	v := strconv.Itoa(n)
	if n < 10 {
		return "0" + v
	}
	return v
}

func applyShareTVContext(q *ShareMediaQuery, filename string, p *ParsedFilename) {
	if q.MediaType != "tv" || p.Episode <= 0 {
		return
	}
	q.Season = p.Season
	title, _, workYear, seasonYear, ok := shareTVDirectoryContext(filename, "tv")
	if !ok {
		return
	}
	if title != "" {
		q.Titles = appendImportCandidate(q.Titles, title)
	}
	if p.Season > 1 {
		if seasonYear == 0 {
			seasonYear = q.fileYear
			if seasonYear == 0 && q.rangeStart == 0 {
				seasonYear = q.Year
			}
		}
		q.SeasonYear = seasonYear
		q.Year = workYear
		q.YearFromDirectory = workYear > 0
		q.YearSource = "season_directory"
		if workYear > 0 {
			q.YearSource = "work_directory"
		}
	} else if workYear > 0 {
		q.Year = workYear
		q.YearFromDirectory = true
		q.YearSource = "work_directory"
	}
}

// shareSeasonEvidence 使用季名与播出年份核验映射，不以人气或候选顺序选择作品。
func shareSeasonEvidence(q ShareMediaQuery, c domain.TmdbSearchResult, detail map[string]interface{}, aliases []string) (int, bool) {
	if _, exists := q.tvYears[shareCandidateIdentity(c)]; !exists {
		q.tvYears = map[string]shareTVYears{shareCandidateIdentity(c): shareDetailYears(detail, c)}
	}
	direct := shareCandidateTitleScore(q, c, aliases) > 0
	seasons, _ := detail["seasons"].([]interface{})
	if direct && len(seasons) == 0 {
		return q.Season, shareCandidateYearMatches(q, c)
	}
	mapped := -1
	for _, raw := range seasons {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		number, ok := item["season_number"].(float64)
		if !ok {
			continue
		}
		n := int(number)
		name, _ := item["name"].(string)
		named := false
		if n > 0 && name != "" && !shareSeasonRE.MatchString(name) {
			for _, title := range q.Titles {
				for _, variant := range []string{name, c.Title + name, c.Title + "之" + name} {
					if canonicalShareTitle(title) == canonicalShareTitle(variant) {
						named = true
					}
				}
			}
		}
		if !named && (!direct || n != q.Season) {
			continue
		}
		seasonQuery := q
		seasonQuery.Season = n
		if !shareCandidateYearMatches(seasonQuery, c) {
			continue
		}
		if mapped >= 0 && mapped != n {
			if q.Evidence != nil {
				q.Evidence.ambiguous = true
			}
			return 0, false
		}
		mapped = n
	}
	return mapped, mapped >= 0
}

func (s *TmdbService) selectShareTVCandidate(ctx context.Context, q ShareMediaQuery, candidates []domain.TmdbSearchResult) (*domain.TmdbSearchResult, error) {
	direct := make([]domain.TmdbSearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if shareCandidateTitleScore(q, candidate, nil) > 0 {
			direct = append(direct, candidate)
		}
	}
	var err error
	q, err = s.enrichShareYearEvidence(ctx, q, direct)
	if err != nil {
		return nil, err
	}
	directMatches := map[string]bool{}
	for _, candidate := range direct {
		if shareCandidateYearMatches(q, candidate) {
			directMatches[shareCandidateIdentity(candidate)] = true
		}
	}
	if len(directMatches) > 1 && selectVerifiedShareCandidate(q, direct) == nil {
		if q.Evidence != nil {
			q.Evidence.record(q, candidates)
			q.Evidence.ambiguous = true
		}
		return nil, nil
	}
	aliases := map[string][]string{}
	relevant := make([]domain.TmdbSearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		key := shareCandidateIdentity(candidate)
		if _, exists := aliases[key]; !exists && shareCandidateTitleScore(q, candidate, nil) == 0 {
			names, err := s.shareAliases(ctx, "tv", candidate.TmdbID)
			if err != nil {
				return nil, err
			}
			aliases[key] = names
		}
		matched := shareCandidateTitleScore(q, candidate, aliases[key]) > 0
		if !matched && q.Season > 0 {
			// 先读取指定季核验具名季，404仅表示该季不存在，其他故障不能当作无关证据。
			_, err := s.getTVSeasonDetailContext(ctx, candidate.TmdbID, q.Season)
			if err != nil && !errors.Is(err, errTVSeasonNotFound) {
				return nil, err
			}
			// 季存在时还需详情核验其他具名季，不能仅凭指定季名称排除合法映射。
			matched = err == nil
		}
		if matched {
			relevant = append(relevant, candidate)
		}
	}
	q, err = s.enrichShareYearEvidence(ctx, q, relevant)
	if err != nil {
		return nil, err
	}
	if q.Evidence != nil {
		q.Evidence.record(q, candidates)
	}
	bestScore := 0
	var best *domain.TmdbSearchResult
	ambiguous := false
	for index := range relevant {
		candidate := &relevant[index]
		key := shareCandidateIdentity(*candidate)
		score := shareCandidateTitleScore(q, *candidate, aliases[key])
		yearMatches := shareCandidateYearMatches(q, *candidate)
		if q.Season > 0 {
			detail, err := s.shareTVDetail(ctx, candidate.TmdbID)
			if err != nil {
				return nil, err
			}
			if _, ok := shareSeasonEvidence(q, *candidate, detail, aliases[key]); ok {
				yearMatches = true
				if score == 0 {
					score = 1
				}
			}
		}
		if score == 0 || !yearMatches {
			continue
		}
		if score > bestScore {
			best, bestScore, ambiguous = candidate, score, false
		} else if score == bestScore && best != nil && shareCandidateIdentity(*best) != key {
			ambiguous = true
		}
	}
	if ambiguous {
		if q.Evidence != nil {
			q.Evidence.ambiguous = true
		}
		return nil, nil
	}
	if q.Evidence != nil && q.Evidence.ambiguous {
		return nil, nil
	}
	return best, nil
}

func (s *TmdbService) selectShareSeasonCandidate(ctx context.Context, q ShareMediaQuery, candidates []domain.TmdbSearchResult, aliases map[string][]string) (*domain.TmdbSearchResult, error) {
	if q.MediaType != "tv" || q.Season <= 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	var best *domain.TmdbSearchResult
	for i := range candidates {
		c := &candidates[i]
		key := shareCandidateIdentity(*c)
		if c.MediaType != "tv" || c.TmdbID <= 0 || seen[key] {
			continue
		}
		seen[key] = true
		detail, err := s.shareTVDetail(ctx, c.TmdbID)
		if err != nil {
			return nil, err
		}
		if _, ok := shareSeasonEvidence(q, *c, detail, aliases[key]); !ok {
			continue
		}
		if best != nil {
			if q.Evidence != nil {
				q.Evidence.ambiguous = true
			}
			return nil, nil
		}
		best = c
	}
	return best, nil
}
