package service

import (
	"fmt"
	"strconv"
)

// taskDisplayStatus 统一通知和任务中心的展示结果，保留原始状态用于调度、取消和重试。
func taskDisplayStatus(task map[string]interface{}) string {
	status := fmt.Sprint(task["status"])
	if status == "partial_failed" || status == "partial_success" {
		return "partial_success"
	}
	if (status == "failed" || status == "completed" || status == "success") && numericTaskCount(task["success_files"]) > 0 && numericTaskCount(task["failed_files"]) > 0 {
		return "partial_success"
	}
	return status
}

func numericTaskCount(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		n, _ := strconv.Atoi(fmt.Sprint(value))
		return n
	}
}
