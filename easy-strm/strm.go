package main

import (
	"context"
	"easy-strm/internal/service"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ProgressCallback 进度回调函数类型
type ProgressCallback func(totalFiles, processedFiles, successFiles, failedFiles int)

// StrmGenerator STRM文件生成器
type StrmGenerator struct {
	Output           *service.StrmOutput
	OutputDir        string
	BaseURL          string
	Extension        string
	ServerURL        string
	ProgressCallback ProgressCallback
}

// CleanupStrmFiles 清理生成的STRM文件
func (sg *StrmGenerator) CleanupStrmFiles() error {
	outputDir := strings.TrimSpace(sg.OutputDir)
	if outputDir == "" {
		return fmt.Errorf("STRM输出目录不能为空")
	}
	if filepath.Clean(outputDir) == "." {
		return fmt.Errorf("STRM输出目录不能是当前工作目录")
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("解析STRM输出目录失败: %v", err)
	}
	volumeRoot := filepath.VolumeName(absOutputDir) + string(filepath.Separator)
	if filepath.Clean(absOutputDir) == filepath.Clean(volumeRoot) {
		return fmt.Errorf("STRM输出目录不能是磁盘根目录: %s", absOutputDir)
	}

	Debug("Cleaning up STRM files in: %s", absOutputDir)

	// 检查目录是否存在
	info, err := os.Stat(absOutputDir)
	if os.IsNotExist(err) {
		Info("Output directory %s does not exist, skipping cleanup", absOutputDir)
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取STRM输出目录失败: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("STRM输出路径不是目录: %s", absOutputDir)
	}

	// 仅删除目录内的全部内容，保留配置的目标目录本身。
	entries, err := os.ReadDir(absOutputDir)
	if err != nil {
		return fmt.Errorf("读取STRM输出目录内容失败: %v", err)
	}
	for _, entry := range entries {
		entryPath := filepath.Join(absOutputDir, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			Error("Failed to cleanup STRM path %s: %v", entryPath, err)
			return fmt.Errorf("清理STRM目录项%s失败: %v", entry.Name(), err)
		}
	}

	Info("STRM files cleanup completed successfully for directory: %s", absOutputDir)
	return nil
}

// NewStrmGeneratorWithServer 创建新的STRM文件生成器（带服务器URL）
func NewStrmGeneratorWithServer(outputDir, serverURL, extension string) *StrmGenerator {
	if extension == "" {
		extension = ".strm"
	}
	return &StrmGenerator{
		OutputDir: outputDir,
		ServerURL: serverURL,
		Extension: extension,
	}
}

// GenerateSingleStrmFile 为单个视频生成STRM文件并返回本地路径
func (sg *StrmGenerator) GenerateSingleStrmFile(video VideoFile, netDiskBasePath string) (string, error) {
	strmPath := filepath.Join(sg.OutputDir, video.Path)

	if err := os.MkdirAll(strmPath, 0755); err != nil {
		Error("Failed to create directory %s for STRM file: %v", strmPath, err)
		return "", fmt.Errorf("create directory failed: %v", err)
	}

	strmFilename := video.Filename
	if ext := filepath.Ext(video.Filename); ext != "" {
		strmFilename = strings.TrimSuffix(video.Filename, ext) + sg.Extension
	} else {
		strmFilename = video.Filename + sg.Extension
	}
	strmFilepath := filepath.Join(strmPath, strmFilename)

	netDiskFullPath := video.Sha1
	if netDiskFullPath == "" {
		netDiskFullPath = filepath.Join(netDiskBasePath, video.Path, video.Filename)
	}

	netDiskFullPath = strings.ReplaceAll(netDiskFullPath, "\\", "/")

	encodedPath := url.QueryEscape(netDiskFullPath)
	strmContent := fmt.Sprintf("%s/direct-link?path=%s", sg.ServerURL, encodedPath)
	if video.Cloud115ID > 0 {
		strmContent = fmt.Sprintf("%s&cloud115_id=%d", strmContent, video.Cloud115ID)
	}
	Debug("Using path for STRM content: %s, cloud115_id: %d", netDiskFullPath, video.Cloud115ID)

	var writeErr error
	if sg.Output != nil {
		_, writeErr = sg.Output.Write(context.Background(), video.RelativePath, strmFilepath, strmContent+"\n", "", "")
	} else {
		writeErr = os.WriteFile(strmFilepath, []byte(strmContent), 0644)
	}
	if writeErr != nil {
		return "", writeErr
	}

	return strmFilepath, nil
}
