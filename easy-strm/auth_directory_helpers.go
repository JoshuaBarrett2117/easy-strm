package main

import (
	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"os"
	"path/filepath"
	"strings"
)

// buildDirTreeFromEntries 从目录树文件条目构建目录树结构
func buildDirTreeFromEntries(entries []DirTreeEntry, rootName string) *DirectoryNode {
	Debug("Building directory tree from %d entries, root name: %s", len(entries), rootName)

	root := &DirectoryNode{
		CID:      "0",
		Name:     rootName,
		Type:     "dir",
		Files:    []driver.FileInfo{},
		Children: []*DirectoryNode{},
	}

	pathToNode := make(map[string]*DirectoryNode)
	pathToNode[""] = root

	for _, entry := range entries {
		if entry.IsDir {
			node := &DirectoryNode{
				CID:      "0",
				Name:     entry.Name,
				Type:     "dir",
				Files:    []driver.FileInfo{},
				Children: []*DirectoryNode{},
			}
			fullPath := entry.Path
			if fullPath != "" {
				fullPath = filepath.Join(fullPath, entry.Name)
			} else {
				fullPath = entry.Name
			}
			pathToNode[fullPath] = node
			Debug("Added directory node: %s at path: %s", entry.Name, fullPath)
		}
	}

	for _, entry := range entries {
		if !entry.IsDir {
			fileInfo := driver.FileInfo{
				Name:     entry.Name,
				Size:     driver.StringInt64(entry.Size),
				PickCode: entry.Pc,
				FileID:   entry.Fid,
				Sha1:     entry.Sha1,
			}
			parentPath := entry.Path
			parentNode, exists := pathToNode[parentPath]
			if !exists {
				parentNode = root
			}
			parentNode.Files = append(parentNode.Files, fileInfo)
			Debug("Added file: %s to directory at path: %s", entry.Name, parentPath)
		}
	}

	for fullPath, node := range pathToNode {
		if fullPath == "" {
			continue
		}
		parentPath := filepath.Dir(fullPath)
		if parentPath == "." {
			parentPath = ""
		}
		parentNode, exists := pathToNode[parentPath]
		if exists && parentNode != node {
			parentNode.Children = append(parentNode.Children, node)
			Debug("Added child: %s to parent at path: %s", node.Name, parentPath)
		}
	}

	return root
}

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
			Debug("Added video file: %s (PickCode: %s, Size: %d, cloud115_id: %d)", netDiskFullPath, filePickCode, file.Size, cloud115Id)
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

// readLastNLines 读取文件的最后N行，返回倒序结果
func readLastNLines(filePath string, n int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	fileSize := stat.Size()

	var lines []string
	var lineBuffer []byte
	var offset int64 = fileSize - 1
	newlineCount := 0

	for offset >= 0 && newlineCount < n {
		b := make([]byte, 1)
		_, err := file.ReadAt(b, offset)
		if err != nil {
			break
		}

		if b[0] == '\n' {
			if len(lineBuffer) > 0 {
				line := reverseBytes(lineBuffer)
				lines = append(lines, string(line))
				lineBuffer = lineBuffer[:0]
				newlineCount++
			}
		} else {
			lineBuffer = append(lineBuffer, b[0])
		}
		offset--
	}

	if len(lineBuffer) > 0 && newlineCount < n {
		line := reverseBytes(lineBuffer)
		lines = append(lines, string(line))
	}

	return strings.Join(lines, "\n"), nil
}

// reverseBytes 反转字节切片
func reverseBytes(b []byte) []byte {
	result := make([]byte, len(b))
	for i := range b {
		result[len(b)-1-i] = b[i]
	}
	return result
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
