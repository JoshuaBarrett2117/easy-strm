package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// MediaSourceService 媒体源服务
// 负责媒体源的CRUD操作和文件浏览功能
type MediaSourceService struct {
	mediaSourceDAO *dao.MediaSourceDAO
	cloud115DAO    *dao.Cloud115DAO
	tmdbCacheDAO   *dao.TmdbCacheDAO
}

// NewMediaSourceService 创建媒体源服务实例
// 参数:
//   - mediaSourceDAO: 媒体源DAO
//   - cloud115DAO: 115账号DAO
// 返回:
//   - *MediaSourceService: 媒体源服务实例
func NewMediaSourceService(mediaSourceDAO *dao.MediaSourceDAO, cloud115DAO *dao.Cloud115DAO) *MediaSourceService {
	return &MediaSourceService{
		mediaSourceDAO: mediaSourceDAO,
		cloud115DAO:    cloud115DAO,
		tmdbCacheDAO:   dao.NewTmdbCacheDAO(),
	}
}

// GetByID 根据ID获取媒体源
// 参数:
//   - id: 媒体源ID
// 返回:
//   - *domain.MediaSource: 媒体源信息
//   - error: 错误信息
func (s *MediaSourceService) GetByID(id int) (*domain.MediaSource, error) {
	return s.mediaSourceDAO.GetByID(id)
}

// GetAll 获取所有媒体源
// 参数:
//   - sortField: 排序字段
//   - sortOrder: 排序方向 (asc/desc)
// 返回:
//   - []*domain.MediaSource: 媒体源列表
//   - error: 错误信息
func (s *MediaSourceService) GetAll(sortField, sortOrder string) ([]*domain.MediaSource, error) {
	return s.mediaSourceDAO.GetAll(sortField, sortOrder)
}

// GetByType 根据类型获取媒体源
// 参数:
//   - sourceType: 媒体源类型
// 返回:
//   - []*domain.MediaSource: 媒体源列表
//   - error: 错误信息
func (s *MediaSourceService) GetByType(sourceType string) ([]*domain.MediaSource, error) {
	return s.mediaSourceDAO.GetByType(sourceType)
}

// GetEnabled 获取所有启用的媒体源
// 返回:
//   - []*domain.MediaSource: 媒体源列表
//   - error: 错误信息
func (s *MediaSourceService) GetEnabled() ([]*domain.MediaSource, error) {
	return s.mediaSourceDAO.GetEnabled()
}

// Create 创建媒体源
// 参数:
//   - name: 媒体源名称
//   - sourceType: 媒体源类型
//   - path: 路径
//   - cloud115ID: 115账号ID（可选）
//   - priority: 优先级
//   - enabled: 是否启用
// 返回:
//   - *domain.MediaSource: 创建的媒体源
//   - error: 错误信息
func (s *MediaSourceService) Create(name, sourceType, path string, cloud115ID *int, priority int, enabled bool) (*domain.MediaSource, error) {
	// 验证媒体源类型
	if sourceType != domain.SourceTypeLocal && sourceType != domain.SourceTypeCloud115 {
		return nil, fmt.Errorf("无效的媒体源类型")
	}

	// 验证必填字段
	if name == "" {
		return nil, fmt.Errorf("媒体源名称不能为空")
	}
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}

	// 验证115账号
	if sourceType == domain.SourceTypeCloud115 {
		if cloud115ID == nil || *cloud115ID == 0 {
			return nil, fmt.Errorf("115账号ID不能为空")
		}
		// 检查cloud115DAO是否初始化
		if s.cloud115DAO == nil {
			logger.Errorf("MediaSourceService[Create] cloud115DAO未初始化")
			return nil, fmt.Errorf("系统错误：cloud115DAO未初始化")
		}
		cloud115, err := s.cloud115DAO.GetByID(*cloud115ID)
		if err != nil {
			logger.Errorf("MediaSourceService[Create] 查询115账号失败: cloud115_id=%d, error=%v", *cloud115ID, err)
			return nil, fmt.Errorf("查询115账号失败: %v", err)
		}
		if cloud115 == nil {
			logger.Errorf("MediaSourceService[Create] 115账号不存在: cloud115_id=%d", *cloud115ID)
			return nil, fmt.Errorf("115账号不存在")
		}
		logger.Infof("MediaSourceService[Create] 验证115账号成功: cloud115_id=%d, name=%s", *cloud115ID, cloud115.Name)
	}

	// 创建媒体源
	source, err := s.mediaSourceDAO.Create(name, sourceType, path, cloud115ID, priority, enabled)
	if err != nil {
		logger.Errorf("MediaSourceService[Create] 创建媒体源失败: %v", err)
		return nil, fmt.Errorf("创建媒体源失败: %v", err)
	}

	logger.Infof("MediaSourceService[Create] 创建媒体源成功: %s (ID: %d)", name, source.ID)
	return source, nil
}

