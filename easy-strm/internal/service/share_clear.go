package service

import (
	"context"
	"fmt"
)

// ClearMedia 同步清空指定分享；HTTP入口使用持久化队列。
func (s *ShareRecordService) ClearMedia(ctx context.Context, id int) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("分享ID无效")
	}
	release, err := s.Coordinator().try(shareResource{key: shareKey(id), exclusive: true})
	if err != nil {
		return 0, err
	}
	defer release()
	count, err := s.dao.ClearMedia(ctx, id)
	if err == nil {
		s.Coordinator().invalidate([]int{id})
	}
	return count, err
}

// ClearAllMedia 固定调用时的分享集合，不使用全局清空锁。
func (s *ShareRecordService) ClearAllMedia(ctx context.Context) (int64, error) {
	ids, err := s.dao.ShareIDs(ctx)
	if err != nil {
		return 0, err
	}
	return s.ClearSelectedMedia(ctx, ids)
}

// ClearSelectedMedia 原子清空选中分享，同步调用冲突时返回错误。
func (s *ShareRecordService) ClearSelectedMedia(ctx context.Context, ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	resources := []shareResource{}
	for _, id := range ids {
		if id <= 0 {
			return 0, fmt.Errorf("分享ID无效")
		}
		resources = append(resources, shareResource{key: shareKey(id), exclusive: true})
	}
	release, err := s.Coordinator().try(resources...)
	if err != nil {
		return 0, err
	}
	defer release()
	count, err := s.dao.ClearSelectedMedia(ctx, ids)
	if err == nil {
		s.Coordinator().invalidate(ids)
	}
	return count, err
}
