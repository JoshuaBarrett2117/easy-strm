package main

import (
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
	OutputDir        string
	BaseURL          string
	Extension        string
	ServerURL        string
	ProgressCallback ProgressCallback
}

// NewStrmGenerator 创建新的STRM文件生成器
func NewStrmGenerator(outputDir, baseURL, extension string) *StrmGenerator {
	if extension == "" {
		extension = ".strm"
	}
	return &StrmGenerator{
		OutputDir: outputDir,
		BaseURL:   baseURL,
		Extension: extension,
	}
}

// GenerateStrmFiles 为视频集合生成STRM文件
func (sg *StrmGenerator) GenerateStrmFiles(collection *VideoCollection) error {
	Debug("Generating STRM files for video collection with %d videos, output dir: %s, extension: %s", collection.Total, sg.OutputDir, sg.Extension)

	// 确保输出目录存在
	if err := os.MkdirAll(sg.OutputDir, 0755); err != nil {
		Error("Failed to create output directory %s: %v", sg.OutputDir, err)
		return fmt.Errorf("create output directory failed: %v", err)
	}

	successCount := 0
	errorCount := 0
	processedCount := 0
	totalCount := len(collection.Videos)

	// 为每个视频生成STRM文件
	for i, video := range collection.Videos {
		processedCount = i + 1
		Debug("Processing video %d/%d: %s", processedCount, totalCount, video.Filename)

		err := sg.generateStrmFile(video)
		if err != nil {
			Error("Failed to generate STRM file for %s (path: %s): %v", video.Filename, video.Path, err)
			errorCount++
			// 继续处理其他文件，不要因为单个文件失败而停止整个过程
			continue
		}
		successCount++

		// 每处理10个文件输出一次进度信息
		if processedCount%10 == 0 {
			Info("STRM generation progress: %d/%d (%.1f%%) - Success: %d, Error: %d",
				processedCount, totalCount, float64(processedCount)/float64(totalCount)*100, successCount, errorCount)
		}
	}

	Info("STRM file generation completed. Total: %d, Success: %d, Error: %d (%.1f%% success rate)",
		totalCount, successCount, errorCount, float64(successCount)/float64(totalCount)*100)

	if errorCount > 0 {
		Warn("%d errors occurred during STRM file generation, but %d files were successfully generated", errorCount, successCount)
		return fmt.Errorf("%d errors occurred during STRM file generation", errorCount)
	}

	Info("All STRM files generated successfully")
	return nil
}

// generateStrmFile 为单个视频生成STRM文件
func (sg *StrmGenerator) generateStrmFile(video VideoFile) error {
	// 构建STRM文件路径
	strmPath := filepath.Join(sg.OutputDir, video.Path)

	// 确保目录存在
	if err := os.MkdirAll(strmPath, 0755); err != nil {
		Error("Failed to create directory %s for STRM file: %v", strmPath, err)
		return fmt.Errorf("create directory failed: %v", err)
	}

	// 构建STRM文件名
	baseFilename := strings.TrimSuffix(video.Filename, filepath.Ext(video.Filename))
	strmFilename := baseFilename + sg.Extension
	strmFilepath := filepath.Join(strmPath, strmFilename)

	// 构建STRM文件内容（服务的无序校验接口URL）
	strmContent := sg.buildStrmContent(video)

	// 写入STRM文件
	if err := os.WriteFile(strmFilepath, []byte(strmContent), 0644); err != nil {
		Error("Failed to write STRM file %s: %v", strmFilepath, err)
		return fmt.Errorf("write STRM file failed: %v", err)
	}

	// 验证文件是否成功写入
	if _, err := os.Stat(strmFilepath); os.IsNotExist(err) {
		Error("STRM file %s was not created after write operation", strmFilepath)
		return fmt.Errorf("STRM file creation failed")
	}

	return nil
}

