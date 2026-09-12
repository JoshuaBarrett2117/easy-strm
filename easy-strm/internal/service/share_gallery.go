package service

import (
	"easy-strm/internal/domain"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// normalizeShareGallery 按可信剧集身份折叠展示，保留原记录以便逐条纠正误识别。
func normalizeShareGallery(record *domain.ShareRecord) {
	seen := map[string]bool{}
	for i := range record.Media {
		media := &record.Media[i]
		media.GalleryDuplicate = false
		r := media.Result
		if media.Status != "identified" || r == nil || !r.Success || r.MediaType != "tv" {
			continue
		}
		key := ""
		if r.TmdbID > 0 {
			key = fmt.Sprintf("tmdb:tv:%d", r.TmdbID)
		}
		if key == "" && r.MetadataID != "" && r.MetadataSource != "" {
			key = r.MetadataSource + ":tv:" + r.MetadataID
		}
		if key == "" {
			continue
		}
		media.GalleryDuplicate = seen[key]
		seen[key] = true
	}
}

func shareCandidatePath(value string) string {
	return path.Clean(strings.ReplaceAll(value, "\\", "/"))
}

func isShareVideoFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts", ".rm", ".rmvb", ".mpg", ".mpeg", ".iso":
		return true
	default:
		return false
	}
}

// selectShareIdentifyCandidates 目录已覆盖的单集不重复识别；混放在根目录的单集和独立电影仍保留。
// selectShareMediaFiles returns concrete media files for step 1 ingestion.
func selectShareMediaFiles(files []domain.ShareFileInfo) []domain.ShareFileInfo {
	result := make([]domain.ShareFileInfo, 0, len(files))
	seen := map[string]bool{}
	var walk func([]domain.ShareFileInfo)
	walk = func(items []domain.ShareFileInfo) {
		for _, file := range items {
			if file.IsDir {
				walk(file.Children)
				continue
			}
			name := file.Name
			if name == "" {
				name = file.Path
			}
			if isShareVideoFile(name) {
				key := file.Fid
				if key == "" {
					key = shareCandidatePath(file.Path)
				}
				if !seen[key] {
					seen[key] = true
					result = append(result, file)
				}
			}
		}
	}
	walk(files)
	return result
}

func selectShareIdentifyCandidates(files []domain.ShareFileInfo) []domain.ShareFileInfo {
	directories := map[string]bool{}
	for _, file := range files {
		if file.IsDir && shareContainerName(file.Name) {
			continue
		}
		if file.IsDir && file.Type == "media" {
			name := file.Path
			if name == "" {
				name = file.Name
			}
			directories[shareCandidatePath(name)] = true
		}
	}
	result := make([]domain.ShareFileInfo, 0, len(files))
	for _, file := range files {
		if file.IsDir && shareContainerName(file.Name) {
			continue
		}
		name := file.Path
		if name == "" {
			name = file.Name
		}
		seriesDisc := shareDiscRE.MatchString(file.Name) && AnalyzeShareFilename(name).MediaType == "tv"
		if !file.IsDir && (tvSeasonEpisodeRe.MatchString(file.Name) || seriesDisc) {
			name := file.Path
			if name == "" {
				name = file.Name
			}
			covered := false
			for parent := path.Dir(shareCandidatePath(name)); parent != "." && parent != "/"; parent = path.Dir(parent) {
				if directories[parent] {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
		}
		result = append(result, file)
	}
	return result
}
