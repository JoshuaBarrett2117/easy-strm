package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"strings"
)

// ErrShareCancelled 表示网盘明确返回分享已取消，与任务的 context 取消独立。
var ErrShareCancelled = errors.New("分享已取消")

func isShareCancelledError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrShareCancelled) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, phrase := range []string{"分享已取消", "分享已被取消", "分享已经取消", "share has been cancelled", "share has been canceled", "share cancelled", "share canceled", "share revoked"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

// parseRecordShare 将网盘状态持久化；网络、密码及任务错误不改变分享状态。
func (s *ShareRecordService) parseRecordShare(ctx context.Context, record domain.ShareRecord) (*domain.ParseShareResponse, error) {
	parsed, err := s.parser.ParseShareLink(ctx, record.URL, record.Password)
	cancelled := isShareCancelledError(err)
	if (err == nil || cancelled) && record.ShareCancelled != cancelled {
		if saveErr := s.dao.SetShareCancelled(ctx, record, cancelled); saveErr != nil {
			return nil, fmt.Errorf("保存分享取消状态失败: %w", saveErr)
		}
	}
	return parsed, err
}
