package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// GetFiles 获取媒体源文件列表。
func (s *MediaSourceService) GetFiles(sourceID int, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	source, err := s.mediaSourceDAO.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("查询媒体源失败")
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}

	// 浏览为只读查看操作，允许对禁用状态的媒体源进行浏览（禁用仅用于拦截自动整理/监控/写入等自动化流程）。
	switch source.SourceType {
	case domain.SourceTypeLocal:
		return s.getLocalFiles(source, path, page, pageSize, sortField, sortOrder, filter, search)
	case domain.SourceTypeCloud115:
		return s.getCloud115Files(source, path, page, pageSize, sortField, sortOrder, filter, search)
	default:
		return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
	}
}

// getLocalFiles 获取本地文件列表。
func (s *MediaSourceService) getLocalFiles(source *domain.MediaSource, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	fullPath := filepath.Join(source.Path, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		logger.Errorf("MediaSourceService[getLocalFiles] 路径不存在: %s", fullPath)
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		logger.Errorf("MediaSourceService[getLocalFiles] 读取目录失败: %v", err)
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	files := make([]domain.MediaFile, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileID := s.joinRelativePath(path, entry.Name())
		files = append(files, domain.MediaFile{
			ID:           fileID,
			Name:         entry.Name(),
			Path:         fileID,
			Type:         s.fileTypeFromLocal(entry),
			Size:         info.Size(),
			ModifiedTime: info.ModTime(),
			IsDirectory:  entry.IsDir(),
		})
	}

	s.FillFileTmdbTitles(files)

	if search != "" || filter != "" {
		files = s.filterFiles(files, filter, search)
	}
	if sortField != "" {
		files = s.sortFiles(files, sortField, sortOrder)
	}

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

// FillFileTmdbTitles 回填文件和目录的识别标题，优先使用文件身份，兼容历史裸键及各元数据来源。
func (s *MediaSourceService) FillFileTmdbTitles(files []domain.MediaFile) {
	queryKeys := make([]string, 0, len(files)*6)
	for _, file := range files {
		queryKeys = append(queryKeys, browseTitleCacheKeys(file.ID)...)
		queryKeys = append(queryKeys, browseTitleCacheKeys(file.Name)...)
	}
	if len(queryKeys) == 0 {
		return
	}
	caches, err := s.tmdbCacheDAO.GetByQueryKeys(queryKeys)
	if err != nil {
		logger.Warnf("MediaSourceService[FillFileTmdbTitles] 查询识别缓存失败: %v", err)
		return
	}
	for i := range files {
		for _, identity := range []string{files[i].ID, files[i].Name} {
			var latest *dao.TmdbCache
			for _, key := range browseTitleCacheKeys(identity) {
				cache := caches[key]
				if cache != nil && (latest == nil || cache.UpdateTime.After(latest.UpdateTime) || (cache.UpdateTime.Equal(latest.UpdateTime) && cache.ID > latest.ID)) {
					latest = cache
				}
			}
			if latest != nil {
				files[i].TmdbTitle = latest.Title
				break
			}
		}
	}
}

func browseTitleCacheKeys(identity string) []string {
	if identity == "" {
		return nil
	}
	svc := &TmdbService{}
	keys := []string{identity}
	for _, source := range []string{"auto", "tmdb", "metatube"} {
		key := svc.buildCacheKeyForSource(identity, "", source)
		if key != identity {
			keys = append(keys, key)
		}
	}
	return keys
}

// getCloud115Files 获取115云盘文件列表占位结果，实际数据由 Controller 使用 115 Client 填充。
func (s *MediaSourceService) getCloud115Files(source *domain.MediaSource, path string, page, pageSize int, sortField, sortOrder, filter, search string) (*domain.FileListResult, error) {
	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil {
		return nil, fmt.Errorf("查询115账号失败")
	}
	if cloud115 == nil {
		return nil, fmt.Errorf("115账号不存在")
	}

	cid := path
	if cid == "" {
		cid = source.Path
	}

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
