package main

import (
	"fmt"
	"strings"
	"time"

	"easy-strm/internal/domain"
)

// cloud115CookieSourceLabel 将扫码登录渠道转换为面向用户的 Cookie 来源文案。
func cloud115CookieSourceLabel(app string) string {
	return domain.Cloud115CookieSourceByApp(app)
}

// saveCloud115CookieLogin 保存扫码取得的 Cookie；指定账号时同步更新原账号及来源。
func saveCloud115CookieLogin(name string, cloudID int, cookie, cookieSource string) (*Cloud115, error) {
	if cloudID <= 0 {
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("115账号_%d", time.Now().Unix())
		}
		return CreateCloud115(name, cookie, cookieSource, "", "", 0, 0, "", "resource", 5, "115driver", "", "")
	}

	account, err := GetCloud115ByID(cloudID)
	if err != nil {
		return nil, err
	}
	return UpdateCloud115(
		account.ID,
		account.Name,
		cookie,
		cookieSource,
		account.RefreshToken,
		account.AccessToken,
		account.ExpiresIn,
		account.TransferAccountID,
		account.TransferDirectory,
		account.AccountType,
		account.Priority,
		account.Status,
		account.TransferMethod,
		account.AlistUrl,
		account.AlistToken,
	)
}