// Update 更新媒体源
// 参数:
//   - id: 媒体源ID
//   - name: 媒体源名称
//   - sourceType: 媒体源类型
//   - path: 路径
//   - cloud115ID: 115账号ID（可选）
//   - priority: 优先级
//   - enabled: 是否启用
// 返回:
//   - *domain.MediaSource: 更新后的媒体源
//   - error: 错误信息
func (s *MediaSourceService) Update(id int, name, sourceType, path string, cloud115ID *int, priority int, enabled bool) (*domain.MediaSource, error) {
	// 验证媒体源类型
	if sourceType != domain.SourceTypeLocal && sourceType != domain.SourceTypeCloud115 {
		return nil, fmt.Errorf("无效的媒体源类型")
	}

	// 验证必填字段
	if name == "" {
		return nil, fmt.Errorf("媒体源名称不能为空")
	}
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}

	// 验证115账号
	if sourceType == domain.SourceTypeCloud115 {
		if cloud115ID == nil || *cloud115ID == 0 {
			return nil, fmt.Errorf("115账号ID不能为空")
		}
		cloud115, err := s.cloud115DAO.GetByID(*cloud115ID)
		if err != nil {
			return nil, fmt.Errorf("查询115账号失败")
		}
		if cloud115 == nil {
			return nil, fmt.Errorf("115账号不存在")
		}
	}

	// 更新媒体源
	source, err := s.mediaSourceDAO.Update(id, name, sourceType, path, cloud115ID, priority, enabled)
	if err != nil {
		logger.Errorf("MediaSourceService[Update] 更新媒体源失败: %v", err)
		return nil, fmt.Errorf("更新媒体源失败: %v", err)
	}

	logger.Infof("MediaSourceService[Update] 更新媒体源成功: %s (ID: %d)", name, id)
	return source, nil
}

// Delete 删除媒体源
// 参数:
//   - id: 媒体源ID
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) Delete(id int) error {
	if err := s.mediaSourceDAO.Delete(id); err != nil {
		logger.Errorf("MediaSourceService[Delete] 删除媒体源失败: %v", err)
		return fmt.Errorf("删除媒体源失败: %v", err)
	}

	logger.Infof("MediaSourceService[Delete] 删除媒体源成功: ID %d", id)
	return nil
}

// UpdateEnabled 更新媒体源启用状态
// 参数:
//   - id: 媒体源ID
//   - enabled: 是否启用
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) UpdateEnabled(id int, enabled bool) error {
	if err := s.mediaSourceDAO.UpdateEnabled(id, enabled); err != nil {
		logger.Errorf("MediaSourceService[UpdateEnabled] 更新状态失败: %v", err)
		return fmt.Errorf("更新媒体源状态失败: %v", err)
	}

	logger.Infof("MediaSourceService[UpdateEnabled] 更新媒体源状态成功: ID %d, enabled=%v", id, enabled)
	return nil
}

