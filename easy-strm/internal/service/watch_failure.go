package service

import (
	"fmt"
	"strings"

	"easy-strm/internal/domain"
)

func resolveWatchOrganizeDefaults(source *domain.MediaSource) (string, string, string) {
	mediaType := normalizeMediaType(source.MediaType)
	conflictPolicy := normalizeConflictPolicy(source.ConflictPolicy)
	operationMode := normalizeOperationMode(source.OperationMode)

	if source != nil && source.SourceType == domain.SourceTypeCloud115 {
		switch operationMode {
		case "hardlink", "symlink":
			operationMode = "move"
		}
	}

	return mediaType, conflictPolicy, operationMode
}

func summarizeAutoOrganizeFailure(err error) (string, string) {
	if err == nil {
		return "", ""
	}

	message := err.Error()
	lowerMessage := strings.ToLower(message)

	switch {
	case strings.Contains(lowerMessage, "target") && strings.Contains(lowerMessage, "path"),
		strings.Contains(message, "目标"),
		strings.Contains(message, "路径"):
		return "target_path", message
	case strings.Contains(lowerMessage, "scan") || strings.Contains(lowerMessage, "list") || strings.Contains(message, "扫描"):
		return "scan_failed", message
	case strings.Contains(lowerMessage, "identify") || strings.Contains(lowerMessage, "tmdb") || strings.Contains(lowerMessage, "recogniz") || strings.Contains(message, "识别"):
		return "identify_failed", message
	case isCloud115AuthFailureMessage(message):
		return "cloud115_auth_failed", "115 账号 Cookie 已失效，请重新登录"
	case strings.Contains(lowerMessage, "115") || strings.Contains(lowerMessage, "cloud"):
		return "cloud115_failed", message
	default:
		return "organize_failed", message
	}
}

func classifyWatchFailureCategory(reason string) string {
	lowerReason := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case lowerReason == "":
		return "other"
	case strings.Contains(lowerReason, "identify") || strings.Contains(lowerReason, "tmdb") || strings.Contains(lowerReason, "recogniz") || strings.Contains(reason, "识别"):
		return "identify_failed"
	case strings.Contains(lowerReason, "cookie") || strings.Contains(lowerReason, "auth") || strings.Contains(lowerReason, "unauthorized") || strings.Contains(lowerReason, "token"):
		return "cloud115_auth_failed"
	case strings.Contains(lowerReason, "cloud115") || strings.Contains(lowerReason, "115"):
		return "cloud115_failed"
	case strings.Contains(lowerReason, "scan") || strings.Contains(lowerReason, "list") || strings.Contains(reason, "扫描"):
		return "scan_failed"
	case strings.Contains(lowerReason, "target") && strings.Contains(lowerReason, "path"),
		(strings.Contains(reason, "目标") && strings.Contains(reason, "路径")):
		return "target_path"
	case strings.Contains(lowerReason, "panic") || strings.Contains(lowerReason, "exception"):
		return "panic"
	case strings.Contains(lowerReason, "organize") || strings.Contains(lowerReason, "move") || strings.Contains(lowerReason, "copy") || strings.Contains(reason, "整理"):
		return "organize_failed"
	default:
		return "other"
	}
}

func summarizeAutoOrganizeResults(results []OrganizeResult) (string, string) {
	if len(results) == 0 {
		return "", ""
	}

	identifyFailures := 0
	conflictFailures := 0
	moveFailures := 0
	var firstMessage string

	for _, result := range results {
		if result.Success {
			continue
		}
		if firstMessage == "" {
			firstMessage = result.Message
		}

		lowerMessage := strings.ToLower(result.Message)
		switch {
		case strings.Contains(lowerMessage, "identify") || strings.Contains(lowerMessage, "tmdb") || strings.Contains(lowerMessage, "recogniz") || strings.Contains(result.Message, "识别"):
			identifyFailures++
		case isCloud115AuthFailureMessage(result.Message):
			return "cloud115_auth_failed", "115 账号 Cookie 已失效，请重新登录"
		case strings.Contains(lowerMessage, "conflict") || strings.Contains(lowerMessage, "exists") || strings.Contains(result.Message, "冲突"):
			conflictFailures++
		default:
			moveFailures++
		}
	}

	switch {
	case identifyFailures > 0 && conflictFailures == 0 && moveFailures == 0:
		return "identify_failed", fmt.Sprintf("识别失败 %d 项：%s", identifyFailures, firstMessage)
	case conflictFailures > 0 && identifyFailures == 0 && moveFailures == 0:
		return "conflict_skipped", fmt.Sprintf("冲突跳过 %d 项：%s", conflictFailures, firstMessage)
	case isCloud115AuthFailureMessage(firstMessage):
		return "cloud115_auth_failed", "115 账号 Cookie 已失效，请重新登录"
	case moveFailures > 0 && identifyFailures == 0 && conflictFailures == 0:
		return "organize_failed", fmt.Sprintf("整理失败 %d 项：%s", moveFailures, firstMessage)
	case identifyFailures > 0 || conflictFailures > 0 || moveFailures > 0:
		return "partial_failed", fmt.Sprintf("识别失败 %d 项，冲突跳过 %d 项，整理失败 %d 项", identifyFailures, conflictFailures, moveFailures)
	default:
		return "", ""
	}
}

func buildWatchFailureItems(results []OrganizeResult) []watchFailureItem {
	items := make([]watchFailureItem, 0)
	for _, result := range results {
		if result.Success {
			continue
		}
		reason := result.Message
		if reason == "" {
			reason = "整理失败"
		}
		items = append(items, watchFailureItem{
			FileID:   result.FileID,
			FileName: result.FileName,
			Category: classifyWatchFailureCategory(reason),
			Reason:   reason,
		})
	}
	return items
}

func buildWatchFailureItemsFromIDs(fileIDs []string, reason, category string) []watchFailureItem {
	if category == "" {
		category = classifyWatchFailureCategory(reason)
	}
	items := make([]watchFailureItem, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if fileID == "" {
			continue
		}
		items = append(items, watchFailureItem{
			FileID:   fileID,
			FileName: fileID,
			Category: category,
			Reason:   reason,
		})
	}
	return items
}

func parseRetryFileIDs(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		fileIDs := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				fileIDs = append(fileIDs, text)
			}
		}
		return fileIDs
	default:
		return nil
	}
}

func parseRetrySourceID(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func isCloud115AuthFailureMessage(message string) bool {
	lowerMessage := strings.ToLower(strings.TrimSpace(message))
	if lowerMessage == "" {
		return false
	}

	switch {
	case strings.Contains(lowerMessage, "parse cookie failed"),
		strings.Contains(lowerMessage, "cookie"),
		strings.Contains(lowerMessage, "unauthorized"),
		strings.Contains(lowerMessage, "authentication"),
		strings.Contains(lowerMessage, "token expired"),
		strings.Contains(lowerMessage, "session expired"),
		strings.Contains(message, "登录失效"),
		strings.Contains(message, "cookie失效"),
		strings.Contains(message, "cookie无效"),
		strings.Contains(message, "115账号失效"),
		strings.Contains(message, "账号失效"),
		strings.Contains(message, "请重新登录"),
		strings.Contains(message, "账号不存在"):
		return true
	default:
		return false
	}
}
