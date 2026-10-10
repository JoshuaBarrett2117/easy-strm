package service

import (
	"context"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	"strings"
)

// ShareSelectionStore 定义选择管理所需能力，不包含播放凭据访问。
type ShareSelectionStore interface {
	CheckSelectionSchema(context.Context) error
	ListSelections(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error)
	SelectionDetail(context.Context, string) (domain.ShareSelectionDetail, error)
	ChangeSelection(context.Context, domain.ShareSelectionChange) error
	SyncSelections(context.Context, string) error
}

// ShareSelectionService 提供只读列表及显式改选、刷新用例。
type ShareSelectionService struct{ store ShareSelectionStore }

// NewShareSelectionService 注入候选持久化，不自动读取数据库或刷新。
func NewShareSelectionService(store ShareSelectionStore) *ShareSelectionService {
	return &ShareSelectionService{store: store}
}

// List 校验页码与搜索范围，查询无发现副作用。
func (s *ShareSelectionService) List(ctx context.Context, q domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 1000000 || q.PageSize < 1 || q.PageSize > 100 || len(q.Keyword) > 300 {
		return nil, 0, fmt.Errorf("分页或搜索参数无效")
	}
	if err := s.store.CheckSelectionSchema(ctx); err != nil {
		return nil, 0, err
	}
	return s.store.ListSelections(ctx, q)
}

// Detail 返回单条季集选择和候选状态。
func (s *ShareSelectionService) Detail(ctx context.Context, key string) (domain.ShareSelectionDetail, error) {
	if strings.TrimSpace(key) == "" || len(key) > 2000 {
		return domain.ShareSelectionDetail{}, fmt.Errorf("媒体键无效")
	}
	return s.store.SelectionDetail(ctx, key)
}

// Change 手选使用显式版本，事务成功后仅通知下次导出生效。
func (s *ShareSelectionService) Change(ctx context.Context, change domain.ShareSelectionChange) error {
	if strings.TrimSpace(change.ItemKey) == "" || len(change.ItemKey) > 2000 || change.CandidateID <= 0 || change.ExpectedRevision <= 0 {
		return fmt.Errorf("改选参数无效")
	}
	err := s.store.ChangeSelection(ctx, change)
	logger.WithContext(ctx, "share_selection").Log(logger.INFO, "来源改选完成", logger.Fields{"candidate_id": change.CandidateID, "expected_revision": change.ExpectedRevision, "next_export": true}, err)
	return err
}

// Refresh 显式从应用库刷新发现，不访问分享网络或文件系统。
func (s *ShareSelectionService) Refresh(ctx context.Context) error {
	if err := s.store.CheckSelectionSchema(ctx); err != nil {
		return err
	}
	err := s.store.SyncSelections(ctx, "")
	logger.WithContext(ctx, "share_selection").Log(logger.INFO, "来源候选显式刷新", logger.Fields{"event": "selection_refresh"}, err)
	return err
}
