package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

func TestDescribeSharePlaybackError(t *testing.T) {
	for _, tc := range []struct {
		errno             int
		message, category string
	}{
		{4100018, "", "原因未确认"},
		{999999, "未知上游消息", "原因未确认"},
		{4100009, "", "分享不可用"},
		{4100026, "", "分享不可用"},
		{999999, "分享已过期", "分享不可用"},
		{4100018, "提取码错误", "分享密码问题"},
		{123, "登录已过期", "账号登录失效"},
		{123, "空间不足", "存储空间不足"},
	} {
		err := fmt.Errorf("115分享转存失败: %w", &domain.ShareReceiveError{Errno: tc.errno, Message: tc.message})
		info := DescribeSharePlaybackError(err)
		if info.Category != tc.category || info.Errno != fmt.Sprint(tc.errno) || !strings.Contains(info.Detail, tc.message) || info.Advice == "" {
			t.Errorf("%v -> %+v", err, info)
		}
	}
	if info := DescribeSharePlaybackError(fmt.Errorf("request: %w", context.DeadlineExceeded)); info.Category != "上游请求超时" {
		t.Errorf("wrapped timeout: %+v", info)
	}
	if info := DescribeSharePlaybackError(errors.New(`token=private https://example.test/?password=secret <script>x</script>`)); strings.Contains(info.Detail, "private") || strings.Contains(info.Detail, "secret") {
		t.Errorf("unsafe detail: %+v", info)
	}
}

func TestSharePlaybackPreservesTypedReceiveError(t *testing.T) {
	s, _, client := strmFixture(t)
	want := &domain.ShareReceiveError{Errno: 4100018, Message: "原始消息"}
	client.receiveErr = want
	link, err := s.Playback(context.Background(), "entry", "Player/1")
	var received *domain.ShareReceiveError
	if link != "" || !errors.As(err, &received) || received != want {
		t.Fatalf("lost wrapped receive error: %q %v", link, err)
	}
}

func TestDescribeSharePlaybackErrorExplicitMessages(t *testing.T) {
	for _, tc := range []struct {
		category, action string
		messages         []string
	}{
		{"文件已删除或不存在", "更新分享来源", []string{"文件已删除", "文件已被删除", "文件被删除", "文件不存在", "File deleted", "File has been deleted", "FILE NOT FOUND", "file does not exist"}},
		{"转存额度或权限不足", "转存额度和访问权限", []string{"转存额度不足", "转存额度超限", "转存配额不足", "转存配额超限", "转存额度已用完", "转存次数已用完", "转存次数超限", "权限不足", "无权访问", "Transfer quota exceeded", "Permission denied", "insufficient permission", "insufficient permissions"}},
		{"上游暂时不可用", "稍后重试", []string{"请求过于频繁", "操作过于频繁", "触发限流", "服务器繁忙", "服务暂时不可用", "Rate limit exceeded", "Rate limited", "Too many requests", "Server busy", "Server is busy", "Temporarily unavailable"}},
		{"原因未确认", "错误码", []string{"", "not found", "deleted", "quota", "permission", "busy", "temporarily", "unknown upstream failure"}},
	} {
		for _, message := range tc.messages {
			for _, errno := range []int{4100018, 999999} {
				t.Run(fmt.Sprintf("%d/%s", errno, message), func(t *testing.T) {
					err := fmt.Errorf("playback: %w", fmt.Errorf("receive: %w", &domain.ShareReceiveError{Errno: errno, Message: message}))
					info := DescribeSharePlaybackError(err)
					if info.Category != tc.category || info.Errno != fmt.Sprint(errno) || !strings.Contains(info.Advice, tc.action) {
						t.Fatalf("%v -> %+v", err, info)
					}
				})
			}
		}
	}
	// 包装层的描述不应覆盖有类型错误中的实际上游消息。
	err := fmt.Errorf("file not found while handling: %w", &domain.ShareReceiveError{Errno: 4100018})
	if info := DescribeSharePlaybackError(err); info.Category != "原因未确认" {
		t.Fatalf("classified wrapper instead of upstream message: %+v", info)
	}
}
