package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"easy-strm/internal/domain"
)

func (s *MediaSourceService) joinRelativePath(basePath, name string) string {
	if basePath == "" || basePath == "/" {
		return name
	}

	joined := filepath.Join(basePath, name)
	return strings.TrimLeft(joined, `\/`)
}

// fileTypeFromLocal 从本地文件判断文件类型。
func (s *MediaSourceService) fileTypeFromLocal(entry os.DirEntry) string {
	if entry.IsDir() {
		return "dir"
	}
	ext := strings.ToLower(filepath.Ext(entry.Name()))
	return mediaFileTypeByExt(ext)
}

// fileTypeFrom115 从115文件判断文件类型。
func (s *MediaSourceService) fileTypeFrom115(name string, isDir bool) string {
	if isDir {
		return "dir"
	}
	ext := strings.ToLower(filepath.Ext(name))
	return mediaFileTypeByExt(ext)
}

func mediaFileTypeByExt(ext string) string {
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

// filterFiles 过滤文件列表。
func (s *MediaSourceService) filterFiles(files []domain.MediaFile, filter, search string) []domain.MediaFile {
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
func (s *MediaSourceService) sortFiles(files []domain.MediaFile, sortField, sortOrder string) []domain.MediaFile {
	result := make([]domain.MediaFile, len(files))
	copy(result, files)

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDirectory != result[j].IsDirectory {
			return result[i].IsDirectory
		}

		switch sortField {
		case "name":
			return compareMediaFileString(result[i].Name, result[j].Name, sortOrder)
		case "size":
			return compareMediaFileInt64(result[i].Size, result[j].Size, sortOrder)
		case "modified_time":
			if sortOrder == "asc" {
				return result[i].ModifiedTime.Before(result[j].ModifiedTime)
			}
			return result[i].ModifiedTime.After(result[j].ModifiedTime)
		default:
			return compareMediaFileString(result[i].Name, result[j].Name, sortOrder)
		}
	})

	return result
}

func compareMediaFileString(left, right, sortOrder string) bool {
	if sortOrder == "asc" {
		return left < right
	}
	return left > right
}

func compareMediaFileInt64(left, right int64, sortOrder string) bool {
	if sortOrder == "asc" {
		return left < right
	}
	return left > right
}

// buildBreadcrumb 构建面包屑导航。
func (s *MediaSourceService) buildBreadcrumb(path string) []domain.PathItem {
	if path == "" {
		return []domain.PathItem{}
	}

	items := []domain.PathItem{}
	parts := strings.Split(path, string(filepath.Separator))
	currentPath := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if currentPath != "" {
			currentPath = filepath.Join(currentPath, part)
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