// GetFiles 获取媒体源文件列表
// 参数:
//   - sourceID: 媒体源ID
//   - path: 相对路径
//   - page: 页码
//   - pageSize: 每页数量
//   - sortField: 排序字段
//   - sortOrder: 排序方向
//   - filter: 过滤条件
//   - search: 搜索关键词
// 返回:
//   - *domain.FileListResult: 文件列表结果
//   - error: 错误信息
func (s *MediaSourceService) GetFiles(sourceID int, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	// 获取媒体源
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("查询媒体源失败")
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	if !source.Enabled {
		return nil, fmt.Errorf("媒体源已禁用")
	}

	// 根据媒体源类型获取文件列表
	switch source.SourceType {
	case domain.SourceTypeLocal:
		return s.getLocalFiles(source, path, page, pageSize, sortField, sortOrder, filter, search)
	case domain.SourceTypeCloud115:
		return s.getCloud115Files(source, path, page, pageSize, sortField, sortOrder, filter, search)
	default:
		return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
	}
}

// getLocalFiles 获取本地文件列表
// 参数:
//   - source: 媒体源
//   - path: 相对路径
//   - page: 页码
//   - pageSize: 每页数量
//   - sortField: 排序字段
//   - sortOrder: 排序方向
//   - filter: 过滤条件
//   - search: 搜索关键词
// 返回:
//   - *domain.FileListResult: 文件列表结果
//   - error: 错误信息
func (s *MediaSourceService) getLocalFiles(source *domain.MediaSource, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	// 构建完整路径
	fullPath := filepath.Join(source.Path, path)

	// 检查路径是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		logger.Errorf("MediaSourceService[getLocalFiles] 路径不存在: %s", fullPath)
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	// 读取目录
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		logger.Errorf("MediaSourceService[getLocalFiles] 读取目录失败: %v", err)
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	// 处理文件列表
	var files []domain.MediaFile
	var fileIDs []string
	for _, entry := range entries {
		// 跳过隐藏文件
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileID := s.joinRelativePath(path, entry.Name())
		file := domain.MediaFile{
			ID:           fileID,
			Name:         entry.Name(),
			Path:         fileID,
			Type:         s.fileTypeFromLocal(entry),
			Size:         info.Size(),
			ModifiedTime: info.ModTime(),
			IsDirectory:  entry.IsDir(),
		}
		files = append(files, file)
		fileIDs = append(fileIDs, fileID)
	}

	// 批量查询TMDB缓存填充TmdbTitle
	if len(files) > 0 {
		var queryKeys []string
		for _, file := range files {
			if !file.IsDirectory {
				queryKeys = append(queryKeys, file.ID) // 兼容旧缓存
				queryKeys = append(queryKeys, strings.ToLower(file.Name))
			}
		}

		if len(queryKeys) > 0 {
			tmdbCacheMap, err := s.tmdbCacheDAO.GetByQueryKeys(queryKeys)
			if err != nil {
				logger.Warnf("MediaSourceService[getLocalFiles] 查询TMDB缓存失败: %v", err)
			} else {
				for i := range files {
					if files[i].IsDirectory {
						continue
					}
					// 优先用小写文件名匹配
					if cache, ok := tmdbCacheMap[strings.ToLower(files[i].Name)]; ok {
						files[i].TmdbTitle = cache.Title
					} else if cache, ok := tmdbCacheMap[files[i].ID]; ok {
						files[i].TmdbTitle = cache.Title
					}
				}
			}
		}
	}

	// 过滤和搜索
	if search != "" || filter != "" {
		files = s.filterFiles(files, filter, search)
	}

	// 排序
	if sortField != "" {
		files = s.sortFiles(files, sortField, sortOrder)
	}

	// 分页
	total := len(files)
	start := (page - 1) * pageSize
	if start > total {
		start = 0
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	files = files[start:end]

	result := &domain.FileListResult{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		Files:      files,
		Breadcrumb: s.buildBreadcrumb(path),
	}

	logger.Infof("MediaSourceService[getLocalFiles] 获取本地文件列表成功: path=%s, total=%d", path, total)
	return result, nil
}

func (s *MediaSourceService) joinRelativePath(basePath, name string) string {
	if basePath == "" || basePath == "/" {
		return name
	}

	joined := filepath.Join(basePath, name)
	return strings.TrimLeft(joined, `\/`)
}

// getCloud115Files 获取115云盘文件列表
// 参数:
//   - source: 媒体源
//   - path: 相对路径（CID）
//   - page: 页码
//   - pageSize: 每页数量
//   - sortField: 排序字段
//   - sortOrder: 排序方向
//   - filter: 过滤条件
//   - search: 搜索关键词
// 返回:
//   - *domain.FileListResult: 文件列表结果
//   - error: 错误信息
func (s *MediaSourceService) getCloud115Files(source *domain.MediaSource, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	// 获取115账号
	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil {
		return nil, fmt.Errorf("查询115账号失败")
	}
	if cloud115 == nil {
		return nil, fmt.Errorf("115账号不存在")
	}

	// 解析CID（如果path为空，使用source.Path作为根CID）
	cid := path
	if cid == "" {
		cid = source.Path
	}

	// 注意：实际的115 API调用需要在Controller层完成，因为需要访问Client实例
	// 这里返回一个占位结果，实际数据由Controller填充
	result := &domain.FileListResult{
		Total:      0,
		Page:       page,
		PageSize:   pageSize,
		Files:      []domain.MediaFile{},
		Breadcrumb: s.buildBreadcrumb(path),
	}

	logger.Infof("MediaSourceService[getCloud115Files] 准备获取115文件列表: CID=%s", cid)
	return result, nil
}

// fileTypeFromLocal 从本地文件判断文件类型
// 参数:
//   - entry: 目录项
// 返回:
//   - string: 文件类型
func (s *MediaSourceService) fileTypeFromLocal(entry os.DirEntry) string {
	if entry.IsDir() {
		return "dir"
	}
	ext := strings.ToLower(filepath.Ext(entry.Name()))
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

// fileTypeFrom115 从115文件判断文件类型
// 参数:
//   - name: 文件名
//   - isDir: 是否为目录
// 返回:
//   - string: 文件类型
func (s *MediaSourceService) fileTypeFrom115(name string, isDir bool) string {
	if isDir {
		return "dir"
	}
	ext := strings.ToLower(filepath.Ext(name))
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

// filterFiles 过滤文件列表
// 参数:
//   - files: 文件列表
//   - filter: 过滤条件
//   - search: 搜索关键词
// 返回:
//   - []domain.MediaFile: 过滤后的文件列表
func (s *MediaSourceService) filterFiles(files []domain.MediaFile, filter, search string) []domain.MediaFile {
	var result []domain.MediaFile
	for _, file := range files {
		// 搜索过滤
		if search != "" && !strings.Contains(strings.ToLower(file.Name), strings.ToLower(search)) {
			continue
		}

		// 类型过滤
		if filter != "" && file.Type != filter {
			continue
		}

		result = append(result, file)
	}
	return result
}

// sortFiles 对文件列表进行排序
// 参数:
//   - files: 文件列表
//   - sortField: 排序字段
//   - sortOrder: 排序方向
// 返回:
//   - []domain.MediaFile: 排序后的文件列表
func (s *MediaSourceService) sortFiles(files []domain.MediaFile, sortField, sortOrder string) []domain.MediaFile {
	// 复制切片避免修改原数据
	result := make([]domain.MediaFile, len(files))
	copy(result, files)

	// 排序逻辑
	sort.Slice(result, func(i, j int) bool {
		// 目录优先
		if result[i].IsDirectory != result[j].IsDirectory {
			return result[i].IsDirectory
		}

		// 按指定字段排序
		switch sortField {
		case "name":
			if sortOrder == "asc" {
				return result[i].Name < result[j].Name
			}
			return result[i].Name > result[j].Name
		case "size":
			if sortOrder == "asc" {
				return result[i].Size < result[j].Size
			}
			return result[i].Size > result[j].Size
		case "modified_time":
			if sortOrder == "asc" {
				return result[i].ModifiedTime.Before(result[j].ModifiedTime)
			}
			return result[i].ModifiedTime.After(result[j].ModifiedTime)
		default:
			// 默认按名称排序
			if sortOrder == "asc" {
				return result[i].Name < result[j].Name
			}
			return result[i].Name > result[j].Name
		}
	})

	return result
}

// buildBreadcrumb 构建面包屑导航
// 参数:
//   - path: 当前路径
// 返回:
//   - []domain.PathItem: 面包屑导航
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

// ValidateLocalPath 验证本地路径是否存在且可访问
// 参数:
//   - path: 本地路径
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) ValidateLocalPath(path string) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}

	// 检查路径是否存在
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("路径不存在: %s", path)
		}
		return fmt.Errorf("无法访问路径: %v", err)
	}

	// 检查是否为目录
	if !info.IsDir() {
		return fmt.Errorf("路径不是目录: %s", path)
	}

	return nil
}

