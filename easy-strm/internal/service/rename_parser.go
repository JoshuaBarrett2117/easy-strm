package service

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedEpisode 解析后的季集信息。
type ParsedEpisode struct {
	Season  int
	Episode int
	Quality string
	Source  string
	Codec   string
}

// parseSeasonEpisode 解析文件名中的季集信息。
func (s *RenameService) parseSeasonEpisode(name string) *ParsedEpisode {
	result := &ParsedEpisode{}

	patterns := []struct {
		regex   string
		handler func(matches []string)
	}{
		{regex: `(?i)S(\d{1,2})E(\d{1,2})`, handler: func(matches []string) {
			result.Season, _ = strconv.Atoi(matches[1])
			result.Episode, _ = strconv.Atoi(matches[2])
		}},
		{regex: `(?i)Season\s*(\d{1,2})\s*Episode\s*(\d{1,2})`, handler: func(matches []string) {
			result.Season, _ = strconv.Atoi(matches[1])
			result.Episode, _ = strconv.Atoi(matches[2])
		}},
		{regex: `(?i)(\d{1,2})x(\d{1,2})`, handler: func(matches []string) {
			result.Season, _ = strconv.Atoi(matches[1])
			result.Episode, _ = strconv.Atoi(matches[2])
		}},
		{regex: `(?i)EP?(\d{1,3})`, handler: func(matches []string) {
			result.Episode, _ = strconv.Atoi(matches[1])
			result.Season = 1
		}},
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern.regex)
		if matches := re.FindStringSubmatch(name); len(matches) > 0 {
			pattern.handler(matches)
			break
		}
	}

	applyFirstMatchedTag(name, map[string]*string{
		`(?i)\b4K\b`:    &result.Quality,
		`(?i)\b2160p\b`: &result.Quality,
		`(?i)\b1080p\b`: &result.Quality,
		`(?i)\b720p\b`:  &result.Quality,
	}, map[string]string{
		`(?i)\b4K\b`:    "4K",
		`(?i)\b2160p\b`: "2160p",
		`(?i)\b1080p\b`: "1080p",
		`(?i)\b720p\b`:  "720p",
	})
	applyFirstMatchedTag(name, map[string]*string{
		`(?i)\bBluRay\b`:  &result.Source,
		`(?i)\bWEB-?DL\b`: &result.Source,
		`(?i)\bHDTV\b`:    &result.Source,
	}, map[string]string{
		`(?i)\bBluRay\b`:  "BluRay",
		`(?i)\bWEB-?DL\b`: "WEB-DL",
		`(?i)\bHDTV\b`:    "HDTV",
	})
	applyFirstMatchedTag(name, map[string]*string{
		`(?i)\bx264\b`: &result.Codec,
		`(?i)\bx265\b`: &result.Codec,
		`(?i)\bHEVC\b`: &result.Codec,
	}, map[string]string{
		`(?i)\bx264\b`: "x264",
		`(?i)\bx265\b`: "x265",
		`(?i)\bHEVC\b`: "HEVC",
	})

	return result
}

func applyFirstMatchedTag(name string, targets map[string]*string, values map[string]string) {
	for pattern, target := range targets {
		if matched, _ := regexp.MatchString(pattern, name); matched {
			*target = values[pattern]
			return
		}
	}
}

// cleanTitle 清理标题。
func (s *RenameService) cleanTitle(title string) string {
	tagsToRemove := []string{
		`(?i)\bBluRay\b`, `(?i)\bWEB-?DL\b`, `(?i)\bHDTV\b`,
		`(?i)\bBDRip\b`, `(?i)\bDVDRip\b`, `(?i)\bHDRip\b`,
		`(?i)\bx264\b`, `(?i)\bx265\b`, `(?i)\bH\.?264\b`, `(?i)\bH\.?265\b`,
		`(?i)\bHEVC\b`, `(?i)\bAVC\b`, `(?i)\bAAC\b`, `(?i)\bAC3\b`,
		`(?i)\bDTS\b`, `(?i)\bDD5\.?1\b`, `(?i)\b5\.?1\b`,
		`(?i)\b4K\b`, `(?i)\b2160p\b`, `(?i)\b1080p\b`, `(?i)\b720p\b`,
		`(?i)\bAMZN\b`, `(?i)\bNF\b`, `(?i)\bHMAX\b`,
		`(?i)\bS\d{1,2}E\d{1,2}\b`, `(?i)\bSeason\s*\d+`,
		`(?i)\bEP?\d{1,3}\b`,
	}

	result := title
	for _, tag := range tagsToRemove {
		re := regexp.MustCompile(tag)
		result = re.ReplaceAllString(result, "")
	}

	result = regexp.MustCompile(`[._]+`).ReplaceAllString(result, " ")
	result = regexp.MustCompile(`\s*-\s*`).ReplaceAllString(result, " ")
	result = regexp.MustCompile(`[()\[\]{}]+`).ReplaceAllString(result, " ")
	result = strings.Trim(result, " .-_")
	result = strings.TrimSpace(result)
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")

	return result
}
