package domain

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// BehaviorID 按十进制整数解析115事件ID，禁止经过浮点数。
type BehaviorID int64

// UnmarshalJSON 同时接受字符串和整数形式的ID，拒绝溢出及非整数。
func (id *BehaviorID) UnmarshalJSON(raw []byte) error {
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
	} else {
		text = string(bytes.TrimSpace(raw))
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err == nil {
		*id = BehaviorID(value)
	}
	return err
}

// BehaviorEvent 保留行为接口的文件身份和事件发生时的父目录。
type BehaviorEvent struct {
	ID       BehaviorID `json:"id"`
	Type     int        `json:"type"`
	FileID   string     `json:"file_id"`
	ParentID string     `json:"parent_id"`
	Name     string     `json:"file_name"`
	Size     int64      `json:"file_size"`
	SHA1     string     `json:"sha1"`
}

// BehaviorPage 是降序行为分页，终止条件以NextPage为准。
type BehaviorPage struct {
	State    bool            `json:"state"`
	Count    int             `json:"count"`
	NextPage bool            `json:"next_page"`
	List     []BehaviorEvent `json:"list"`
}

// BehaviorIndex 记录身份归属；PendingPath与ContentHash支持文件系统先成功后的安全重试。
type BehaviorIndex struct {
	CloudID                                  int
	FileID, ParentID, Path                   string
	IsDir                                    bool
	Size                                     int64
	SHA1                                     string
	Owned                                    bool
	ContentHash, PendingPath, PendingContent string
}

// BehaviorRoot 是账号已有STRM目录配置，不使用转存或其他账号作为回退。
type BehaviorRoot struct {
	ID                   int
	CloudPath, LocalPath string
}
