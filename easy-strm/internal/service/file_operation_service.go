package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/deadblue/elevengo"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// FileOperationService 文件操作服务
// 提供文件的移动、复制、删除、重命名等操作
type FileOperationService struct {
	mediaSourceService *MediaSourceService
	cloud115DAO        *dao.Cloud115DAO
	cloud115Client     Cloud115Client
}

// NewFileOperationService 创建文件操作服务实例
// 参数:
//   - mediaSourceService: 媒体源服务
//   - cloud115DAO: 115账号DAO
//
// 返回:
//   - *FileOperationService: 文件操作服务实例
func NewFileOperationService(mediaSourceService *MediaSourceService, cloud115DAO *dao.Cloud115DAO, cloud115Client ...Cloud115Client) *FileOperationService {
	var client Cloud115Client
	if len(cloud115Client) > 0 {
		client = cloud115Client[0]
	}
	return &FileOperationService{
		mediaSourceService: mediaSourceService,
		cloud115DAO:        cloud115DAO,
		cloud115Client:     client,
	}
}

// MoveFile 移动文件
// 参数:
//   - sourceID: 源媒体源ID
//   - fileID: 文件ID
//   - targetPath: 目标路径
//
// 返回:
//   - *domain.FileOperationResult: 操作结果
//   - error: 错误信息
func (s *FileOperationService) MoveFile(sourceID int, fileID, targetPath string) (*domain.FileOperationResult, error) {
	if fileID == "" {
		return nil, errors.New("文件ID不能为空")
	}

	// 获取源媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取源媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("源媒体源不存在")
	}
	if !source.Enabled {
		return nil, fmt.Errorf("源媒体源已禁用")
	}

	// 根据媒体源类型处理
	if source.SourceType == domain.SourceTypeLocal {
		sourcePath := filepath.Join(source.Path, fileID)
		targetFilePath := filepath.Join(source.Path, targetPath, filepath.Base(fileID))

		// 检查源文件是否存在
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("源文件不存在: %s", fileID)
		}

		// 确保目标目录存在
		if err := os.MkdirAll(filepath.Dir(targetFilePath), 0755); err != nil {
			return nil, fmt.Errorf("创建目标目录失败: %v", err)
		}

		// 移动文件
		if err := os.Rename(sourcePath, targetFilePath); err != nil {
			// 如果是跨文件系统，需要复制后删除
			if err := s.copyFile(sourcePath, targetFilePath); err != nil {
				return nil, fmt.Errorf("移动文件失败: %v", err)
			}
			// 删除源文件
			os.Remove(sourcePath)
		}

		logger.Infof("FileOperationService[MoveFile] 移动文件成功: %s -> %s", fileID, targetPath)
		return &domain.FileOperationResult{
			Success: true,
			Message: "移动成功",
			Source:  fileID,
			Target:  targetFilePath,
		}, nil
	} else if source.SourceType == domain.SourceTypeCloud115 {
		cloud115, err := s.getCloud115Account(source)
		if err != nil {
			return nil, err
		}
		targetDirID, err := s.resolveCloud115TargetDir(targetPath, cloud115.ID, cloud115.Cookie)
		if err != nil {
			return nil, err
		}
		if err := s.cloud115Client.MoveFile115(fileID, targetDirID, cloud115.ID, cloud115.Cookie); err != nil {
			return nil, fmt.Errorf("移动115文件失败: %v", err)
		}

		logger.Infof("FileOperationService[MoveFile] 115文件移动成功: %s -> %s", fileID, targetDirID)
		return &domain.FileOperationResult{
			Success: true,
			Message: "移动成功",
			Source:  fileID,
			Target:  targetDirID,
		}, nil
	}

	return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
}

