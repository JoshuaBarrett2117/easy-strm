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
