package domain

import "time"

const (
	FileManagerLocationLocal    = "local"
	FileManagerLocationCloud115 = "cloud115"
	FileManagerOperationCopy    = "copy"
	FileManagerOperationMove    = "move"
)

// FileManagerLocation 表示文件管理器中可选择的存储位置。
type FileManagerLocation struct {
	Type   string `json:"type"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Root   string `json:"root"`
	Status string `json:"status"`
}

// FileManagerEntry 表示统一文件列表中的一个文件或目录。
type FileManagerEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	ParentPath  string    `json:"parent_path"`
	IsDirectory bool      `json:"is_directory"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
	PickCode    string    `json:"pick_code,omitempty"`
	SHA1        string    `json:"sha1,omitempty"`
}

// FileManagerBrowseResult 表示文件管理器目录浏览结果。
type FileManagerBrowseResult struct {
	Path    string             `json:"path"`
	Entries []FileManagerEntry `json:"entries"`
	Total   int                `json:"total"`
}

// FileManagerLocationRef 唯一定位一个本地媒体源或115账号。
type FileManagerLocationRef struct {
	Type string `json:"type" binding:"required"`
	ID   int    `json:"id" binding:"required"`
}

// FileManagerTransferItem 表示剪贴板中的一个待传输项。
type FileManagerTransferItem struct {
	ID          string `json:"id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"is_directory"`
	PickCode    string `json:"pick_code"`
}

// FileManagerTransferRequest 表示复制或剪切粘贴请求。
type FileManagerTransferRequest struct {
	Operation  string                    `json:"operation" binding:"required"`
	Source     FileManagerLocationRef    `json:"source" binding:"required"`
	Target     FileManagerLocationRef    `json:"target" binding:"required"`
	TargetPath string                    `json:"target_path"`
	Items      []FileManagerTransferItem `json:"items" binding:"required,min=1"`
}

// FileManagerTransferResponse 表示异步传输任务创建结果。
type FileManagerTransferResponse struct {
	TaskID string `json:"task_id"`
	Total  int    `json:"total"`
}

// FileManagerDeleteRequest 表示批量删除请求。
type FileManagerDeleteRequest struct {
	Location FileManagerLocationRef    `json:"location" binding:"required"`
	Items    []FileManagerTransferItem `json:"items" binding:"required,min=1"`
}
