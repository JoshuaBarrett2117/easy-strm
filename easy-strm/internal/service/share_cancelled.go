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

// ErrShareUnavailable 表示网盘明确返回分享不存在、链接无效或过期。
var ErrShareUnavailable = errors.New("分享不存在或已过期")

func isShareUnavailableError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, ErrShareUnavailable) || isShareCancelledError(err)
}

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
	// 沿用历史字段兼容已有数据和客户端，统一保存明确失效状态。
	cancelled := isShareUnavailableError(err)
	if (err == nil || cancelled) && record.ShareCancelled != cancelled {
		if saveErr := s.dao.SetShareCancelled(ctx, record, cancelled); saveErr != nil {
			return nil, fmt.Errorf("保存分享失效状态失败: %w", saveErr)
		}
	}
	return parsed, err
}
