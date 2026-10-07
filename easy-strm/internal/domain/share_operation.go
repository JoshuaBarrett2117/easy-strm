package domain

import "time"

// ShareOperation 记录用户确认的清理目标和执行进度，数据库是恢复时的事实来源。
type ShareOperation struct {
	Sequence  int64                  `json:"sequence"`
	TaskID    string                 `json:"task_id"`
	Kind      string                 `json:"operation"`
	Key       string                 `json:"request_key"`
	ShareIDs  []int                  `json:"share_ids"`
	FileID    int                    `json:"file_id,omitempty"`
	Status    string                 `json:"status"`
	Phase     string                 `json:"phase"`
	Result    map[string]interface{} `json:"result"`
	Error     string                 `json:"error"`
	CreatedAt time.Time              `json:"created_at"`
}

// ShareOperationAccepted 表示操作已持久化；完成结果须通过任务ID查询。
type ShareOperationAccepted struct {
	TaskID string `json:"task_id"`
}

// IsShareOperationType 判断任务是否由持久化分享操作队列负责恢复。
func IsShareOperationType(kind string) bool {
	return kind == "share_delete" || kind == "share_clear" || kind == "share_media_delete"
}
