package service

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *OrganizeService) filterFilesByIDs(files []domain.MediaFile, fileIDs []string) []domain.MediaFile {
	if len(fileIDs) == 0 {
		return files
	}

	// 统一规范化用户选中的 ID
	targets := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		targets = append(targets, s.normalizePath(id))
	}

	filtered := make([]domain.MediaFile, 0)
	for _, file := range files {
		fID := s.normalizePath(file.ID)
		cloudID := s.normalizePath(file.CID)
		match := false
		for _, target := range targets {
			// 直接匹配文件名或目录名
			if fID == target {
				match = true
				break
			}
			// 115 云盘场景下，允许使用文件/目录内部 ID 精确匹配
			if cloudID != "" && cloudID == target {
				match = true
				break
			}
			// 目录包含匹配
			if strings.HasPrefix(fID, target+"/") {
				match = true
				break
			}
		}

		if match {
			logger.Infof("OrganizeService[filterFilesByIDs] 匹配成功: %s -> Normalized: %s (在选择范围内)", file.ID, fID)
			filtered = append(filtered, file)
		} else {
			// 为了调试，抽样打印不匹配的情况
			if len(filtered) == 0 && len(files) > 0 {
				logger.Debugf("OrganizeService[filterFilesByIDs] 未匹配: fID=%s, targets=%v", fID, targets)
			}
		}
	}

	return filtered
}

func (s *OrganizeService) filterScannedFiles(source *domain.MediaSource, files []domain.MediaFile, fileIDs []string) []domain.MediaFile {
	if source != nil && source.SourceType == domain.SourceTypeCloud115 {
		return files
	}
	return s.filterFilesByIDs(files, fileIDs)
}

func (s *OrganizeService) appendMissingFilePreviews(previews []OrganizePreview, files []domain.MediaFile, fileIDs []string) []OrganizePreview {
	if len(fileIDs) == 0 {
		return previews
	}

	existingPreviewKeys := make(map[string]struct{}, len(previews)*2)
	for _, preview := range previews {
		if fileID := strings.TrimSpace(preview.FileID); fileID != "" {
			existingPreviewKeys["file:"+s.normalizePath(fileID)] = struct{}{}
		}
		if cloudID := strings.TrimSpace(preview.CloudID); cloudID != "" {
			existingPreviewKeys["cloud:"+s.normalizePath(cloudID)] = struct{}{}
		}
	}

	scannedFileKeys := make(map[string]struct{}, len(files)*2)
	for _, file := range files {
		if fileID := strings.TrimSpace(file.ID); fileID != "" {
			scannedFileKeys["file:"+s.normalizePath(fileID)] = struct{}{}
		}
		if cloudID := strings.TrimSpace(file.CID); cloudID != "" {
			scannedFileKeys["cloud:"+s.normalizePath(cloudID)] = struct{}{}
		}
	}

	for _, fileID := range fileIDs {
		normalizedID := s.normalizePath(fileID)
		fileKey := "file:" + normalizedID
		cloudKey := "cloud:" + normalizedID
		if _, ok := scannedFileKeys[fileKey]; ok {
			continue
		}
		if _, ok := scannedFileKeys[cloudKey]; ok {
			continue
		}
		if _, ok := existingPreviewKeys[fileKey]; ok {
			continue
		}
		if _, ok := existingPreviewKeys[cloudKey]; ok {
			continue
		}

		previews = append(previews, OrganizePreview{
			FileID:        fileID,
			FileName:      filepath.Base(fileID),
			FilePath:      fileID,
			IdentifyError: "源文件不存在或已被移除",
		})
	}

	return previews
}

func (s *OrganizeService) normalizePath(p string) string {
	// 统合 slash 格式
	p = filepath.ToSlash(p)
	// 移除可能存在的驱动器盘符 (Windows 环境处理)
	if len(p) > 1 && p[1] == ':' {
		p = p[2:]
	}
	// 去除首尾连续斜杠
	p = strings.Trim(p, "/")
	return p
}

// isPathRelevant 判定当前扫描出的相对路径是否与待选 ID 相关

func (s *OrganizeService) isPathRelevant(fPath string, targets []string) (isExact bool, isAncestor bool) {
	if len(targets) == 0 {
		return false, true // 如果没有指定 ID，则默认为全量扫描
	}

	fPath = s.normalizePath(fPath)
	for _, t := range targets {
		if fPath == t {
			return true, false // 刚好是选中的目标（如选中的文件或选中的目录）
		}
		// 目标路径以当前路径开头且后面紧跟斜杠，说明当前是目标的祖先
		if strings.HasPrefix(t, fPath+"/") {
			return false, true
		}
	}
	return false, false
}

