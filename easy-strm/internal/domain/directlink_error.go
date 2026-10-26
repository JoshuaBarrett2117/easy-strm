package domain

import "fmt"

// DirectLinkError 保留实际上游状态及错误链；错误文本不包含 SDK 响应体或签名链接。
type DirectLinkError struct {
	Errno      int
	HTTPStatus int
	Cause      error
}

// Error 返回可安全展示的状态元数据，零表示上游未提供。
func (err *DirectLinkError) Error() string {
	return fmt.Sprintf("115直链请求失败 (errno=%d, http_status=%d)", err.Errno, err.HTTPStatus)
}

// Unwrap 保留 SDK 类型和网络取消语义，供 errors.Is/As 使用。
func (err *DirectLinkError) Unwrap() error { return err.Cause }
