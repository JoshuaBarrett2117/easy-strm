package domain

import "fmt"

// ShareReceiveError 保留 115 分享转存的原始错误码和消息，供上层通过 errors.As 识别。
type ShareReceiveError struct {
	Errno   int
	Message string
}

// Error 返回可追踪的上游错误；展示给用户前仍需脱敏和转义。
func (e *ShareReceiveError) Error() string {
	return fmt.Sprintf("share receive failed (errno=%d): %s", e.Errno, e.Message)
}
