package service

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func (s *OrganizeService) organizeCloud115File(source *domain.MediaSource, preview OrganizePreview, conflictPolicy, operationMode string) (*OrganizeResult, error) {
	operationMode = normalizeOrganizeOperationMode(operationMode)
	if source.Cloud115ID == nil {
		return nil, fmt.Errorf("未绑定115账号")
	}

	cloud115, err := s.cloud115DAO.GetByID(*source.Cloud115ID)
	if err != nil || cloud115 == nil {
		return nil, fmt.Errorf("获取账号信息失败: %v", err)
	}

	// 这里的 targetPath 需要解析为目标目录的 CID
	targetPath := preview.TargetPath
	if targetPath == "" {
		targetPath = "/"
	}

	targetCID, err := s.resolve115CID(targetPath, *source.Cloud115ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("解析或创建115目标目录失败: %v", err)
	}

	fileID := preview.CloudID // 使用 115 云盘的真实内部 ID (CID/PickCode)
	if fileID == "" {
		fileID = preview.FileID // 备用
	}

	needRename := preview.NewName != "" && preview.NewName != preview.FileName

	// 1. 移动或复制
	switch operationMode {
	case organizeOperationMove:
		if needRename {
			err = s.client.RenameFile(fileID, preview.NewName, *source.Cloud115ID, cloud115.Cookie)
			if err != nil {
				return nil, fmt.Errorf("115重命名失败: %v", err)
			}
			time.Sleep(200 * time.Millisecond) // 防风控
		}
		err = s.client.MoveFile115(fileID, targetCID, *source.Cloud115ID, cloud115.Cookie)
		if err != nil {
			return nil, fmt.Errorf("115移动文件失败: %v", err)
		}
		logger.Infof("OrganizeService[organizeCloud115File] 移动成功 %s -> CID %s", preview.FileName, targetCID)
	case organizeOperationCopy:
		var beforeFiles map[string]struct{}
		if needRename {
			beforeFiles, err = s.listCloud115TargetFileIDs(targetCID, *source.Cloud115ID, cloud115.Cookie)
			if err != nil {
				return nil, fmt.Errorf("复制前读取115目标目录失败: %v", err)
			}
		}
		err = s.client.CopyFile(fileID, targetCID, *source.Cloud115ID, cloud115.Cookie)
		if err != nil {
			return nil, fmt.Errorf("115复制文件失败: %v", err)
		}
		if needRename {
			time.Sleep(200 * time.Millisecond) // 等待副本出现在目录列表中
			copiedFileID, findErr := s.findNewlyCopiedCloud115FileID(targetCID, beforeFiles, *source.Cloud115ID, cloud115.Cookie)
			if findErr != nil {
				return nil, fmt.Errorf("定位115复制后的副本失败: %v", findErr)
			}
			if err := s.client.RenameFile(copiedFileID, preview.NewName, *source.Cloud115ID, cloud115.Cookie); err != nil {
				return nil, fmt.Errorf("115复制后重命名副本失败: %v", err)
			}
		}
		logger.Infof("OrganizeService[organizeCloud115File] 复制成功 %s -> CID %s", preview.FileName, targetCID)
	case organizeOperationHardLink, organizeOperationSymLink:
		return nil, fmt.Errorf("115云盘暂不支持%s整理", organizeOperationActionName(operationMode))
	default:
		return nil, fmt.Errorf("不支持的整理方式: %s", operationMode)
	}
	time.Sleep(200 * time.Millisecond) // 防风控
	s.invalidateCloud115ListCache(*source.Cloud115ID)

	return &OrganizeResult{
		FileID:   preview.FileID,
		FileName: preview.NewName, // 最后使用的新名字
		Success:  true,
		Skipped:  false,
		Message:  fmt.Sprintf("云盘整理并%s成功", organizeOperationActionName(operationMode)),
		OldPath:  preview.FilePath,
		NewPath:  preview.NewPath,
	}, nil
}

func (s *OrganizeService) listCloud115TargetFileIDs(targetCID string, cloud115ID int, cookie string) (map[string]struct{}, error) {
	targetCIDInt, err := strconv.Atoi(targetCID)
	if err != nil {
		return nil, fmt.Errorf("目标CID无效: %w", err)
	}

	resp, err := s.client.GetFileList(targetCIDInt, 1, 0, 1000, cloud115ID, cookie)
	if err != nil {
		return nil, err
	}

	fileIDs := make(map[string]struct{})
	if resp == nil {
		return fileIDs, nil
	}
	for _, file := range resp.Files {
		if file.FileID == "" {
			continue
		}
		fileIDs[file.FileID] = struct{}{}
	}
	return fileIDs, nil
}

