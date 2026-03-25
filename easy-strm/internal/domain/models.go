package domain

import (
	"time"
)

// User 领域模型：系统用户
type User struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Password   string    `json:"password"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// Cloud115 领域模型：115云账号
type Cloud115 struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	Cookie            string    `json:"cookie"`
	RefreshToken      string    `json:"refresh_token"`
	AccessToken       string    `json:"access_token"`
	ExpiresIn         int       `json:"expires_in"`
	TransferAccountID int       `json:"transfer_account_id"`
	TransferDirectory string    `json:"transfer_directory"`
	CreateTime        time.Time `json:"create_time"`
	UpdateTime        time.Time `json:"update_time"`
}

// StrmConfig 领域模型：STRM文件配置
type StrmConfig struct {
	ID          int       `json:"id"`
	Cloud115Id  int       `json:"cloud115_id"`
	NetDiskPath string    `json:"net_disk_path"`
	LocalPath   string    `json:"local_path"`
	Cron        string    `json:"cron"`
	Extension   string    `json:"extension"`
	DirTreeFile string    `json:"dir_tree_file"`
	CreateTime  time.Time `json:"create_time"`
	UpdateTime  time.Time `json:"update_time"`
}

// SystemConfig 领域模型：系统配置
type SystemConfig struct {
	ID         int       `json:"id"`
	ConfigKey  string    `json:"config_key"`
	ConfigVal  string    `json:"config_val"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// StrmFile 领域模型：已生成的STRM文件记录
type StrmFile struct {
	ID            int       `json:"id"`
	StrmConfigID  int       `json:"strm_config_id"`
	FileName      string    `json:"file_name"`
	FilePath      string    `json:"file_path"`
	PickCode      string    `json:"pick_code"`
	Sha1          string    `json:"sha1"`
	FileSize      int64     `json:"file_size"`
	LocalStrmPath string    `json:"local_strm_path"`
	CreateTime    time.Time `json:"create_time"`
	UpdateTime    time.Time `json:"update_time"`
}

// CronTask 领域模型：定时任务配置
type CronTask struct {
	ID             int        `json:"id"`
	TaskName       string     `json:"task_name"`
	TaskType       string     `json:"task_type"`
	Cloud115ID     int        `json:"cloud115_id"`
	StrmConfigID   int        `json:"strm_config_id"`
	CronExpr       string     `json:"cron_expr"`
	Status         string     `json:"status"`
	LastRunTime    *time.Time `json:"last_run_time"`
	NextRunTime    *time.Time `json:"next_run_time"`
	LastRunStatus  string     `json:"last_run_status"`
	LastRunMessage string     `json:"last_run_message"`
	CreateTime     time.Time  `json:"create_time"`
	UpdateTime     time.Time  `json:"update_time"`
}

// TaskType 任务类型枚举
type TaskType string

const (
	TaskTypeStrmGenerate    TaskType = "strm_generate"
	TaskTypeIncrementalSync TaskType = "incremental_sync"
)

// TaskTypeNames 任务类型中文名称映射
var TaskTypeNames = map[TaskType]string{
	TaskTypeStrmGenerate:    "STRM文件生成",
	TaskTypeIncrementalSync: "增量同步",
}

// TaskStatus 任务运行状态
type TaskStatus struct {
	TaskID         string   `json:"task_id"`
	TaskType       TaskType `json:"task_type"`
	TaskName       string   `json:"task_name"`
	Status         string   `json:"status"`
	Progress       int      `json:"progress"`
	TotalFiles     int      `json:"total_files"`
	ProcessedFiles int      `json:"processed_files"`
	SuccessFiles   int      `json:"success_files"`
	FailedFiles    int      `json:"failed_files"`
	ErrorMessage   string   `json:"error_message"`
	CreateTime     string   `json:"create_time"`
	UpdateTime     string   `json:"update_time"`
}

// 任务状态常量
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
)
