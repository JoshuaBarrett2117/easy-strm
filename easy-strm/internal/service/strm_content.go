package service

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildCloud115StrmContent 统一现有生成器和行为同步的播放地址格式，优先使用稳定SHA1。
func BuildCloud115StrmContent(serverURL, cloudPath, sha1 string, account int) string {
	if sha1 != "" {
		cloudPath = sha1
	}
	content := fmt.Sprintf("%s/direct-link?path=%s", strings.TrimRight(serverURL, "/"), url.QueryEscape(strings.ReplaceAll(cloudPath, "\\", "/")))
	if account > 0 {
		content += fmt.Sprintf("&cloud115_id=%d", account)
	}
	return content + "\n"
}
