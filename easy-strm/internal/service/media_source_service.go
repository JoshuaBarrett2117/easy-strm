package service

import (
	"fmt"
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
//
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
//
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
//
// 返回:
//   - []*domain.MediaSource: 媒体源列表
//   - error: 错误信息
func (s *MediaSourceService) GetAll(sortField, sortOrder string) ([]*domain.MediaSource, error) {
	return s.mediaSourceDAO.GetAll(sortField, sortOrder)
}

// GetByType 根据类型获取媒体源
// 参数:
//   - sourceType: 媒体源类型
//
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

// GetWatchEnabled 获取启用目录监控的媒体源
func (s *MediaSourceService) GetWatchEnabled() ([]*domain.MediaSource, error) {
	return s.mediaSourceDAO.GetWatchEnabled()
}

func normalizeMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return "all"
	case "movie", "tv":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "all"
	}
}

func normalizeConflictPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "skip":
		return "skip"
	case "overwrite", "suffix":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "skip"
	}
}

func normalizeOperationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "move":
		return "move"
	case "copy", "hardlink", "symlink":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "move"
	}
}

func normalizeWatchPath(sourceType, path, watchPath string, watchEnabled bool) (string, error) {
	watchPath = strings.TrimSpace(watchPath)
	path = strings.TrimSpace(path)

	if watchPath != "" {
		return watchPath, nil
	}

	if sourceType == domain.SourceTypeCloud115 {
		if watchEnabled {
			return "", fmt.Errorf("115 目录监控需要指定监控目录")
		}
		if path != "" {
			return path, nil
		}
		return "", nil
	}

	if path != "" {
		return path, nil
	}

	return "", nil
}

// normalizeCloud115DirectoryPath 将 115 目录配置统一为以斜杠开头的绝对路径。
func normalizeCloud115DirectoryPath(value, fieldName string, allowEmpty bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("%s不能为空", fieldName)
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("%s必须使用绝对路径，例如 /影视资源", fieldName)
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(value, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "/", nil
	}
	return "/" + strings.Join(parts, "/"), nil
}

// Create 创建媒体源
// 参数:
//   - name: 媒体源名称
//   - sourceType: 媒体源类型
//   - path: 路径
//   - cloud115ID: 115账号ID（可选）
//   - priority: 优先级
//   - enabled: 是否启用
//
// 返回:
//   - *domain.MediaSource: 创建的媒体源
//   - error: 错误信息
func (s *MediaSourceService) Create(name, sourceType, path, watchPath string, cloud115ID *int, priority int, enabled bool, organizeTargetPath, mediaType, conflictPolicy, operationMode string, autoOrganize, watchEnabled bool, watchInterval int, embyLibraryID string) (*domain.MediaSource, error) {
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

	watchPath, err := normalizeWatchPath(sourceType, path, watchPath, watchEnabled)
	if err != nil {
		return nil, err
	}
	mediaType = normalizeMediaType(mediaType)
	conflictPolicy = normalizeConflictPolicy(conflictPolicy)
	operationMode = normalizeOperationMode(operationMode)
	if !watchEnabled {
		autoOrganize = false
	}
	if watchInterval <= 0 {
		watchInterval = 1800
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
		path, err = normalizeCloud115DirectoryPath(path, "115 媒体源目录", false)
		if err != nil {
			return nil, err
		}
		watchPath, err = normalizeCloud115DirectoryPath(watchPath, "115 监控目录", !watchEnabled)
		if err != nil {
			return nil, err
		}
		organizeTargetPath, err = normalizeCloud115DirectoryPath(organizeTargetPath, "115 整理目标目录", true)
		if err != nil {
			return nil, err
		}
		logger.Infof("MediaSourceService[Create] 验证115账号成功: cloud115_id=%d, name=%s", *cloud115ID, cloud115.Name)
	}

	// 创建媒体源
	source, err := s.mediaSourceDAO.Create(name, sourceType, path, watchPath, cloud115ID, priority, enabled, organizeTargetPath, mediaType, conflictPolicy, operationMode, autoOrganize, watchEnabled, watchInterval, embyLibraryID)
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
//
// 返回:
//   - *domain.MediaSource: 更新后的媒体源
//   - error: 错误信息
func (s *MediaSourceService) Update(id int, name, sourceType, path, watchPath string, cloud115ID *int, priority int, enabled bool, organizeTargetPath, mediaType, conflictPolicy, operationMode string, autoOrganize, watchEnabled bool, watchInterval int, embyLibraryID string) (*domain.MediaSource, error) {
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

	watchPath, err := normalizeWatchPath(sourceType, path, watchPath, watchEnabled)
	if err != nil {
		return nil, err
	}
	mediaType = normalizeMediaType(mediaType)
	conflictPolicy = normalizeConflictPolicy(conflictPolicy)
	operationMode = normalizeOperationMode(operationMode)
	if !watchEnabled {
		autoOrganize = false
	}
	if watchInterval <= 0 {
		watchInterval = 1800
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
		path, err = normalizeCloud115DirectoryPath(path, "115 媒体源目录", false)
		if err != nil {
			return nil, err
		}
		watchPath, err = normalizeCloud115DirectoryPath(watchPath, "115 监控目录", !watchEnabled)
		if err != nil {
			return nil, err
		}
		organizeTargetPath, err = normalizeCloud115DirectoryPath(organizeTargetPath, "115 整理目标目录", true)
		if err != nil {
			return nil, err
		}
	}

	// 更新媒体源
	source, err := s.mediaSourceDAO.Update(id, name, sourceType, path, watchPath, cloud115ID, priority, enabled, organizeTargetPath, mediaType, conflictPolicy, operationMode, autoOrganize, watchEnabled, watchInterval, embyLibraryID)
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
//
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
//
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
