package service

import (
	"path/filepath"
	"regexp"
	"strings"
)

func (s *RenameService) normalizeGeneratedName(generatedName, originalName, ext string) string {
	generatedName = strings.TrimSpace(generatedName)
	if generatedName == "" {
		return generatedName
	}

	normalizedExt := strings.TrimSpace(ext)
	if normalizedExt == "" {
		normalizedExt = filepath.Ext(originalName)
	}
	if normalizedExt == "" {
		return generatedName
	}

	normalizedOriginal := strings.TrimSpace(originalName)
	if normalizedOriginal == "" {
		return generatedName
	}

	originalStem := strings.TrimSuffix(normalizedOriginal, normalizedExt)
	if originalStem == "" {
		return generatedName
	}

	dirPart := filepath.Dir(generatedName)
	baseName := filepath.Base(generatedName)
	duplicatePrefix := normalizedOriginal + originalStem
	if strings.HasPrefix(baseName, duplicatePrefix) {
		baseName = strings.TrimPrefix(baseName, normalizedOriginal)
	}
	baseName = s.normalizeGeneratedBaseName(baseName, originalStem, normalizedExt)

	if dirPart != "." && dirPart != "" {
		return filepath.Join(dirPart, baseName)
	}
	return baseName
}

func (s *RenameService) normalizeGeneratedBaseName(baseName, originalStem, ext string) string {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		return baseName
	}

	normalizedExt, extIndex := findGeneratedExtension(baseName, ext)
	if extIndex < 0 {
		return baseName
	}

	stemBeforeExt := strings.TrimSpace(baseName[:extIndex])
	tailAfterExt := strings.TrimSpace(baseName[extIndex+len(normalizedExt):])
	if stemBeforeExt == "" || tailAfterExt == "" {
		return baseName
	}

	for _, candidate := range s.duplicateStemCandidates(stemBeforeExt, originalStem) {
		if candidate != "" && tailAfterExt == candidate {
			return stemBeforeExt + normalizedExt
		}
	}

	return baseName
}

func findGeneratedExtension(baseName, preferredExt string) (string, int) {
	if preferredExt != "" {
		if index := strings.Index(baseName, preferredExt); index >= 0 {
			return preferredExt, index
		}
	}

	candidates := []string{
		".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts",
	}
	bestExt := ""
	bestIndex := -1
	for _, candidate := range candidates {
		index := strings.Index(baseName, candidate)
		if index < 0 {
			continue
		}
		if bestIndex < 0 || index < bestIndex {
			bestExt = candidate
			bestIndex = index
		}
	}
	return bestExt, bestIndex
}

func (s *RenameService) duplicateStemCandidates(stemBeforeExt, originalStem string) []string {
	candidates := make([]string, 0, 8)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}

	add(stemBeforeExt)
	add(originalStem)
	add(stripTrailingYear(stemBeforeExt))
	add(stripTrailingYear(originalStem))
	add(s.cleanTitle(stemBeforeExt))
	add(s.cleanTitle(originalStem))
	add(stripTrailingYear(s.cleanTitle(stemBeforeExt)))
	add(stripTrailingYear(s.cleanTitle(originalStem)))

	return candidates
}

func stripTrailingYear(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	re := regexp.MustCompile(`\s*[\(\[（【]\d{4}[\)\]）】]\s*$`)
	return strings.TrimSpace(re.ReplaceAllString(value, ""))
}

func pickRenameTitlesFromDetail(detail map[string]interface{}, mediaType, fallbackTitle, fallbackOriginalTitle string) (string, string) {
	title := fallbackTitle
	originalTitle := fallbackOriginalTitle

	if mediaType == "tv" {
		if value, ok := detail["name"].(string); ok && value != "" {
			title = value
		}
		if value, ok := detail["original_name"].(string); ok && value != "" {
			originalTitle = value
		}
	} else {
		if value, ok := detail["title"].(string); ok && value != "" {
			title = value
		}
		if value, ok := detail["original_title"].(string); ok && value != "" {
			originalTitle = value
		}
	}

	if translations, ok := detail["translations"].(map[string]interface{}); ok {
		if items, ok := translations["translations"].([]interface{}); ok {
			for _, item := range items {
				entry, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				lang, _ := entry["iso_639_1"].(string)
				if lang != "zh" {
					continue
				}
				data, ok := entry["data"].(map[string]interface{})
				if !ok {
					continue
				}
				if mediaType == "tv" {
					if value, ok := data["name"].(string); ok && value != "" {
						title = value
						break
					}
				} else {
					if value, ok := data["title"].(string); ok && value != "" {
						title = value
						break
					}
				}
			}
		}
	}

	if title == "" {
		title = originalTitle
	}
	if originalTitle == "" {
		originalTitle = title
	}
	return title, originalTitle
}
