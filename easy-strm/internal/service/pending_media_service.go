package service

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

// PendingMediaRunResult 待处理媒体重新入库结果。
type PendingMediaRunResult struct {
	Item *domain.PendingMediaItem `json:"data"`
	Task interface{}              `json:"task"`
}

// PendingMediaService 待处理媒体服务。
// 负责待处理项查询、识别修正、状态更新与重新入库编排。
type PendingMediaService struct {
	pendingDAO *dao.PendingMediaDAO
	pipeline   *MediaLibraryPipelineService
}

// NewPendingMediaService 创建待处理媒体服务实例。
func NewPendingMediaService(pendingDAO *dao.PendingMediaDAO, pipeline *MediaLibraryPipelineService) *PendingMediaService {
	return &PendingMediaService{pendingDAO: pendingDAO, pipeline: pipeline}
}

// List 查询待处理媒体列表。
func (s *PendingMediaService) List(status, mediaType string, sourceID int) ([]*domain.PendingMediaItem, error) {
	return s.pendingDAO.List(status, mediaType, sourceID)
}

// Create 创建待处理媒体项。
func (s *PendingMediaService) Create(item *domain.PendingMediaItem) (*domain.PendingMediaItem, error) {
	return s.pendingDAO.Create(item)
}

// Identify 更新待处理媒体的人工识别信息。
func (s *PendingMediaService) Identify(id, tmdbID, year, season, episode int, title, mediaType string) (*domain.PendingMediaItem, error) {
	return s.pendingDAO.UpdateIdentify(id, tmdbID, year, season, episode, title, mediaType)
}

// Run 将待处理媒体重新送入媒体库流水线。
func (s *PendingMediaService) Run(id int) (*PendingMediaRunResult, error) {
	item, err := s.pendingDAO.UpdateStatus(id, "running", "", "")
	if err != nil {
		return nil, err
	}
	result, err := s.pipeline.ProcessPendingItem(id)
	if err != nil {
		return nil, err
	}
	if refreshed, refreshErr := s.pendingDAO.GetByID(id); refreshErr == nil && refreshed != nil {
		item = refreshed
	}
	return &PendingMediaRunResult{Item: item, Task: result}, nil
}

// Ignore 将待处理媒体标记为忽略。
func (s *PendingMediaService) Ignore(id int) (*domain.PendingMediaItem, error) {
	return s.pendingDAO.UpdateStatus(id, "ignored", "用户忽略", "")
}