// scanFiles 扫描文件

func (s *OrganizeService) isPathOrIDRelevant(fPath string, ids []string, targets []string) (isExact bool, isAncestor bool) {
	isExact, isAncestor = s.isPathRelevant(fPath, targets)
	if isExact || isAncestor {
		return isExact, isAncestor
	}
	if len(targets) == 0 {
		return false, true
	}

	for _, id := range ids {
		if id == "" {
			continue
		}
		normalizedID := s.normalizePath(id)
		for _, target := range targets {
			if normalizedID == target {
				return true, false
			}
		}
	}
	return false, false
}

func (s *OrganizeService) scanFiles(source *domain.MediaSource, path, mediaType string, fileIDs []string, includeMetadata bool) ([]domain.MediaFile, error) {
	// 格式化目标 ID 以便匹配
	targets := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		targets = append(targets, s.normalizePath(id))
	}

	// 根据媒体源类型扫描
	if source.SourceType == domain.SourceTypeLocal {
		if len(targets) > 0 {
			directFiles, remainingTargets := s.collectLocalSelectedFiles(source.Path, mediaType, targets, includeMetadata)
			if len(remainingTargets) == 0 {
				return directFiles, nil
			}
			scannedFiles, err := s.scanLocalFiles(source.Path, path, mediaType, remainingTargets, includeMetadata)
			if err != nil {
				return nil, err
			}
			return append(directFiles, scannedFiles...), nil
		}
		return s.scanLocalFiles(source.Path, path, mediaType, targets, includeMetadata)
	} else if source.SourceType == domain.SourceTypeCloud115 {
		return s.scanCloud115Files(source, path, mediaType, targets, includeMetadata)
	}
	return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
}

func (s *OrganizeService) collectLocalSelectedFiles(basePath, mediaType string, targets []string, includeMetadata bool) ([]domain.MediaFile, []string) {
	if len(targets) == 0 {
		return nil, nil
	}

	files := make([]domain.MediaFile, 0, len(targets))
	remainingTargets := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))

	for _, target := range targets {
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}

		fullPath := filepath.Join(basePath, filepath.FromSlash(target))
		info, err := os.Stat(fullPath)
		if err != nil {
			remainingTargets = append(remainingTargets, target)
			continue
		}
		if info.IsDir() {
			remainingTargets = append(remainingTargets, target)
			continue
		}

		ext := strings.ToLower(filepath.Ext(target))
		if !s.isOrganizeVideoFile(ext) {
			continue
		}

		mediaFile := domain.MediaFile{
			ID:          target,
			Name:        filepath.Base(target),
			Path:        target,
			Type:        s.getFileType(ext),
			IsDirectory: false,
			Extension:   ext,
		}
		if includeMetadata {
			mediaFile.Size = info.Size()
			mediaFile.ModifyTime = info.ModTime()
		}
		files = append(files, mediaFile)
	}

	return files, remainingTargets
}

// scanLocalFiles 扫描本地文件 (已添加路径剪枝)

func (s *OrganizeService) scanLocalFiles(basePath, relativePath, mediaType string, targets []string, includeMetadata bool) ([]domain.MediaFile, error) {
	fullPath := filepath.Join(basePath, relativePath)

	// 读取目录
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	files := make([]domain.MediaFile, 0)
	for _, entry := range entries {
		filePath := filepath.Join(relativePath, entry.Name())
		isExact, isAncestor := s.isPathRelevant(filePath, targets)

		if !isExact && !isAncestor {
			continue // 路径无关，跳过扫描
		}

		if entry.IsDir() {
			// 如果是完全匹配的目录，则其下的所有内容都视为选中 (传入空 targets 代表全量扫描子集)
			subTargets := targets
			if isExact {
				subTargets = nil
			}
			// 递归扫描子目录
			subFiles, err := s.scanLocalFiles(basePath, filePath, mediaType, subTargets, includeMetadata)
			if err != nil {
				logger.Warnf("OrganizeService[scanLocalFiles] 扫描子目录失败: %s, error: %v", filePath, err)
				continue
			}
			files = append(files, subFiles...)
		} else {
			// 检查文件类型
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if !s.isOrganizeVideoFile(ext) {
				continue
			}

			mediaFile := domain.MediaFile{
				ID:          filePath,
				Name:        entry.Name(),
				Path:        filePath,
				Type:        s.getFileType(ext),
				IsDirectory: false,
				Extension:   ext,
			}
			if includeMetadata {
				info, err := entry.Info()
				if err != nil {
					continue
				}
				mediaFile.Size = info.Size()
				mediaFile.ModifyTime = info.ModTime()
			}

			files = append(files, mediaFile)
		}
	}

	return files, nil
}

