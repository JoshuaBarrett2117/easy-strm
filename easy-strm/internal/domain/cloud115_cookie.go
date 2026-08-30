package domain

import (
	"strconv"
	"strings"
)

// Cloud115CookieIdentity 表示从 115 Cookie UID 中解析出的账号与登录端信息。
type Cloud115CookieIdentity struct {
	UserID    string
	SSOEnt    string
	Timestamp int64
	Source    string
}

var cloud115SSOEntSources = map[string]string{
	"A1": "网页版 / 115 浏览器",
	"A2": "未知安卓端",
	"A3": "未知 iOS 端",
	"A4": "未知 iPad 端",
	"B1": "未知安卓端",
	"D1": "115 生活 iOS 端",
	"D2": "iOS 端",
	"D3": "115 iOS 端",
	"F1": "115 生活安卓端",
	"F2": "安卓端",
	"F3": "115 安卓端",
	"H1": "115 生活 iPad 端",
	"H2": "iPad 端",
	"H3": "115 iPad 端",
	"I1": "安卓电视端",
	"I2": "Apple TV 端",
	"M1": "115 管理安卓端",
	"N1": "115 管理 iOS 端",
	"O1": "115 管理 iPad 端",
	"P1": "Windows 端",
	"P2": "macOS 端",
	"P3": "Linux 端",
	"R1": "微信小程序",
	"R2": "支付宝小程序",
	"S1": "鸿蒙端",
}

var cloud115AppSources = map[string]string{
	"web":        "网页版 / 115 浏览器",
	"desktop":    "网页版 / 115 浏览器",
	"android":    "115 生活安卓端",
	"115android": "115 安卓端",
	"ios":        "115 生活 iOS 端",
	"115ios":     "115 iOS 端",
	"ipad":       "115 生活 iPad 端",
	"115ipad":    "115 iPad 端",
	"tv":         "安卓电视端",
	"apple_tv":   "Apple TV 端",
	"qandroid":   "115 管理安卓端",
	"qios":       "115 管理 iOS 端",
	"qipad":      "115 管理 iPad 端",
	"os_windows": "Windows 端",
	"os_mac":     "macOS 端",
	"os_linux":   "Linux 端",
	"wechatmini": "微信小程序",
	"alipaymini": "支付宝小程序",
	"harmony":    "鸿蒙端",
}

// ParseCloud115CookieIdentity 从 UID=<用户ID>_<设备码>_<时间戳> 中解析 Cookie 来源。
func ParseCloud115CookieIdentity(cookie string) (Cloud115CookieIdentity, bool) {
	uid := cloud115CookieValue(cookie, "UID")
	parts := strings.Split(uid, "_")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return Cloud115CookieIdentity{}, false
	}
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		return Cloud115CookieIdentity{}, false
	}
	timestamp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return Cloud115CookieIdentity{}, false
	}

	ssoent := strings.ToUpper(parts[1])
	source := cloud115SSOEntSources[ssoent]
	if source == "" {
		source = "未知渠道"
	}
	return Cloud115CookieIdentity{
		UserID:    parts[0],
		SSOEnt:    ssoent,
		Timestamp: timestamp,
		Source:    source,
	}, true
}

// Cloud115CookieSourceByApp 返回扫码登录渠道对应的可读来源。
func Cloud115CookieSourceByApp(app string) string {
	if source := cloud115AppSources[strings.ToLower(strings.TrimSpace(app))]; source != "" {
		return source
	}
	return "网页版 / 115 浏览器"
}

func cloud115CookieValue(cookie, name string) string {
	for _, item := range strings.Split(cookie, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(item), "=")
		if found && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}