// CopyFile 复制文件
// 参数:
//   - sourceID: 源媒体源ID
//   - targetID: 目标媒体源ID
//   - fileID: 文件ID
//   - targetPath: 目标路径
//   - deleteSource: 是否删除源文件
//
// 返回:
//   - *domain.FileOperationResult: 操作结果
//   - error: 错误信息
func (s *FileOperationService) CopyFile(sourceID, targetID int, fileID, targetPath string, deleteSource bool) (*domain.FileOperationResult, error) {
	if fileID == "" {
		return nil, errors.New("文件ID不能为空")
	}

	// 获取源媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取源媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("源媒体源不存在")
	}

	// 获取目标媒体源
	target, err := s.mediaSourceService.GetByID(targetID)
	if err != nil {
		return nil, fmt.Errorf("获取目标媒体源失败: %v", err)
	}
	if target == nil {
		return nil, fmt.Errorf("目标媒体源不存在")
	}

	// 根据媒体源类型处理
	if source.SourceType == domain.SourceTypeLocal && target.SourceType == domain.SourceTypeLocal {
		sourcePath := filepath.Join(source.Path, fileID)
		targetFilePath := filepath.Join(target.Path, targetPath, filepath.Base(fileID))

		// 检查源文件是否存在
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("源文件不存在: %s", fileID)
		}

		// 确保目标目录存在
		if err := os.MkdirAll(filepath.Dir(targetFilePath), 0755); err != nil {
			return nil, fmt.Errorf("创建目标目录失败: %v", err)
		}

		// 复制文件
		if err := s.copyFile(sourcePath, targetFilePath); err != nil {
			return nil, fmt.Errorf("复制文件失败: %v", err)
		}

		// 如果是移动操作，删除源文件
		if deleteSource {
			os.Remove(sourcePath)
		}

		logger.Infof("FileOperationService[CopyFile] 复制文件成功: %s -> %s", fileID, targetPath)
		return &domain.FileOperationResult{
			Success: true,
			Message: "复制成功",
			Source:  fileID,
			Target:  targetFilePath,
		}, nil
	} else if source.SourceType == domain.SourceTypeCloud115 && target.SourceType == domain.SourceTypeCloud115 {
		sourceAccount, err := s.getCloud115Account(source)
		if err != nil {
			return nil, err
		}
		if target.Cloud115ID == nil {
			return nil, fmt.Errorf("目标媒体源未关联115账号")
		}
		if *target.Cloud115ID != sourceAccount.ID {
			return nil, fmt.Errorf("暂不支持跨115账号复制文件")
		}

		targetDirPath := targetPath
		if strings.TrimSpace(targetDirPath) == "" {
			targetDirPath = target.Path
		}
		targetDirID, err := s.resolveCloud115TargetDir(targetDirPath, sourceAccount.ID, sourceAccount.Cookie)
		if err != nil {
			return nil, err
		}
		if err := s.cloud115Client.CopyFile(fileID, targetDirID, sourceAccount.ID, sourceAccount.Cookie); err != nil {
			return nil, fmt.Errorf("复制115文件失败: %v", err)
		}
		if deleteSource {
			if err := s.deleteCloud115Path(source, fileID); err != nil {
				return nil, fmt.Errorf("复制后删除115源文件失败: %v", err)
			}
		}

		logger.Infof("FileOperationService[CopyFile] 115文件复制成功: %s -> %s", fileID, targetDirID)
		return &domain.FileOperationResult{
			Success: true,
			Message: "复制成功",
			Source:  fileID,
			Target:  targetDirID,
		}, nil
	}

	return nil, fmt.Errorf("不支持的媒体源类型组合: %s -> %s", source.SourceType, target.SourceType)
}

// DeleteFile 删除文件（支持批量删除）
// 参数:
//   - sourceID: 媒体源ID
//   - fileIDs: 文件ID列表
//
// 返回:
//   - *domain.FileOperationResult: 操作结果
//   - error: 错误信息
func (s *FileOperationService) DeleteFile(sourceID int, fileIDs []string) (*domain.FileOperationResult, error) {
	if len(fileIDs) == 0 {
		return nil, errors.New("文件ID列表不能为空")
	}

	// 获取源媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取源媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("源媒体源不存在")
	}

	var successCount int
	var failedFiles []string

	// 遍历删除每个文件
	for _, fileID := range fileIDs {
		if source.SourceType == domain.SourceTypeCloud115 {
			if err := s.deleteCloud115Path(source, fileID); err != nil {
				failedFiles = append(failedFiles, fileID)
				logger.Warnf("FileOperationService[DeleteFile] 删除115文件失败: %s, 错误: %v", fileID, err)
				continue
			}
			logger.Infof("FileOperationService[DeleteFile] 删除115文件成功: %s", fileID)
			successCount++
			continue
		}

		if source.SourceType == domain.SourceTypeLocal {
			sourcePath := filepath.Join(source.Path, fileID)
			if err := s.deleteLocalPath(sourcePath); err != nil {
				failedFiles = append(failedFiles, fileID)
				logger.Warnf("FileOperationService[DeleteFile] 删除文件失败: %s, 错误: %v", fileID, err)
				continue
			}
			logger.Infof("FileOperationService[DeleteFile] 删除文件成功: %s", fileID)
			successCount++
		} else {
			return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
		}
	}

	// 构建返回结果
	message := fmt.Sprintf("成功删除 %d 个文件", successCount)
	if len(failedFiles) > 0 {
		message = fmt.Sprintf("成功删除 %d 个文件，失败 %d 个", successCount, len(failedFiles))
	}

	return &domain.FileOperationResult{
		Success: len(failedFiles) == 0,
		Message: message,
		Source:  fmt.Sprintf("成功: %d, 失败: %d", successCount, len(failedFiles)),
	}, nil
}

