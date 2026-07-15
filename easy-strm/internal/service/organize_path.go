package service

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

func (s *OrganizeService) buildOrganizeTargetPath(targetPath, generatedName string) (string, string, string) {
	folderName := ""
	newName := generatedName
	if strings.Contains(generatedName, "/") || strings.Contains(generatedName, "\\") {
		folderName = filepath.Dir(generatedName)
		if folderName == "." {
			folderName = ""
		}
		newName = filepath.Base(generatedName)
	}

	finalTargetPath := targetPath
	if folderName != "" {
		finalTargetPath = filepath.Join(targetPath, s.sanitizeFolderName(folderName))
	}
	return finalTargetPath, newName, filepath.Join(finalTargetPath, newName)
}

func (s *OrganizeService) buildCloud115OrganizeTargetPath(targetPath, generatedName string) (string, string, string) {
	normalizedName := strings.ReplaceAll(generatedName, "\\", "/")
	folderName := ""
	newName := pathpkg.Base(normalizedName)
	if strings.Contains(normalizedName, "/") {
		folderName = pathpkg.Dir(normalizedName)
		if folderName == "." {
			folderName = ""
		}
	}

	finalTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
	if finalTargetPath == "" {
		finalTargetPath = "/"
	}
	if folderName != "" {
		finalTargetPath = pathpkg.Join(finalTargetPath, s.sanitizeFolderName(folderName))
	}
	return finalTargetPath, newName, pathpkg.Join(finalTargetPath, newName)
}

// prependCategoryTargetPath 将分类目录作为用户目标目录下的前缀，避免分类配置覆盖本次选择的根目录。

func (s *OrganizeService) prependCategoryTargetPath(targetPath, categoryPath string) string {
	categoryPath = strings.TrimSpace(categoryPath)
	if categoryPath == "" {
		return targetPath
	}

	if volume := filepath.VolumeName(categoryPath); volume != "" {
		categoryPath = strings.TrimPrefix(categoryPath, volume)
	}
	categoryPath = strings.TrimLeft(filepath.ToSlash(categoryPath), "/")
	if categoryPath == "" {
		return targetPath
	}

	parts := strings.Split(categoryPath, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "" {
			return filepath.Join(targetPath, s.sanitizeFolderName(part))
		}
	}
	return targetPath
}

func (s *OrganizeService) buildUniqueTargetPath(targetPath string) (string, error) {
	ext := filepath.Ext(targetPath)
	base := strings.TrimSuffix(filepath.Base(targetPath), ext)
	dir := filepath.Dir(targetPath)

	for i := 1; i <= 999; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("未找到可用的冲突后缀路径")
}

// isMediaFile 判断是否为媒体文件

func (s *OrganizeService) isMediaFile(ext string) bool {
	mediaExts := map[string]bool{
		".mp4":  true,
		".mkv":  true,
		".avi":  true,
		".mov":  true,
		".wmv":  true,
		".flv":  true,
		".webm": true,
		".m4v":  true,
		".rmvb": true,
		".rm":   true,
		".mp3":  true,
		".flac": true,
		".wav":  true,
		".aac":  true,
		".m4a":  true,
	}
	return mediaExts[ext]
}

func (s *OrganizeService) isOrganizeVideoFile(ext string) bool {
	return s.getFileType(ext) == "video"
}

// getFileType 获取文件类型

func (s *OrganizeService) getFileType(ext string) string {
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
		".rmvb": true, ".rm": true,
	}
	audioExts := map[string]bool{
		".mp3": true, ".flac": true, ".wav": true, ".aac": true, ".m4a": true,
	}

	if videoExts[ext] {
		return "video"
	}
	if audioExts[ext] {
		return "audio"
	}
	return "file"
}

// BatchIdentify 批量识别文件
// 参数:
//   - sourceID: 媒体源ID
//   - fileIDs: 文件ID列表
//
// 返回:
//   - []domain.TmdbIdentifyResult: 识别结果列表
//   - error: 错误信息

func (s *OrganizeService) sanitizeFolderName(name string) string {
	replacer := strings.NewReplacer(
		":", " -",
		"?", "",
		"*", "",
		"<", "",
		">", "",
		"|", "",
		"\"", "",
	)
	return strings.TrimSpace(replacer.Replace(name))
}
