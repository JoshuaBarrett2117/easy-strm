package domain

import "time"

// CronCleanupTarget 定义统一清空任务的账号目录；CID仅作兼容展示，执行时重新解析。
type CronCleanupTarget struct {
	ID         int       `json:"id"`
	CronTaskID int       `json:"cron_task_id"`
	Cloud115ID int       `json:"cloud115_id"`
	DirPath    string    `json:"dir_path"`
	DirCID     string    `json:"dir_cid,omitempty"`
	Enabled    bool      `json:"enabled"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}