func (s *FileOperationService) deleteLocalPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func (s *FileOperationService) getCloud115Account(source *domain.MediaSource) (*domain.Cloud115, error) {
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("媒体源未关联115账号")
	}
	if s.cloud115DAO == nil {
		return nil, fmt.Errorf("115账号仓库未初始化")
	}
	if s.cloud115Client == nil {
		return nil, fmt.Errorf("115文件操作客户端未初始化")
	}

	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil {
		return nil, fmt.Errorf("获取115账号失败: %v", err)
	}
	if cloud115 == nil {
		return nil, fmt.Errorf("115账号不存在")
	}
	return cloud115, nil
}

func (s *FileOperationService) resolveCloud115TargetDir(targetPath string, cloud115ID int, cookie string) (string, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return "0", nil
	}
	if isCloud115CID(targetPath) {
		return targetPath, nil
	}
	targetDirID, err := s.cloud115Client.MkdirAll115(targetPath, cloud115ID, cookie)
	if err != nil {
		return "", fmt.Errorf("创建115目标目录失败: %v", err)
	}
	return targetDirID, nil
}

func isCloud115CID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// RenameFile 重命名文件
// 参数:
//   - sourceID: 媒体源ID
//   - fileID: 文件ID
//   - newName: 新文件名（可包含或不包含扩展名）
//
// 返回:
//   - *domain.FileOperationResult: 操作结果
//   - error: 错误信息
func (s *FileOperationService) RenameFile(sourceID int, fileID, newName string) (*domain.FileOperationResult, error) {
	if fileID == "" {
		return nil, errors.New("文件ID不能为空")
	}
	if newName == "" {
		return nil, errors.New("新文件名不能为空")
	}

	// 获取源媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取源媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("源媒体源不存在")
	}

	// 根据媒体源类型处理
	if source.SourceType == domain.SourceTypeLocal {
		sourcePath := filepath.Join(source.Path, fileID)
		dir := filepath.Dir(sourcePath)
		originalExt := filepath.Ext(fileID)

		// 智能处理扩展名：如果 newName 已包含扩展名，则直接使用；否则追加原扩展名
		// 业务背景：用户可能传入 "video.mp4" 或 "video"，需兼容两种场景
		newNameExt := filepath.Ext(newName)
		finalName := newName
		if newNameExt == "" {
			// newName 没有扩展名，追加原文件扩展名
			finalName = newName + originalExt
		}
		// 否则 newName 已包含扩展名，直接使用

		newFilePath := filepath.Join(dir, finalName)

		// 检查是否与原文件名相同
		if sourcePath == newFilePath {
			return &domain.FileOperationResult{
				Success: true,
				Message: "文件名未改变",
				Source:  fileID,
			}, nil
		}

		// 重命名文件
		if err := os.Rename(sourcePath, newFilePath); err != nil {
			return nil, fmt.Errorf("重命名失败: %v", err)
		}

		logger.Infof("FileOperationService[RenameFile] 重命名成功: %s -> %s", fileID, finalName)
		return &domain.FileOperationResult{
			Success: true,
			Message: "重命名成功",
			Source:  fileID,
			Target:  newFilePath,
		}, nil
	} else if source.SourceType == domain.SourceTypeCloud115 {
		// 115云盘文件重命名
		logger.Infof("FileOperationService[RenameFile] 115文件重命名: %s -> %s", fileID, newName)

		// 检查 Cloud115ID 是否存在
		if source.Cloud115ID == nil {
			return nil, fmt.Errorf("媒体源未关联115账号")
		}

		// 获取115账号信息
		cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
		if err != nil {
			return nil, fmt.Errorf("获取115账号失败: %v", err)
		}

		// 智能处理扩展名：如果 newName 已包含扩展名，则直接使用；否则追加原扩展名
		originalExt := filepath.Ext(fileID)
		newNameExt := filepath.Ext(newName)
		finalName := newName
		if newNameExt == "" {
			finalName = newName + originalExt
		}

		// 调用115 API进行重命名
		// 使用 elevengo 进行重命名
		agent := elevengo.New()
		cr := &elevengo.Credential{}
		// 解析 cookie
		cookieParts := strings.Split(cloud115.Cookie, ";")
		for _, part := range cookieParts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "UID=") {
				cr.UID = strings.TrimPrefix(part, "UID=")
			} else if strings.HasPrefix(part, "CID=") {
				cr.CID = strings.TrimPrefix(part, "CID=")
			} else if strings.HasPrefix(part, "SEID=") {
				cr.SEID = strings.TrimPrefix(part, "SEID=")
			} else if strings.HasPrefix(part, "KID=") {
				cr.KID = strings.TrimPrefix(part, "KID=")
			}
		}
		if err := agent.CredentialImport(cr); err != nil {
			return nil, fmt.Errorf("导入凭证失败: %v", err)
		}
		if err := agent.FileRename(fileID, finalName); err != nil {
			return nil, fmt.Errorf("重命名失败: %v", err)
		}

		logger.Infof("FileOperationService[RenameFile] 115文件重命名成功: %s -> %s", fileID, finalName)
		return &domain.FileOperationResult{
			Success: true,
			Message: "重命名成功",
			Source:  fileID,
			Target:  finalName,
		}, nil
	}

	return nil, fmt.Errorf("不支持的媒体源类型: %s", source.SourceType)
}