// FormatFileSize 格式化文件大小显示
// 参数:
//   - size: 文件大小（字节）
// 返回:
//   - string: 格式化后的文件大小
func (s *MediaSourceService) FormatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	} else if size < unit*unit {
		return fmt.Sprintf("%.1f KB", float64(size)/float64(unit))
	} else if size < unit*unit*unit {
		return fmt.Sprintf("%.1f MB", float64(size)/float64(unit*unit))
	} else if size < unit*unit*unit*unit {
		return fmt.Sprintf("%.1f GB", float64(size)/float64(unit*unit*unit))
	}
	return fmt.Sprintf("%.1f TB", float64(size)/float64(unit*unit*unit*unit))
}

// CopyFile 复制文件（支持大文件流式复制）
// 参数:
//   - src: 源文件路径
//   - dst: 目标文件路径
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) CopyFile(src, dst string) error {
	// 打开源文件
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %v", err)
	}
	defer sourceFile.Close()

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %v", err)
	}
	defer dstFile.Close()

	// 流式复制
	buffer := make([]byte, 32*1024) // 32KB 缓冲区
	_, err = io.CopyBuffer(dstFile, sourceFile, buffer)
	if err != nil {
		return fmt.Errorf("复制文件失败: %v", err)
	}

	// 复制文件权限
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %v", err)
	}
	return os.Chmod(dst, info.Mode())
}

