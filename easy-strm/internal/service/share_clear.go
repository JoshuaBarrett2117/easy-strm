package service

import (
	"context"
	"fmt"
)

// ClearMedia 清空分享的全部媒体候选及识别结果，保留分享配置，供后续重新扫描。
func (s *ShareRecordService) ClearMedia(ctx context.Context, id int) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("分享ID无效")
	}
	if !s.identifyMu.TryLock() {
		return 0, fmt.Errorf("有分享识别任务正在运行，请等待任务结束后再清空")
	}
	defer s.identifyMu.Unlock()
	return s.dao.ClearMedia(ctx, id)
}

// ClearAllMedia 清空全部分享的媒体候选及识别结果，保留所有分享配置。
func (s *ShareRecordService) ClearAllMedia(ctx context.Context) (int64, error) {
	if !s.identifyMu.TryLock() {
		return 0, fmt.Errorf("有分享识别任务正在运行，请等待任务结束后再清空")
	}
	defer s.identifyMu.Unlock()
	return s.dao.ClearAllMedia(ctx)
}
