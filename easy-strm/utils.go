package main

import (
	"net/url"
	"strings"
)

// EncodePath 编码网络磁盘路径，处理特殊字符和非ASCII字符
func EncodePath(path string) string {
	// 首先进行URL编码
	encoded := url.QueryEscape(path)

	// 处理一些特殊情况
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "%2F", "/")

	Debug("Encoded path: %s -> %s", path, encoded)
	return encoded
}

// DecodePath 解码网络磁盘路径
func DecodePath(encodedPath string) (string, error) {
	decoded, err := url.QueryUnescape(encodedPath)
	if err != nil {
		Error("Failed to decode path: %v", err)
		return "", err
	}

	Debug("Decoded path: %s -> %s", encodedPath, decoded)
	return decoded, nil
}

// ValidateEncodedPath 验证编码后的路径是否有效
func ValidateEncodedPath(encodedPath string) bool {
	// 尝试解码路径
	_, err := DecodePath(encodedPath)
	if err != nil {
		return false
	}

	// 检查是否包含无效字符
	invalidChars := []string{"%00", "%01", "%02", "%03", "%04", "%05", "%06", "%07", "%08", "%09", "%0A", "%0B", "%0C", "%0D", "%0E", "%0F"}
	for _, char := range invalidChars {
		if strings.Contains(encodedPath, char) {
			return false
		}
	}

	return true
}

// BuildApiUrl 构建API URL，处理参数编码
func BuildApiUrl(baseUrl string, endpoint string, params map[string]string) string {
	// 构建基础URL
	fullUrl := baseUrl
	if !strings.HasSuffix(fullUrl, "/") {
		fullUrl += "/"
	}
	fullUrl += endpoint

	// 添加参数
	if len(params) > 0 {
		fullUrl += "?"
		first := true
		for key, value := range params {
			if !first {
				fullUrl += "&"
			}
			fullUrl += url.QueryEscape(key) + "=" + url.QueryEscape(value)
			first = false
		}
	}

	Debug("Built API URL: %s", fullUrl)
	return fullUrl
}

// SanitizeFilename 清理文件名，移除或替换无效字符
func SanitizeFilename(filename string) string {
	// 替换Windows系统中的无效字符
	invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*"}
	for _, char := range invalidChars {
		filename = strings.ReplaceAll(filename, char, "_")
	}

	// 移除控制字符
	filename = strings.Map(func(r rune) rune {
		if r < 32 {
			return '_'
		}
		return r
	}, filename)

	// 限制文件名长度
	if len(filename) > 255 {
		filename = filename[:255]
	}

	Debug("Sanitized filename: %s", filename)
	return filename
}

// GetFileExtension 获取文件扩展名，包括点
func GetFileExtension(filename string) string {
	ext := ""
	lastDot := strings.LastIndex(filename, ".")
	if lastDot != -1 && lastDot < len(filename)-1 {
		ext = filename[lastDot:]
	}
	return strings.ToLower(ext)
}
