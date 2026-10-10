package service

import (
	"context"
	"fmt"

	"easy-strm/internal/domain"
)

// FileOperationBatchLimit 限制单次同步批量文件操作的项数，防止协程与结果内存无界增长。
const FileOperationBatchLimit = 500

type fileOperationSource struct {
	source *domain.MediaSource
	err    error
}

type fileOperationAccount struct {
	account *domain.Cloud115
	err     error
}

func (s *FileOperationService) operationContext() context.Context {
	if s.requestContext != nil {
		return s.requestContext
	}
	return context.Background()
}

func (s *FileOperationService) operationSource(id int) (*domain.MediaSource, error) {
	if err := s.operationContext().Err(); err != nil {
		return nil, err
	}
	if value, present := s.preparedSources[id]; present {
		return value.source, value.err
	}
	return s.mediaSourceService.GetByIDContext(s.operationContext(), id)
}

func (s *FileOperationService) operationAccount(id int) (*domain.Cloud115, error) {
	if err := s.operationContext().Err(); err != nil {
		return nil, err
	}
	var account *domain.Cloud115
	var err error
	if value, present := s.preparedAccounts[id]; present {
		account, err = value.account, value.err
	} else {
		if s.cloud115DAO == nil {
			return nil, fmt.Errorf("115账号仓库未初始化")
		}
		account, err = s.cloud115DAO.GetByIDContext(s.operationContext(), id)
	}
	if err == nil && account == nil {
		err = fmt.Errorf("115账号不存在: %d", id)
	}
	return account, err
}

func (s *FileOperationService) runBatchItem(operation string, sourceID, targetID int, item domain.BatchOperationItem) (domain.FileOperationResult, bool) {
	var result *domain.FileOperationResult
	err := s.operationContext().Err()
	if err == nil {
		switch operation {
		case "move":
			result, err = s.MoveFile(sourceID, item.FileID, item.TargetPath)
		case "copy":
			result, err = s.CopyFile(sourceID, targetID, item.FileID, item.TargetPath, false)
		case "delete":
			result, err = s.DeleteFile(sourceID, []string{item.FileID})
		case "rename":
			result, err = s.RenameFile(sourceID, item.FileID, item.NewName)
		}
	}
	if err != nil {
		return domain.FileOperationResult{Success: false, Message: err.Error(), Source: item.FileID}, false
	}
	return *result, true
}
