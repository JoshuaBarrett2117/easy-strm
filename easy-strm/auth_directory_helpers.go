package main

import (
	"path/filepath"
	"strings"
)

// extractVideoFiles 从目录树中递归提取视频文件
func extractVideoFiles(node *DirectoryNode, currentPath string, netDiskPath string, cid string, targetExts []string, collection *VideoCollection, cloud115Id int, isRoot bool) {
	Debug("Extracting video files from directory: %s, currentPath: %s, isRoot: %v", node.Name, currentPath, isRoot)

	for _, file := range node.Files {
		fileExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.Name), "."))

		matched := false
		for _, targetExt := range targetExts {
			if fileExt == targetExt {
				matched = true
				break
			}
		}

		if matched {
			netDiskFullPath := filepath.Join(netDiskPath, currentPath, file.Name)
			filePickCode := file.PickCode
			if filePickCode == "" {
				filePickCode = file.FileID
			}

			relativePath := currentPath
			if relativePath == "" {
				relativePath = file.Name
			} else {
				relativePath = filepath.Join(currentPath, file.Name)
			}

			videoFile := VideoFile{
				Path:         currentPath,
				Filename:     file.Name,
				CID:          cid,
				FID:          filePickCode,
				Size:         int(file.Size),
				Extension:    filepath.Ext(file.Name),
				Sha1:         netDiskFullPath,
				Cloud115ID:   cloud115Id,
				RelativePath: relativePath,
				PickCode:     filePickCode,
				Name:         file.Name,
			}

			collection.Videos = append(collection.Videos, videoFile)
			Debug("Added video file (Size: %d, cloud115_id: %d)", file.Size, cloud115Id)
		}
	}

	for _, child := range node.Children {
		childPath := child.Name
		if !isRoot && currentPath != "" {
			childPath = filepath.Join(currentPath, child.Name)
		}
		extractVideoFiles(child, childPath, netDiskPath, cid, targetExts, collection, cloud115Id, false)
	}
}
