package service

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// MediaLibraryService 媒体库查询服务。
// 负责把同步索引转换为页面需要的媒体库条目。
type MediaLibraryService struct {
	indexDAO *dao.MediaSyncIndexDAO
}

// NewMediaLibraryService 创建媒体库查询服务实例。
func NewMediaLibraryService(indexDAO *dao.MediaSyncIndexDAO) *MediaLibraryService {
	return &MediaLibraryService{indexDAO: indexDAO}
}

// ListItems 按媒体源与状态查询媒体库条目。
func (s *MediaLibraryService) ListItems(sourceID int, status string) ([]domain.MediaLibraryItem, error) {
	indexes, err := s.indexDAO.ListBySource(sourceID, status)
	if err != nil {
		return nil, err
	}
	items := make([]domain.MediaLibraryItem, 0, len(indexes))
	for _, index := range indexes {
		items = append(items, buildMediaLibraryItem(index))
	}
	return items, nil
}

// GetItem 获取单个媒体库条目。
func (s *MediaLibraryService) GetItem(id int) (*domain.MediaLibraryItem, error) {
	index, err := s.indexDAO.GetByID(id)
	if err != nil || index == nil {
		return nil, err
	}
	item := buildMediaLibraryItem(index)
	return &item, nil
}

func buildMediaLibraryItem(index *domain.MediaSyncIndex) domain.MediaLibraryItem {
	return domain.MediaLibraryItem{
		MediaSyncIndex: *index,
		HasStrm:        index.StrmPath != "",
		HasMetadata:    index.MetadataPath != "",
		HealthStatus:   resolveMediaLibraryHealthStatus(index),
		LatestTaskID:   index.LastTaskID,
	}
}

func resolveMediaLibraryHealthStatus(index *domain.MediaSyncIndex) string {
	if index.SyncStatus == domain.SyncStatusMissing || index.SyncStatus == domain.SyncStatusDeleted {
		return "missing"
	}
	if index.IdentityStatus == domain.IdentityStatusFailed {
		return "identify_failed"
	}
	if index.StrmPath == "" && index.SourceType == domain.SourceTypeCloud115 {
		return "strm_missing"
	}
	return "ok"
}
