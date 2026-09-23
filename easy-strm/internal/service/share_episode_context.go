package service

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
)

var shareBareEpisodeRE = regexp.MustCompile(`^(.+?)[ ._]*-[ ._]*([0-9]{1,3})$`)
var shareDirectorySeasonRE = regexp.MustCompile(`(?i)(?:\bS|\bSeason[ ._-]*|第)([0-9]{1,2})(?:季|\b)`)

func shareBareEpisode(filename string) (string, int, bool) {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	name = strings.TrimSuffix(name, path.Ext(name))
	if _, _, _, ok := parseRomanSeasonEpisode(filename); ok {
		return "", 0, false
	}
	m := shareBareEpisodeRE.FindStringSubmatch(name)
	if m == nil {
		return "", 0, false
	}
	n, _ := strconv.Atoi(m[2])
	return strings.TrimSpace(m[1]), n, n > 0
}

func contextualShareEpisode(filename, title string, episode int) string {
	parent := path.Dir(strings.ReplaceAll(filename, "\\", "/"))
	season := 1
	if matches := shareDirectorySeasonRE.FindAllStringSubmatch(parent, -1); len(matches) > 0 {
		season, _ = strconv.Atoi(matches[len(matches)-1][1])
	}
	return path.Join(parent, fmt.Sprintf("%s S%02dE%02d%s", title, season, episode, path.Ext(filename)))
}

// prepareShareEpisodeInputs 仅对同目录同标题的连续01、02编号启用单集推断，不修改原始路径。
func prepareShareEpisodeInputs(ctx context.Context, records []domain.ShareRecord) {
	round := ctx.Value(recognitionRoundKey{}).(*recognitionRound)
	for _, record := range records {
		if record.MediaType == "movie" {
			continue
		}
		groups := map[string]map[int]bool{}
		for _, media := range record.Media {
			if !media.Available || isMaskedSharePath(media.FileName) {
				continue
			}
			title, n, ok := shareBareEpisode(media.FileName)
			if !ok {
				continue
			}
			key := path.Join(path.Dir(strings.ReplaceAll(media.FileName, "\\", "/")), title)
			if groups[key] == nil {
				groups[key] = map[int]bool{}
			}
			groups[key][n] = true
		}
		for _, media := range record.Media {
			title, n, ok := shareBareEpisode(media.FileName)
			if !ok {
				continue
			}
			key := path.Join(path.Dir(strings.ReplaceAll(media.FileName, "\\", "/")), title)
			if groups[key][1] && groups[key][2] {
				round.inputs[fmt.Sprintf("%d:%s", record.ID, media.FileName)] = contextualShareEpisode(media.FileName, title, n)
			}
		}
	}
}

func shareEpisodeInput(ctx context.Context, filename, forcedType string) string {
	if round, ok := ctx.Value(recognitionRoundKey{}).(*recognitionRound); ok {
		if value := round.inputs[fmt.Sprintf("%v:%s", ctx.Value(shareWorkScopeKey{}), filename)]; value != "" && forcedType != "movie" {
			return value
		}
	}
	if title, n, ok := shareBareEpisode(filename); ok && forcedType != "movie" {
		parent := path.Dir(strings.ReplaceAll(filename, "\\", "/"))
		if forcedType == "tv" || shareSeriesRE.MatchString(parent) || strings.Contains(parent, "电视剧") || strings.Contains(parent, "剧集") {
			return contextualShareEpisode(filename, title, n)
		}
	}
	return filename
}
