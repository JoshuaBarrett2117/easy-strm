package controller

import (
	"strings"

	"easy-strm/internal/domain"
)

func resolveCloud115CID(path, sourcePath string) string {
	if path != "" && path != "/" {
		return path
	}
	if sourcePath != "" && sourcePath != "/" {
		return sourcePath
	}
	return "0"
}

// fileTypeFrom115 从115文件判断文件类型。
func (c *MediaSourceController) fileTypeFrom115(name string, isDir bool) string {
	if isDir {
		return "dir"
	}

	ext := strings.ToLower(getFileExt(name))
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts":
		return "video"
	case ".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a":
		return "audio"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return "image"
	case ".srt", ".ass", ".ssa", ".vtt":
		return "subtitle"
	default:
		return "file"
	}
}

// getFileExt 获取文件扩展名。
func getFileExt(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

// filterFiles 过滤文件列表。
func (c *MediaSourceController) filterFiles(files []domain.MediaFile, filter, search string) []domain.MediaFile {
	var result []domain.MediaFile
	for _, file := range files {
		if search != "" && !strings.Contains(strings.ToLower(file.Name), strings.ToLower(search)) {
			continue
		}
		if filter != "" && file.Type != filter {
			continue
		}
		result = append(result, file)
	}
	return result
}

// sortFiles 对文件列表进行排序。
func (c *MediaSourceController) sortFiles(files []domain.MediaFile, sortField, sortOrder string) []domain.MediaFile {
	result := make([]domain.MediaFile, len(files))
	copy(result, files)

	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].IsDirectory && !result[j].IsDirectory {
				continue
			}
			if !result[i].IsDirectory && result[j].IsDirectory {
				result[i], result[j] = result[j], result[i]
				continue
			}

			var compare bool
			switch sortField {
			case "name":
				compare = compareString(result[i].Name, result[j].Name, sortOrder)
			case "size":
				compare = compareInt64(result[i].Size, result[j].Size, sortOrder)
			default:
				compare = compareString(result[i].Name, result[j].Name, sortOrder)
			}

			if compare {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

func compareString(left, right, sortOrder string) bool {
	if sortOrder == "asc" {
		return left > right
	}
	return left < right
}

func compareInt64(left, right int64, sortOrder string) bool {
	if sortOrder == "asc" {
		return left > right
	}
	return left < right
}

// buildBreadcrumb 构建面包屑导航。
func (c *MediaSourceController) buildBreadcrumb(path string) []domain.PathItem {
	if path == "" {
		return []domain.PathItem{}
	}

	items := []domain.PathItem{}
	parts := strings.Split(path, "/")
	currentPath := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if currentPath != "" {
			currentPath = currentPath + "/" + part
		} else {
			currentPath = part
		}
		items = append(items, domain.PathItem{
			Name: part,
			Path: currentPath,
		})
	}

	return items
}
