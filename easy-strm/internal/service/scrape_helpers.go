package service

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"easy-strm/internal/pkg/logger"
)

func tmdbImageURL(size, path string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "original"
	}
	return tmdbImageBaseURL + size + path
}

func isRemoteURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func extractTmdbID(rawData json.RawMessage) int {
	if len(rawData) == 0 {
		return 0
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rawData, &payload); err != nil {
		return 0
	}
	switch value := payload["tmdb_id"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	}
	switch value := payload["id"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	}
	return 0
}

func needsMovieDetailRefresh(detail *tmdbMovieDetail) bool {
	if detail == nil {
		return false
	}
	return detail.ID <= 0 ||
		detail.BackdropPath == "" ||
		detail.Runtime <= 0 ||
		len(detail.Genres) == 0 ||
		detail.Credits == nil ||
		isRemoteURL(detail.PosterPath) ||
		isRemoteURL(detail.BackdropPath)
}

func needsTVDetailRefresh(detail *tmdbTVDetail) bool {
	if detail == nil {
		return false
	}
	return detail.ID <= 0 ||
		detail.BackdropPath == "" ||
		len(detail.Genres) == 0 ||
		detail.Credits == nil ||
		isRemoteURL(detail.PosterPath) ||
		isRemoteURL(detail.BackdropPath)
}

func sidecarName(mediaPath, suffix string) string {
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	return base + suffix
}

func sidecarNames(mediaPath, suffix string) []string {
	names := []string{sidecarName(mediaPath, suffix)}
	if generic := genericArtworkName(suffix); generic != "" && generic != names[0] {
		names = append(names, generic)
	}
	return names
}

func genericArtworkName(suffix string) string {
	switch suffix {
	case "-poster.jpg":
		return "poster.jpg"
	case "-fanart.jpg":
		return "fanart.jpg"
	case "-thumb.jpg":
		return "thumb.jpg"
	default:
		return ""
	}
}

func copyFile(srcPath, destPath string) error {
	if srcPath == destPath {
		return nil
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source artwork: %v", err)
	}
	defer in.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create alias artwork: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy alias artwork: %v", err)
	}
	return nil
}

func extractDirectors(crew []tmdbCrew) []string {
	return extractCrewNames(crew, map[string]bool{"Director": true})
}

func extractWriters(crew []tmdbCrew) []string {
	return extractCrewNames(crew, map[string]bool{
		"Writer":     true,
		"Screenplay": true,
		"Story":      true,
	})
}

func extractCrewNames(crew []tmdbCrew, allowed map[string]bool) []string {
	seen := make(map[string]bool)
	var names []string
	for _, item := range crew {
		if !allowed[item.Job] || seen[item.Name] {
			continue
		}
		names = append(names, item.Name)
		seen[item.Name] = true
	}
	return names
}

func extractActors(cast []tmdbCast) []nfoActor {
	sorted := make([]tmdbCast, len(cast))
	copy(sorted, cast)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Order < sorted[j].Order
	})

	limit := 10
	if len(sorted) < limit {
		limit = len(sorted)
	}

	actors := make([]nfoActor, 0, limit)
	for i := 0; i < limit; i++ {
		actor := nfoActor{
			Name: sorted[i].Name,
			Role: sorted[i].Character,
		}
		if sorted[i].ProfilePath != "" {
			actor.Thumb = tmdbImageURL("original", sorted[i].ProfilePath)
		}
		actors = append(actors, actor)
	}
	return actors
}

func marshalNFO(v interface{}) (string, error) {
	output, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("ScrapeService[marshalNFO] XML序列化失败: %v", err)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + string(output) + "\n", nil
}

func nfoPathFromMedia(mediaPath string) string {
	ext := filepath.Ext(mediaPath)
	return strings.TrimSuffix(mediaPath, ext) + ".nfo"
}

func (s *ScrapeService) writeNFOFile(mediaFilePath string, nfoContent string) error {
	nfoPath := nfoPathFromMedia(mediaFilePath)

	dir := filepath.Dir(nfoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	if err := os.WriteFile(nfoPath, []byte(nfoContent), 0644); err != nil {
		return fmt.Errorf("写入NFO文件失败: %v", err)
	}

	logger.Infof("[ScrapeService] NFO文件已写入: %s", nfoPath)
	return nil
}
