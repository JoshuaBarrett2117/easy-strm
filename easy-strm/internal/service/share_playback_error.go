package service

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

var sharePlaybackErrno = regexp.MustCompile(`\berrno\s*[=:]\s*([0-9]+)\b`)

// SharePlaybackErrorInfo 为播放错误页和显式 JSON 客户端提供统一、已脱敏的诊断建议。
type SharePlaybackErrorInfo struct {
	Category string
	Advice   string
	Detail   string
	Errno    string
}

// DescribeSharePlaybackError 仅按已确认的错误码或明确消息归类，不推断未知转存码的含义。
func DescribeSharePlaybackError(err error) SharePlaybackErrorInfo {
	info := SharePlaybackErrorInfo{Category: "原因未确认", Advice: "请稍后重试；若持续失败，请在 115 中检查该分享是否可访问、能否手动转存，并将错误码提供给管理员。"}
	if err == nil {
		return info
	}
	info.Detail = logger.SanitizeText(err.Error())
	message := strings.ToLower(err.Error())
	var receive *domain.ShareReceiveError
	if errors.As(err, &receive) {
		info.Errno = strconv.Itoa(receive.Errno)
		// 有类型错误时仅使用上游消息，避免包装层的上下文误触发分类。
		message = strings.ToLower(receive.Message)
	} else if match := sharePlaybackErrno.FindStringSubmatch(err.Error()); len(match) > 1 {
		info.Errno = match[1]
	}
	switch {
	// 115driver v1.3.5/pkg/driver/error.go 明确映射这两个分享错误码。
	// elevengo 的 ShareCanceled3=4100018 未提供当前 receive 场景的判定证据，故不在此映射。
	case info.Errno == "4100009" || info.Errno == "4100026" || strings.Contains(message, "分享已过期") || strings.Contains(message, "分享已取消") || strings.Contains(message, "分享不存在"):
		info.Category, info.Advice = "分享不可用", "请在 115 中确认分享有效，更新分享来源后重新导出 STRM。"
	case strings.Contains(message, "提取码错误") || strings.Contains(message, "访问密码错误") || strings.Contains(message, "需要访问密码"):
		info.Category, info.Advice = "分享密码问题", "请核对分享提取码并更新来源后重试。"
	case strings.Contains(message, "未登录") || strings.Contains(message, "登录失效") || strings.Contains(message, "登录已过期"):
		info.Category, info.Advice = "账号登录失效", "请由管理员检查转存账号的登录状态后重试。"
	case strings.Contains(message, "空间不足") || strings.Contains(message, "容量不足"):
		info.Category, info.Advice = "存储空间不足", "请检查转存账号的可用空间后重试。"
	case sharePlaybackMessageContains(message, "文件已删除", "文件已被删除", "文件被删除", "文件不存在", "file deleted", "file has been deleted", "file not found", "file does not exist"):
		info.Category, info.Advice = "文件已删除或不存在", "请在 115 中确认文件仍可访问，更新分享来源后重新导出 STRM。"
	case sharePlaybackMessageContains(message, "转存额度不足", "转存额度超限", "转存配额不足", "转存配额超限", "转存额度已用完", "转存次数已用完", "转存次数超限", "权限不足", "无权访问", "transfer quota exceeded", "permission denied", "insufficient permission"):
		info.Category, info.Advice = "转存额度或权限不足", "请检查转存账号的转存额度和访问权限，恢复额度或取得授权后重试。"
	case sharePlaybackMessageContains(message, "请求过于频繁", "操作过于频繁", "触发限流", "服务器繁忙", "服务暂时不可用", "rate limit exceeded", "rate limited", "too many requests", "server busy", "server is busy", "temporarily unavailable"):
		info.Category, info.Advice = "上游暂时不可用", "请稍后重试，避免连续发起转存请求；若持续失败，请检查 115 服务状态。"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(message, "timeout"):
		info.Category, info.Advice = "上游请求超时", "请检查服务到 115 的网络连接，稍后重试。"
	}
	return info
}

func sharePlaybackMessageContains(message string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}
