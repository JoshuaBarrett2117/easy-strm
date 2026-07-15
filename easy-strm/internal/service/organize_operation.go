package service

import "strings"

const (
	organizeOperationMove     = "move"
	organizeOperationCopy     = "copy"
	organizeOperationHardLink = "hardlink"
	organizeOperationSymLink  = "symlink"
)

func normalizeOrganizeOperationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", organizeOperationMove:
		return organizeOperationMove
	case organizeOperationCopy:
		return organizeOperationCopy
	case organizeOperationHardLink:
		return organizeOperationHardLink
	case organizeOperationSymLink:
		return organizeOperationSymLink
	default:
		return ""
	}
}

func organizeOperationSuccessMessage(mode string) string {
	switch normalizeOrganizeOperationMode(mode) {
	case organizeOperationCopy:
		return "整理并复制成功"
	case organizeOperationHardLink:
		return "整理并创建硬链接成功"
	case organizeOperationSymLink:
		return "整理并创建软链接成功"
	default:
		return "整理并移动成功"
	}
}

func organizeOperationActionName(mode string) string {
	switch normalizeOrganizeOperationMode(mode) {
	case organizeOperationCopy:
		return "复制"
	case organizeOperationHardLink:
		return "创建硬链接"
	case organizeOperationSymLink:
		return "创建软链接"
	default:
		return "移动"
	}
}
