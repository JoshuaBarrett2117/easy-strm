package service

import (
	driver "github.com/SheltonZhu/115driver/pkg/driver"
)

// Cloud115Client 115客户端接口
// 包含批量整理与常用文件操作所必需的 API
type Cloud115Client interface {
	GetFileList(cid int, showDir int, offset int, limit int, cloud115ID int, cookie string) (*driver.FileListResp, error)
	GetPickCodeByPath(filePath string, cloud115ID int, cookie string) (string, error)
	GetCIDByPath(path string, cloud115ID int, cookie string) (string, error)
	RenameFile(fileID, newName string, cloud115ID int, cookie string) error
	CopyFile(fileID, targetDirID string, cloud115ID int, cookie string) error
	MoveFile115(fileID, targetDirID string, cloud115ID int, cookie string) error
	MkdirAll115(path string, cloud115ID int, cookie string) (string, error)
}

// Cloud115LifeEventClient 表示支持 115 生活事件流的客户端能力。
// 事件流用于增量同步的快速判断，目录扫描仍作为首次同步、异常和周期对账兜底。
type Cloud115LifeEventClient interface {
	GetLifeEvents(offset int, limit int, behaviorType string, date string, cloud115ID int, cookie string) (*Cloud115LifeEventResp, error)
}

type Cloud115LifeEventResp struct {
	Count    int                    `json:"count"`
	NextPage bool                   `json:"next_page"`
	Events   []Cloud115LifeEvent    `json:"events"`
	Raw      map[string]interface{} `json:"raw,omitempty"`
}

type Cloud115LifeEvent struct {
	ID          int64                  `json:"id"`
	UpdateTime  int64                  `json:"update_time"`
	Type        int                    `json:"type"`
	EventName   string                 `json:"event_name"`
	FileID      string                 `json:"file_id"`
	PickCode    string                 `json:"pick_code"`
	ParentID    string                 `json:"parent_id"`
	FileName    string                 `json:"file_name"`
	IsDirectory bool                   `json:"is_directory"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}