// BatchOperation 批量文件操作
// 参数:
//   - operation: 操作类型 (move/copy/delete/rename)
//   - sourceID: 源媒体源ID
//   - targetID: 目标媒体源ID (复制操作需要)
//   - items: 操作项列表
//
// 返回:
//   - *domain.BatchOperationResult: 批量操作结果
//   - error: 错误信息
func (s *FileOperationService) BatchOperation(operation string, sourceID, targetID int, items []domain.BatchOperationItem) (*domain.BatchOperationResult, error) {
	result := &domain.BatchOperationResult{
		Total:   len(items),
		Results: []domain.FileOperationResult{},
	}

	if len(items) == 0 {
		return nil, errors.New("操作项列表不能为空")
	}

	// 获取源媒体源
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取源媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("源媒体源不存在")
	}

	// 使用 WaitGroup 和 mutex 进行并发控制
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, item := range items {
		wg.Add(1)
		go func(item domain.BatchOperationItem) {
			defer wg.Done()

			var opResult *domain.FileOperationResult
			var err error

			switch operation {
			case "move":
				opResult, err = s.MoveFile(sourceID, item.FileID, item.TargetPath)
			case "copy":
				opResult, err = s.CopyFile(sourceID, targetID, item.FileID, item.TargetPath, false)
			case "delete":
				opResult, err = s.DeleteFile(sourceID, []string{item.FileID})
			case "rename":
				opResult, err = s.RenameFile(sourceID, item.FileID, item.NewName)
			default:
				err = fmt.Errorf("不支持的操作类型: %s", operation)
			}

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Failed++
				result.Results = append(result.Results, domain.FileOperationResult{
					Success: false,
					Message: err.Error(),
					Source:  item.FileID,
				})
			} else {
				result.Success++
				result.Results = append(result.Results, *opResult)
			}
		}(item)
	}

	wg.Wait()

	logger.Infof("FileOperationService[BatchOperation] 批量操作完成: 类型=%s, 成功=%d, 失败=%d",
		operation, result.Success, result.Failed)
	return result, nil
}

// copyFile 复制文件（内部方法）
// 参数:
//   - src: 源文件路径
//   - dst: 目标文件路径
//
// 返回:
//   - error: 错误信息
func (s *FileOperationService) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// 使用缓冲区复制
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := srcFile.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		if _, err := dstFile.Write(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

func (s *FileOperationService) deleteCloud115Path(source *domain.MediaSource, fileID string) error {
	if source.Cloud115ID == nil {
		return fmt.Errorf("媒体源未关联115账号")
	}

	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil {
		return fmt.Errorf("获取115账号失败: %v", err)
	}
	if cloud115 == nil {
		return fmt.Errorf("115账号不存在")
	}

	cred := &driver.Credential{}
	if err := cred.FromCookie(cloud115.Cookie); err != nil {
		return fmt.Errorf("解析115 Cookie失败: %v", err)
	}

	client := driver.New(driver.UA(driver.UA115Browser)).ImportCredential(cred)
	if err := client.Delete(fileID); err != nil {
		return fmt.Errorf("调用115删除接口失败: %v", err)
	}

	return nil
}