func (s *OrganizeService) findNewlyCopiedCloud115FileID(targetCID string, beforeFiles map[string]struct{}, cloud115ID int, cookie string) (string, error) {
	afterFiles, err := s.listCloud115TargetFileIDs(targetCID, cloud115ID, cookie)
	if err != nil {
		return "", err
	}
	for fileID := range afterFiles {
		if _, exists := beforeFiles[fileID]; exists {
			continue
		}
		return fileID, nil
	}
	return "", fmt.Errorf("未找到新增副本")
}

// organizeFile 按媒体源 ID 整理单个文件（先查源再委托 organizeFileForSource）

func (s *OrganizeService) organizeFile(sourceID int, preview OrganizePreview, conflictPolicy, operationMode string) (*OrganizeResult, error) {
	source, err := s.mediaSourceService.GetByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取媒体源失败: %v", err)
	}
	if source == nil {
		return nil, fmt.Errorf("媒体源不存在")
	}
	return s.organizeFileForSource(source, preview, conflictPolicy, operationMode)
}

// organizeFileForSource 整理单个文件（直接消费已构造的 *MediaSource，避免重复 GetByID）。
// 供 OrganizeDirectoryForSource 在「已持有媒体源」场景下复用（含临时源）。
func (s *OrganizeService) organizeFileForSource(source *domain.MediaSource, preview OrganizePreview, conflictPolicy, operationMode string) (*OrganizeResult, error) {
	operationMode = normalizeOrganizeOperationMode(operationMode)
	if source == nil {
		return nil, fmt.Errorf("媒体源不能为空")
	}
	if source.SourceType != domain.SourceTypeLocal && source.SourceType != domain.SourceTypeCloud115 {
		return nil, fmt.Errorf("当前仅支持本地和115源文件整理")
	}

	// 115云盘处理逻辑
	if source.SourceType == domain.SourceTypeCloud115 {
		return s.organizeCloud115File(source, preview, conflictPolicy, operationMode)
	}

	// 以下为本地源处理逻辑
	sourcePath := filepath.Join(source.Path, preview.FilePath)
	targetPath := preview.NewPath

	if _, err := os.Stat(sourcePath); err != nil {
		return nil, fmt.Errorf("源文件不存在: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("创建目标目录失败: %v", err)
	}
	if conflictPolicy == "" {
		conflictPolicy = "skip"
	}
	if _, err := os.Stat(targetPath); err == nil {
		switch conflictPolicy {
		case "skip":
			return &OrganizeResult{
				FileID:   preview.FileID,
				FileName: preview.FileName,
				Success:  true,
				Skipped:  true,
				Message:  "目标文件已存在，已跳过",
				OldPath:  sourcePath,
				NewPath:  targetPath,
			}, nil
		case "overwrite":
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return nil, fmt.Errorf("删除冲突文件失败: %v", removeErr)
			}
		case "suffix":
			targetPath, err = s.buildUniqueTargetPath(targetPath)
			if err != nil {
				return nil, fmt.Errorf("生成冲突文件新路径失败: %v", err)
			}
		default:
			return nil, fmt.Errorf("不支持的冲突策略: %s", conflictPolicy)
		}
	}

	switch operationMode {
	case organizeOperationMove:
		if err := s.mediaSourceService.MoveFile(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("移动文件失败: %v", err)
		}
	case organizeOperationCopy:
		if err := s.mediaSourceService.CopyFile(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("复制文件失败: %v", err)
		}
	case organizeOperationHardLink:
		if err := s.mediaSourceService.CreateHardLink(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("创建硬链接失败: %v", err)
		}
	case organizeOperationSymLink:
		if err := s.mediaSourceService.CreateSymbolicLink(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("创建软链接失败: %v", err)
		}
	default:
		return nil, fmt.Errorf("不支持的整理方式: %s", operationMode)
	}

	return &OrganizeResult{
		FileID:   preview.FileID,
		FileName: preview.NewName,
		Success:  true,
		Skipped:  false,
		Message:  organizeOperationSuccessMessage(operationMode),
		OldPath:  sourcePath,
		NewPath:  targetPath,
	}, nil
}