// buildStrmContent 构建STRM文件内容
func (sg *StrmGenerator) buildStrmContent(video VideoFile) string {
	// 构建完整的文件路径
	fullPath := filepath.Join(video.Path, video.Filename)
	// 编码路径
	encodedPath := url.QueryEscape(fullPath)

	// 构建直链获取接口URL
	// 格式：{serverURL}/api/direct-link?path={encodedPath}
	directLinkURL := fmt.Sprintf("%s/api/direct-link?path=%s", sg.ServerURL, encodedPath)

	Debug("Generated STRM content for %s: %s", fullPath, directLinkURL)
	return directLinkURL
}

// CleanupStrmFiles 清理生成的STRM文件
func (sg *StrmGenerator) CleanupStrmFiles() error {
	Debug("Cleaning up STRM files in: %s", sg.OutputDir)

	// 检查目录是否存在
	if _, err := os.Stat(sg.OutputDir); os.IsNotExist(err) {
		Info("Output directory %s does not exist, skipping cleanup", sg.OutputDir)
		return nil
	}

	// 删除目录及其所有内容
	if err := os.RemoveAll(sg.OutputDir); err != nil {
		Error("Failed to cleanup STRM files in %s: %v", sg.OutputDir, err)
		return fmt.Errorf("cleanup STRM files failed: %v", err)
	}

	Info("STRM files cleanup completed successfully for directory: %s", sg.OutputDir)
	return nil
}

// ValidateStrmFiles 验证生成的STRM文件
func (sg *StrmGenerator) ValidateStrmFiles(collection *VideoCollection) error {
	Debug("Validating STRM files for video collection with %d videos, output dir: %s, extension: %s",
		collection.Total, sg.OutputDir, sg.Extension)

	missingCount := 0
	validCount := 0
	processedCount := 0
	totalCount := len(collection.Videos)

	for i, video := range collection.Videos {
		processedCount = i + 1

		// 构建STRM文件路径
		strmPath := filepath.Join(sg.OutputDir, video.Path)
		baseFilename := strings.TrimSuffix(video.Filename, filepath.Ext(video.Filename))
		strmFilename := baseFilename + sg.Extension
		strmFilepath := filepath.Join(strmPath, strmFilename)

		// 检查文件是否存在
		if _, err := os.Stat(strmFilepath); os.IsNotExist(err) {
			Warn("Missing STRM file: %s", strmFilepath)
			missingCount++
		} else {
			validCount++
			Debug("Validated STRM file: %s", strmFilepath)
		}

		// 每验证10个文件输出一次进度信息
		if processedCount%10 == 0 {
			Info("STRM validation progress: %d/%d (%.1f%%) - Valid: %d, Missing: %d",
				processedCount, totalCount, float64(processedCount)/float64(totalCount)*100, validCount, missingCount)
		}
	}

	Info("STRM file validation completed. Total: %d, Valid: %d, Missing: %d (%.1f%% success rate)",
		totalCount, validCount, missingCount, float64(validCount)/float64(totalCount)*100)

	if missingCount > 0 {
		return fmt.Errorf("%d STRM files are missing", missingCount)
	}

	Info("All STRM files validated successfully")
	return nil
}

// NewStrmGeneratorForPath 创建新的STRM文件生成器（用于网盘路径模式）
func NewStrmGeneratorForPath(outputDir, extension string) *StrmGenerator {
	if extension == "" {
		extension = ".strm"
	}
	return &StrmGenerator{
		OutputDir: outputDir,
		BaseURL:   "",
		Extension: extension,
	}
}

