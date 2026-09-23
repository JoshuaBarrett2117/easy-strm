package service

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var romanEpisodePattern = regexp.MustCompile(`(?i)^(.+?)([ⅠⅡⅢⅣⅤⅥⅦⅧⅨⅩⅪⅫ]|XII|XI|IX|VIII|VII|VI|IV|III|II|V|X|I)[ ._-]*-[ ._-]*([0-9]{1,3})(?:[ ._].*)?$`)

// parseRomanSeasonEpisode 将明确的罗马部数与集号解释为季集，普通片名中的罗马字母不作转换。
func parseRomanSeasonEpisode(input string) (string, int, int, bool) {
	name := path.Base(strings.ReplaceAll(input, "\\", "/"))
	switch strings.ToLower(path.Ext(name)) {
	case ".mkv", ".mp4", ".avi", ".ts", ".m2ts", ".iso":
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	m := romanEpisodePattern.FindStringSubmatch(name)
	if m == nil {
		return "", 0, 0, false
	}
	title := strings.TrimSpace(m[1])
	// ASCII 部数必须与英文片名有分隔，中文片名可以紧邻部数。
	last := []rune(m[1])
	marker := []rune(m[2])[0]
	if marker < 128 && len(last) > 0 && unicode.IsLetter(last[len(last)-1]) && !unicode.Is(unicode.Han, last[len(last)-1]) {
		return "", 0, 0, false
	}
	seasons := map[string]int{"Ⅰ": 1, "Ⅱ": 2, "Ⅲ": 3, "Ⅳ": 4, "Ⅴ": 5, "Ⅵ": 6, "Ⅶ": 7, "Ⅷ": 8, "Ⅸ": 9, "Ⅹ": 10, "Ⅺ": 11, "Ⅻ": 12, "I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10, "XI": 11, "XII": 12}
	episode, _ := strconv.Atoi(m[3])
	return strings.Trim(title, " ._-"), seasons[strings.ToUpper(m[2])], episode, title != "" && episode > 0
}
