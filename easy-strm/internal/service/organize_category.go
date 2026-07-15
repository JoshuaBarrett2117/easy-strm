package service

import (
	"easy-strm/internal/domain"
	"fmt"
	"strings"
)

func (s *OrganizeService) matchCategoryPath(identifyResult *domain.TmdbIdentifyResult, categories []*domain.MediaCategory) string {
	targetTitleStr := fmt.Sprintf("%s %s", identifyResult.Title, identifyResult.OriginalTitle)
	targetTitleStr = strings.ToLower(targetTitleStr)
	defaultPath := ""
	bestPath := ""
	bestScore := -1

	for _, cat := range categories {
		if !cat.Enabled || cat.MediaType != identifyResult.MediaType {
			continue
		}

		rule := cat.GetMatchRule()
		if rule.Default {
			if defaultPath == "" {
				defaultPath = cat.TargetPath
			}
			continue
		}

		matched, score := s.matchCategoryRuleScore(identifyResult, targetTitleStr, rule)
		if matched && score > bestScore {
			bestScore = score
			bestPath = cat.TargetPath
		}
	}

	if bestPath != "" {
		return bestPath
	}
	return defaultPath
}

func (s *OrganizeService) matchCategoryRule(identifyResult *domain.TmdbIdentifyResult, targetTitleStr string, rule *domain.CategoryMatchRule) bool {
	matched, _ := s.matchCategoryRuleScore(identifyResult, targetTitleStr, rule)
	return matched
}

func (s *OrganizeService) matchCategoryRuleScore(identifyResult *domain.TmdbIdentifyResult, targetTitleStr string, rule *domain.CategoryMatchRule) (bool, int) {
	hasCondition := false
	score := 0

	if len(rule.Keywords) > 0 {
		hasCondition = true
		keywordMatched := false
		for _, kw := range rule.Keywords {
			if kw == "" {
				continue
			}
			if strings.Contains(targetTitleStr, strings.ToLower(kw)) {
				keywordMatched = true
				break
			}
		}
		if !keywordMatched {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Keywords), 150)
	}

	if len(rule.GenreIDs) > 0 {
		hasCondition = true
		if !hasAnyInt(identifyResult.GenreIDs, rule.GenreIDs) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.GenreIDs), 50)
	}

	if len(rule.Countries) > 0 {
		hasCondition = true
		if !hasAnyString(normalizeCategoryCodes(identifyResult.Countries), normalizeCategoryCodes(rule.Countries)) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Countries), 40)
	}

	if len(rule.Languages) > 0 {
		hasCondition = true
		if !hasAnyString([]string{strings.ToLower(identifyResult.Language)}, normalizeLanguageCodes(rule.Languages)) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Languages), 35)
	}

	if len(rule.Years) > 0 {
		hasCondition = true
		if !hasAnyInt([]int{identifyResult.Year}, rule.Years) {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Years), 20)
	}

	if len(rule.Genres) > 0 {
		hasCondition = true
		genreMatched := false
		for _, genre := range rule.Genres {
			if strings.Contains(targetTitleStr, strings.ToLower(genre)) {
				genreMatched = true
				break
			}
		}
		if !genreMatched {
			return false, 0
		}
		score += s.categoryRuleGroupScore(len(rule.Genres), 10)
	}

	return hasCondition, score
}

func (s *OrganizeService) categoryRuleGroupScore(valueCount int, weight int) int {
	if valueCount <= 0 {
		return 0
	}
	specificityBonus := 100 - valueCount
	if specificityBonus < 1 {
		specificityBonus = 1
	}
	return weight*1000 + specificityBonus
}

func hasAnyInt(values []int, candidates []int) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func hasAnyString(values []string, candidates []string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value != "" && value == candidate {
				return true
			}
		}
	}
	return false
}

func normalizeCategoryCodes(values []string) []string {
	codes := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToUpper(value))
		if value != "" {
			codes = append(codes, value)
		}
	}
	return codes
}

func normalizeLanguageCodes(values []string) []string {
	codes := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value != "" {
			codes = append(codes, value)
		}
	}
	return codes
}

// organizeCloud115File 整理 115云盘 单个文件