// scanCloud115Files 扫描115云盘文件 (支持批量扫描剪枝)

func (s *OrganizeService) scanCloud115Files(source *domain.MediaSource, cidStr, mediaType string, targets []string, includeMetadata bool) ([]domain.MediaFile, error) {
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("未绑定115账号")
	}

	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("获取账号信息失败: %v", err)
	}

	cid := resolveCloud115ScanRootCID(source.Path)
	// 如果 cidStr 是路径格式，先转为 CID
	if cidStr == "/" {
		cidStr = source.Path
	}
	if cidStr != "" && cidStr != "/" && (strings.Contains(cidStr, "/") || !s.isNumeric(cidStr)) {
		realCID, err := s.client.GetCIDByPath(cidStr, cloud115.ID, cloud115.Cookie)
		if err != nil {
			logger.Warnf("OrganizeService[scanCloud115Files] 无法解析路径 %s 为 CID, 尝试作为 CID 继续: %v", cidStr, err)
			cid = cidStr
		} else {
			cid = realCID
		}
	} else if cidStr != "" {
		cid = cidStr
	}
	if strings.HasPrefix(cid, "/") {
		realCID, resolveErr := s.client.GetCIDByPath(cid, cloud115.ID, cloud115.Cookie)
		if resolveErr != nil {
			return nil, fmt.Errorf("解析 115 媒体源目录失败: %w", resolveErr)
		}
		cid = realCID
	}

	logger.Infof("OrganizeService[scanCloud115Files] 开始执行 115 智能扫描, CID=%s", cid)
	// 115 递归扫描，起始路径设为空，ID 由 relativePath 组成
	return s.scanCloud115RecursiveWithThrottle(cid, "", cloud115.ID, cloud115.Cookie, mediaType, targets, includeMetadata, nil)
}

func resolveCloud115ScanRootCID(sourcePath string) string {
	cid := strings.TrimSpace(sourcePath)
	if cid == "" || cid == "/" {
		return "0"
	}
	return cid
}

// scanCloud115Recursive 递归扫描115云盘 (已添加定向扫描逻辑)

func (s *OrganizeService) scanCloud115Recursive(currentCID, relativePath string, cloud115ID int, cookie, mediaType string, targets []string) ([]domain.MediaFile, error) {
	return s.scanCloud115RecursiveWithThrottle(currentCID, relativePath, cloud115ID, cookie, mediaType, targets, true, nil)
}

func (s *OrganizeService) scanCloud115RecursiveWithThrottle(currentCID, relativePath string, cloud115ID int, cookie, mediaType string, targets []string, includeMetadata bool, lastRequestAt *time.Time) ([]domain.MediaFile, error) {
	resp, err := s.getCloud115FileList(currentCID, cloud115ID, cookie, lastRequestAt)
	if err != nil {
		return nil, err
	}

	files := make([]domain.MediaFile, 0)
	for _, f := range resp {
		// 生成用于匹配的相对路径 ID
		fPath := filepath.Join(relativePath, f.Name)
		categoryID := f.CID
		fileID := f.ID
		isDir := f.IsDirectory

		isExact, isAncestor := s.isPathOrIDRelevant(fPath, []string{fileID, categoryID}, targets)
		if !isExact && !isAncestor {
			// 该文件夹/文件与用户选中项无关，直接剪枝并跳过 API 调用
			logger.Debugf("OrganizeService[scanCloud115Recursive] 路径无关，跳过扫描: %s", fPath)
			continue
		}

		if isDir {
			// 如果该文件夹被完全选中，则子目录不再需要 targets 过滤 (实现全量扫描)
			subTargets := targets
			if isExact {
				subTargets = nil
			}
			// 递归扫子目录
			subFiles, err := s.scanCloud115RecursiveWithThrottle(fileID, fPath, cloud115ID, cookie, mediaType, subTargets, includeMetadata, lastRequestAt)
			if err == nil {
				files = append(files, subFiles...)
			}
		} else {
			ext := strings.ToLower(filepath.Ext(f.Name))
			if !s.isOrganizeVideoFile(ext) {
				continue
			}

			files = append(files, domain.MediaFile{
				ID:          fPath,
				Name:        f.Name,
				Path:        fPath,
				Type:        s.getFileType(ext),
				IsDirectory: false,
				Extension:   ext,
				CID:         fileID, // 115 文件的唯一 ID
			})
		}
	}
	return files, nil
}

