package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VideoFile 表示一个视频文件

type VideoFile struct {
	Path         string `json:"path"`         // 完整路径
	Filename     string `json:"filename"`     // 文件名
	CID          string `json:"cid"`          // 云盘目录ID
	FID          string `json:"fid"`          // 云盘文件ID（pickcode）
	Size         int    `json:"size"`         // 文件大小
	Extension    string `json:"extension"`    // 文件扩展名
	Sha1         string `json:"sha1"`         // SHA1哈希值（也用于存储网盘绝对路径）
	Cloud115ID   int    `json:"cloud115Id"`   // 115账号ID
	RelativePath string `json:"relativePath"` // 相对路径
	PickCode     string `json:"pickCode"`     // 文件pickcode
	Name         string `json:"name"`         // 文件名（同Filename）
}

// VideoCollection 视频文件集合
type VideoCollection struct {
	Videos []VideoFile `json:"videos"`
	Total  int         `json:"total"`
}

// 支持的视频文件扩展名
var supportedVideoExtensions = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".m4v":  true,
	".ts":   true,
	".m2ts": true,
	".iso":  true,
	".ifo":  true,
	".vob":  true,
}

// ParseDirectoryTree 解析目录树文件，提取视频资源
func ParseDirectoryTree(treeFilePath string) (*VideoCollection, error) {
	Debug("Parsing directory tree file: %s", treeFilePath)

	// 读取目录树文件
	treeData, err := os.ReadFile(treeFilePath)
	if err != nil {
		Error("Failed to read directory tree file: %v", err)
		return nil, fmt.Errorf("read directory tree file failed: %v", err)
	}

	// 解析JSON
	var rootNode DirectoryNode
	if err := json.Unmarshal(treeData, &rootNode); err != nil {
		Error("Failed to unmarshal directory tree: %v", err)
		return nil, fmt.Errorf("unmarshal directory tree failed: %v", err)
	}

	// 提取视频文件
	collection := &VideoCollection{
		Videos: []VideoFile{},
	}

	// 递归遍历目录树，提取视频文件
	error := traverseDirectoryTree(&rootNode, "", collection)
	if error != nil {
		Error("Failed to traverse directory tree: %v", error)
		return nil, fmt.Errorf("traverse directory tree failed: %v", error)
	}

	// 设置视频总数
	collection.Total = len(collection.Videos)

	Info("Directory tree parsing completed. Found %d video files", collection.Total)
	return collection, nil
}

// traverseDirectoryTree 递归遍历目录树，提取视频文件
func traverseDirectoryTree(node *DirectoryNode, currentPath string, collection *VideoCollection) error {
	Debug("Traversing directory: %s, path: %s", node.Name, currentPath)

	// 构建当前目录的完整路径
	dirPath := filepath.Join(currentPath, node.Name)

	// 处理当前目录下的文件
	for _, file := range node.Files {
		// 检查文件是否为视频文件
		extension := strings.ToLower(filepath.Ext(file.Name))
		if supportedVideoExtensions[extension] {
			// 获取文件ID（优先使用FileID，如果没有则使用PickCode拾取码）
			fileID := file.FileID
			if fileID == "" {
				fileID = file.PickCode // 使用PickCode字段作为文件ID（拾取码）
			}

			// 获取文件大小
			fileSize := int(file.Size)

			// 创建视频文件对象
			videoFile := VideoFile{
				Path:      dirPath,
				Filename:  file.Name,
				CID:       node.CID,
				FID:       fileID,
				Size:      fileSize,
				Extension: extension,
			}

			// 添加到集合中
			collection.Videos = append(collection.Videos, videoFile)
			Debug("Added video file: %s (FID: %s, Size: %d)", filepath.Join(dirPath, file.Name), fileID, fileSize)
		}
	}

	// 递归处理子目录
	for _, child := range node.Children {
		if err := traverseDirectoryTree(child, dirPath, collection); err != nil {
			Error("Failed to traverse child directory %s: %v", child.Name, err)
			return err
		}
	}

	return nil
}

// FilterVideoExtensions 过滤指定扩展名的视频文件
func (vc *VideoCollection) FilterVideoExtensions(extensions []string) *VideoCollection {
	Debug("Filtering video files by extensions: %v", extensions)

	filtered := &VideoCollection{
		Videos: []VideoFile{},
	}

	// 创建扩展名映射
	extMap := make(map[string]bool)
	for _, ext := range extensions {
		extMap[strings.ToLower(ext)] = true
	}

	// 过滤视频文件
	for _, video := range vc.Videos {
		if extMap[video.Extension] {
			filtered.Videos = append(filtered.Videos, video)
		}
	}

	filtered.Total = len(filtered.Videos)
	Info("Filtered video collection. Total: %d", filtered.Total)
	return filtered
}

// GetVideoByPath 根据路径查找视频文件
func (vc *VideoCollection) GetVideoByPath(path string) (*VideoFile, error) {
	Debug("Searching for video by path: %s", path)

	for i, video := range vc.Videos {
		fullPath := filepath.Join(video.Path, video.Filename)
		if fullPath == path {
			Info("Found video by path: %s", path)
			return &vc.Videos[i], nil
		}
	}

	Error("Video not found by path: %s", path)
	return nil, fmt.Errorf("video not found by path: %s", path)
}

// ExportVideoCollection 导出视频集合为JSON文件
func (vc *VideoCollection) ExportVideoCollection(outputPath string) error {
	Debug("Exporting video collection to: %s", outputPath)

	// 确保目录存在
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		Error("Failed to create output directory: %v", err)
		return fmt.Errorf("create output directory failed: %v", err)
	}

	// 写入JSON文件
	file, err := os.Create(outputPath)
	if err != nil {
		Error("Failed to create output file: %v", err)
		return fmt.Errorf("create output file failed: %v", err)
	}
	defer file.Close()

	// 编码为JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(vc); err != nil {
		Error("Failed to encode video collection: %v", err)
		return fmt.Errorf("encode video collection failed: %v", err)
	}

	Info("Video collection exported successfully to: %s", outputPath)
	return nil
}
