package service

import (
	"easy-strm/internal/domain"
	"fmt"
	"sort"
	"strings"
)

func shareSourceEpisodeSignature(source domain.ShareStrmSource) string {
	episodes := append([]domain.ShareEpisode(nil), source.Episodes...)
	sort.Slice(episodes, func(first, second int) bool {
		if episodes[first].SeasonNumber != episodes[second].SeasonNumber {
			return episodes[first].SeasonNumber < episodes[second].SeasonNumber
		}
		return episodes[first].EpisodeNumber < episodes[second].EpisodeNumber
	})
	parts := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		parts = append(parts, fmt.Sprintf("%d:%d", episode.SeasonNumber, episode.EpisodeNumber))
	}
	return strings.Join(parts, ",")
}