func (s *OrganizeService) getCloud115FileList(currentCID string, cloud115ID int, cookie string, lastRequestAt *time.Time) ([]domain.MediaFile, error) {
	cacheKey := fmt.Sprintf("%d:%s", cloud115ID, strings.TrimSpace(currentCID))
	if cached, ok := s.getCloud115CachedFileList(cacheKey); ok {
		return cached, nil
	}

	s.waitCloud115ScanInterval(lastRequestAt)
	cidInt, _ := strconv.Atoi(currentCID)
	resp, err := s.client.GetFileList(cidInt, 1, 0, 1000, cloud115ID, cookie)
	if err != nil {
		return nil, err
	}
	if lastRequestAt != nil {
		*lastRequestAt = time.Now()
	}

	files := make([]domain.MediaFile, 0, len(resp.Files))
	for _, f := range resp.Files {
		categoryID := string(f.CategoryID)
		fileID := f.FileID
		isDir := fileID == "" || f.Type == "folder"
		if isDir && fileID == "" {
			fileID = categoryID
		}
		files = append(files, domain.MediaFile{
			ID:          fileID,
			Name:        f.Name,
			Type:        f.Type,
			IsDirectory: isDir,
			CID:         categoryID,
		})
	}
	s.setCloud115CachedFileList(cacheKey, files)
	return files, nil
}

func (s *OrganizeService) getCloud115CachedFileList(cacheKey string) ([]domain.MediaFile, bool) {
	s.cloud115ListCacheMu.RLock()
	entry, ok := s.cloud115ListCache[cacheKey]
	s.cloud115ListCacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		s.cloud115ListCacheMu.Lock()
		delete(s.cloud115ListCache, cacheKey)
		s.cloud115ListCacheMu.Unlock()
		return nil, false
	}
	files := make([]domain.MediaFile, len(entry.files))
	copy(files, entry.files)
	return files, true
}

func (s *OrganizeService) setCloud115CachedFileList(cacheKey string, files []domain.MediaFile) {
	cloned := make([]domain.MediaFile, len(files))
	copy(cloned, files)

	s.cloud115ListCacheMu.Lock()
	if s.cloud115ListCache == nil {
		s.cloud115ListCache = make(map[string]cloud115ListCacheEntry)
	}
	s.cloud115ListCache[cacheKey] = cloud115ListCacheEntry{
		files:     cloned,
		expiresAt: time.Now().Add(cloud115ListCacheTTL),
	}
	s.cloud115ListCacheMu.Unlock()
}

func (s *OrganizeService) invalidateCloud115ListCache(cloud115ID int) {
	s.cloud115ListCacheMu.Lock()
	defer s.cloud115ListCacheMu.Unlock()
	if len(s.cloud115ListCache) == 0 {
		return
	}

	prefix := fmt.Sprintf("%d:", cloud115ID)
	for cacheKey := range s.cloud115ListCache {
		if strings.HasPrefix(cacheKey, prefix) {
			delete(s.cloud115ListCache, cacheKey)
		}
	}
}

func (s *OrganizeService) waitCloud115ScanInterval(lastRequestAt *time.Time) {
	if lastRequestAt == nil || lastRequestAt.IsZero() {
		return
	}
	wait := cloud115ScanInterval - time.Since(*lastRequestAt)
	if wait > 0 {
		time.Sleep(wait)
	}
}

func (s *OrganizeService) isNumeric(str string) bool {
	_, err := strconv.Atoi(str)
	return err == nil
}

// resolve115CID 解析路径为 115 CID, 逻辑同 MkdirAll115 但包含缓存或业务验证

func (s *OrganizeService) resolve115CID(path string, cloud115ID int, cookie string) (string, error) {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" || path == "/" {
		return "0", nil
	}
	if s.isNumeric(path) {
		return path, nil
	}
	return s.client.MkdirAll115(path, cloud115ID, cookie)
}

// previewFile 预览单个文件
