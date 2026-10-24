package service

import (
	"context"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"fmt"
	"strings"
	"time"
)

// ShareSelectionStore 定义选择管理所需能力，不包含播放凭据访问。
type ShareSelectionStore interface {
	CheckSelectionSchema(context.Context) error
	ListSelections(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error)
	SelectionDetail(context.Context, string) (domain.ShareSelectionDetail, error)
	ChangeSelection(context.Context, domain.ShareSelectionChange) (string, error)
	SyncSelections(context.Context, string) error
}

// ShareSelectionService 提供只读列表及显式改选、刷新用例。
type ShareSelectionService struct {
	store    ShareSelectionStore
	applyNow func(context.Context, string) (string, error)
}

// SetImmediateExport 注入改选后立即重建能力；未注入时仍排队到下次导出。
func (s *ShareSelectionService) SetImmediateExport(start func(context.Context, string) (string, error)) {
	s.applyNow = start
}

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
	if err := s.store.CheckSelectionSchema(ctx); err != nil {
		return domain.ShareSelectionDetail{}, err
	}
	return s.store.SelectionDetail(ctx, key)
}

// Change 手选使用显式版本，事务成功后在当前进程立即重建该作品的 STRM，
// 不再等待下一次定时或全量导出；重建失败只记录日志，改选本身仍然生效。
func (s *ShareSelectionService) Change(ctx context.Context, change domain.ShareSelectionChange) (string, error) {
	if strings.TrimSpace(change.ItemKey) == "" || len(change.ItemKey) > 2000 || change.ExpectedRevision <= 0 {
		return "", fmt.Errorf("改选参数无效")
	}
	if err := domain.ValidateShareSelectionChange(&change); err != nil {
		return "", err
	}
	if err := s.store.CheckSelectionSchema(ctx); err != nil {
		return "", err
	}
	work, err := s.store.ChangeSelection(ctx, change)
	if err != nil {
		logger.WithContext(ctx, "share_selection").Log(logger.ERROR, "来源改选失败", logger.Fields{"candidate_id": change.CandidateID, "expected_revision": change.ExpectedRevision}, err)
		return "", err
	}
	taskID := ""
	if s.applyNow != nil && work != "" {
		id, startErr := s.applyNow(ctx, work)
		if startErr != nil {
			logger.WithContext(ctx, "share_selection").Log(logger.WARN, "来源改选已保存，立即重建未启动", logger.Fields{"work_key": work}, startErr)
		} else {
			taskID = id
		}
	}
	logger.WithContext(ctx, "share_selection").Log(logger.INFO, "来源改选完成", logger.Fields{"candidate_id": change.CandidateID, "expected_revision": change.ExpectedRevision, "work_key": work, "immediate": taskID != "", "task_id": taskID}, nil)
	return taskID, nil
}

// Apply 对已保存但尚未导出的改选立即重建该作品，供历史待应用条目手动触发。
func (s *ShareSelectionService) Apply(ctx context.Context, itemKey string) (string, error) {
	if strings.TrimSpace(itemKey) == "" || len(itemKey) > 2000 {
		return "", fmt.Errorf("媒体键无效")
	}
	if s.applyNow == nil {
		return "", fmt.Errorf("立即重建未启用")
	}
	if err := s.store.CheckSelectionSchema(ctx); err != nil {
		return "", err
	}
	detail, err := s.store.SelectionDetail(ctx, itemKey)
	if err != nil {
		return "", err
	}
	if detail.Selection.WorkKey == "" {
		return "", fmt.Errorf("作品键缺失")
	}
	taskID, err := s.applyNow(ctx, detail.Selection.WorkKey)
	logger.WithContext(ctx, "share_selection").Log(logger.INFO, "来源改选手动应用", logger.Fields{"media_item_key": itemKey, "work_key": detail.Selection.WorkKey, "task_id": taskID}, err)
	return taskID, err
}

// Refresh 显式从应用库刷新发现，不访问分享网络或文件系统。
func (s *ShareSelectionService) Refresh(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.store.CheckSelectionSchema(ctx); err != nil {
		return err
	}
	err := s.store.SyncSelections(ctx, "")
	logger.WithContext(ctx, "share_selection").Log(logger.INFO, "来源候选显式刷新", logger.Fields{"event": "selection_refresh"}, err)
	return err
}