// GenerateStrmFilesWithPath 为文件集合生成STRM文件（内容为网盘绝对路径）
func (sg *StrmGenerator) GenerateStrmFilesWithPath(collection *VideoCollection, netDiskBasePath string) error {
	Debug("Generating STRM files for file collection with %d files, output dir: %s, extension: %s, net_disk_base_path: %s",
		collection.Total, sg.OutputDir, sg.Extension, netDiskBasePath)

	// 确保输出目录存在
	if err := os.MkdirAll(sg.OutputDir, 0755); err != nil {
		Error("Failed to create output directory %s: %v", sg.OutputDir, err)
		return fmt.Errorf("create output directory failed: %v", err)
	}

	successCount := 0
	errorCount := 0
	processedCount := 0
	totalCount := len(collection.Videos)

	// 为每个文件生成STRM文件
	for i, video := range collection.Videos {
		processedCount = i + 1
		Debug("Processing file %d/%d: %s", processedCount, totalCount, video.Filename)

		err := sg.generateStrmFileWithPath(video, netDiskBasePath)
		if err != nil {
			Error("Failed to generate STRM file for %s (path: %s): %v", video.Filename, video.Path, err)
			errorCount++
			// 继续处理其他文件，不要因为单个文件失败而停止整个过程
			continue
		}
		successCount++

		// 每处理10个文件输出一次进度信息
		if processedCount%10 == 0 {
			Info("STRM generation progress: %d/%d (%.1f%%) - Success: %d, Error: %d",
				processedCount, totalCount, float64(processedCount)/float64(totalCount)*100, successCount, errorCount)
		}
	}

	Info("STRM file generation completed. Total: %d, Success: %d, Error: %d (%.1f%% success rate)",
		totalCount, successCount, errorCount, float64(successCount)/float64(totalCount)*100)

	if errorCount > 0 {
		Warn("%d errors occurred during STRM file generation, but %d files were successfully generated", errorCount, successCount)
		return fmt.Errorf("%d errors occurred during STRM file generation", errorCount)
	}

	Info("All STRM files generated successfully")
	return nil
}