// MoveFile 移动文件
// 参数:
//   - src: 源文件路径
//   - dst: 目标文件路径
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) MoveFile(src, dst string) error {
	// 首先尝试重命名（同一文件系统）
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// 如果重命名失败（跨文件系统），则复制后删除
	if err := s.CopyFile(src, dst); err != nil {
		return fmt.Errorf("复制文件失败: %v", err)
	}

	// 删除源文件
	if err := os.Remove(src); err != nil {
		// 尝试删除目标文件
		os.Remove(dst)
		return fmt.Errorf("删除源文件失败: %v", err)
	}

	return nil
}

// DeleteFile 删除文件
// 参数:
//   - path: 文件路径
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) DeleteFile(path string) error {
	return os.Remove(path)
}

// RenameFile 重命名文件
// 参数:
//   - oldPath: 旧文件路径
//   - newPath: 新文件路径
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) RenameFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// CreateDirectory 创建目录
// 参数:
//   - path: 目录路径
// 返回:
//   - error: 错误信息
func (s *MediaSourceService) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}
// FileExists 检查文件是否存在
// 参数:
//   - path: 文件路径
// 返回:
//   - bool: 是否存在
func (s *MediaSourceService) FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
// IsDirectory 检查是否为目录
// 参数:
//   - path: 文件路径
// 返回:
//   - bool: 是否为目录
func (s *MediaSourceService) IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
