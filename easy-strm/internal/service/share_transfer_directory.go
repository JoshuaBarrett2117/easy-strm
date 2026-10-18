package service

import (
	"fmt"
	"strings"
	"unicode"
)

// ShareTransferDirectoryKey 是分享转存默认目录的系统配置键。
const ShareTransferDirectoryKey = "share_transfer_dir"

// NormalizeShareTransferDirectory 校验绝对网盘路径，空值使用默认目录。
func NormalizeShareTransferDirectory(value string) (string, error) {
	if strings.Contains(value, "\\") || strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("目录不能包含控制字符或反斜杠")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "/EmbyCache", nil
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("目录必须为以 / 开头的绝对路径")
	}
	parts := []string{}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "." {
			return "", fmt.Errorf("目录不能包含 . 或 .. 路径段")
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	return "/" + strings.Join(parts, "/"), nil
}

// ResolveShareTransferDirectory 在执行时读取最新默认值，不修改持久化的显式配置。
func ResolveShareTransferDirectory(settings ShareTaskSettingsStore, explicit, account string) (string, error) {
	for _, value := range []string{explicit, account} {
		if strings.TrimSpace(value) != "" {
			return NormalizeShareTransferDirectory(value)
		}
	}
	if settings != nil {
		row, err := settings.GetByKey(ShareTransferDirectoryKey)
		if err != nil {
			return "", err
		}
		if row != nil {
			return NormalizeShareTransferDirectory(row.ConfigVal)
		}
	}
	return NormalizeShareTransferDirectory("")
}

// NormalizeShareTransferSettings 在任何配置写入前校验并归一化默认转存目录。
func NormalizeShareTransferSettings(values map[string]string) error {
	if value, ok := values[ShareTransferDirectoryKey]; ok {
		normalized, err := NormalizeShareTransferDirectory(value)
		if err != nil {
			return err
		}
		values[ShareTransferDirectoryKey] = normalized
	}
	return nil
}