// generateStrmFileWithPath 为单个文件生成STRM文件（内容为网盘绝对路径）
func (sg *StrmGenerator) generateStrmFileWithPath(video VideoFile, netDiskBasePath string) error {
	// 构建STRM文件路径
	strmPath := filepath.Join(sg.OutputDir, video.Path)

	// 确保目录存在
	if err := os.MkdirAll(strmPath, 0755); err != nil {
		Error("Failed to create directory %s for STRM file: %v", strmPath, err)
		return fmt.Errorf("create directory failed: %v", err)
	}

	// 构建STRM文件名（保持原文件名，只改后缀）
	strmFilename := video.Filename
	// 如果原文件有扩展名，替换为配置的扩展名
	if ext := filepath.Ext(video.Filename); ext != "" {
		strmFilename = strings.TrimSuffix(video.Filename, ext) + sg.Extension
	} else {
		strmFilename = video.Filename + sg.Extension
	}
	strmFilepath := filepath.Join(strmPath, strmFilename)

	// 构建STRM文件内容（网盘文件的绝对路径）
	// 使用Sha1字段存储的网盘完整路径
	strmContent := video.Sha1
	if strmContent == "" {
		// 如果Sha1字段为空，构建网盘绝对路径
		strmContent = filepath.Join(netDiskBasePath, video.Path, video.Filename)
	}

	// 写入STRM文件
	if err := os.WriteFile(strmFilepath, []byte(strmContent), 0644); err != nil {
		Error("Failed to write STRM file %s: %v", strmFilepath, err)
		return fmt.Errorf("write STRM file failed: %v", err)
	}

	// 验证文件是否成功写入
	if _, err := os.Stat(strmFilepath); os.IsNotExist(err) {
		Error("STRM file %s was not created after write operation", strmFilepath)
		return fmt.Errorf("STRM file creation failed")
	}

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

// GenerateStrmFilesAsync 异步生成STRM文件（带进度回调）
func (sg *StrmGenerator) GenerateStrmFilesAsync(collection *VideoCollection, netDiskBasePath string) error {
	Debug("Generating STRM files asynchronously for file collection with %d files, output dir: %s, extension: %s, net_disk_base_path: %s",
		collection.Total, sg.OutputDir, sg.Extension, netDiskBasePath)

	if err := os.MkdirAll(sg.OutputDir, 0755); err != nil {
		Error("Failed to create output directory %s: %v", sg.OutputDir, err)
		return fmt.Errorf("create output directory failed: %v", err)
	}

	successCount := 0
	errorCount := 0
	totalCount := len(collection.Videos)

	for i, video := range collection.Videos {
		err := sg.generateStrmFileWithDirectLink(video, netDiskBasePath)
		if err != nil {
			Error("Failed to generate STRM file for %s (path: %s): %v", video.Filename, video.Path, err)
			errorCount++
		} else {
			successCount++
		}

		// 调用进度回调
		if sg.ProgressCallback != nil {
			sg.ProgressCallback(totalCount, i+1, successCount, errorCount)
		}

		// 每处理10个文件输出一次进度信息
		if (i+1)%10 == 0 {
			Info("STRM generation progress: %d/%d (%.1f%%) - Success: %d, Error: %d",
				i+1, totalCount, float64(i+1)/float64(totalCount)*100, successCount, errorCount)
		}
	}

	Info("STRM file generation completed. Total: %d, Success: %d, Error: %d (%.1f%% success rate)",
		totalCount, successCount, errorCount, float64(successCount)/float64(totalCount)*100)

	if errorCount > 0 {
		Warn("%d errors occurred during STRM file generation, but %d files were successfully generated", errorCount, successCount)
		return fmt.Errorf("%d errors occurred during STRM file generation", errorCount)
	}

	Info("All STRM files generated successfully")
	return nil
}

// generateStrmFileWithDirectLink 为单个文件生成STRM文件（内容为直链URL）
func (sg *StrmGenerator) generateStrmFileWithDirectLink(video VideoFile, netDiskBasePath string) error {
	strmPath := filepath.Join(sg.OutputDir, video.Path)

	if err := os.MkdirAll(strmPath, 0755); err != nil {
		Error("Failed to create directory %s for STRM file: %v", strmPath, err)
		return fmt.Errorf("create directory failed: %v", err)
	}

	strmFilename := video.Filename
	if ext := filepath.Ext(video.Filename); ext != "" {
		strmFilename = strings.TrimSuffix(video.Filename, ext) + sg.Extension
	} else {
		strmFilename = video.Filename + sg.Extension
	}
	strmFilepath := filepath.Join(strmPath, strmFilename)

	// 构建网盘完整路径
	netDiskFullPath := video.Sha1
	if netDiskFullPath == "" {
		netDiskFullPath = filepath.Join(netDiskBasePath, video.Path, video.Filename)
	}

	// 将Windows路径分隔符转换为正斜杠，确保URL格式正确
	netDiskFullPath = strings.ReplaceAll(netDiskFullPath, "\\", "/")

	// 使用路径形式存储URL，播放时再获取pickcode和直链
	// 这样可以减少生成时的API调用，避免风控
	encodedPath := url.QueryEscape(netDiskFullPath)
	strmContent := fmt.Sprintf("%s/direct-link?path=%s", sg.ServerURL, encodedPath)
	if video.Cloud115ID > 0 {
		strmContent = fmt.Sprintf("%s&cloud115_id=%d", strmContent, video.Cloud115ID)
	}
	Debug("Using path for STRM content: %s, cloud115_id: %d", netDiskFullPath, video.Cloud115ID)

	if err := os.WriteFile(strmFilepath, []byte(strmContent), 0644); err != nil {
		Error("Failed to write STRM file %s: %v", strmFilepath, err)
		return fmt.Errorf("write STRM file failed: %v", err)
	}

	// 验证文件是否成功写入
	if _, err := os.Stat(strmFilepath); os.IsNotExist(err) {
		Error("STRM file %s was not created after write operation", strmFilepath)
		return fmt.Errorf("STRM file creation failed")
	}

	return nil
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

	if err := os.WriteFile(strmFilepath, []byte(strmContent), 0644); err != nil {
		Error("Failed to write STRM file %s: %v", strmFilepath, err)
		return "", fmt.Errorf("write STRM file failed: %v", err)
	}

	return strmFilepath, nil
}
