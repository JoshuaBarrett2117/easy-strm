package main

import "time"

type User struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Password   string    `json:"password"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// Cloud115 115云账号信息。
type Cloud115 struct {
	ID                int        `json:"id"`
	Name              string     `json:"name"`
	Cookie            string     `json:"cookie"`
	RefreshToken      string     `json:"refresh_token"`
	AccessToken       string     `json:"access_token"`
	ExpiresIn         int        `json:"expires_in"`
	TransferAccountID int        `json:"transfer_account_id"`
	TransferDirectory string     `json:"transfer_directory"`
	AccountType       string     `json:"account_type"`
	QuotaUsed         int64      `json:"quota_used"`
	Priority          int        `json:"priority"`
	Status            string     `json:"status"`
	CoolingStartTime  *time.Time `json:"cooling_start_time"`
	TransferMethod    string     `json:"transfer_method"`
	AlistUrl          string     `json:"alist_url"`
	AlistToken        string     `json:"alist_token"`
	CreateTime        time.Time  `json:"create_time"`
	UpdateTime        time.Time  `json:"update_time"`
}

// StrmConfig STRM文件配置信息。
type StrmConfig struct {
	ID               int       `json:"id"`
	Cloud115Id       int       `json:"cloud115_id"`       // 115账号ID
	NetDiskPath      string    `json:"net_disk_path"`     // 网盘目录
	LocalPath        string    `json:"local_path"`        // 本地目录
	Cron             string    `json:"cron"`              // cron表达式
	Extension        string    `json:"extension"`         // STRM文件后缀名
	DirTreeFile      string    `json:"dir_tree_file"`     // 本地目录树文件路径
	SyncMode         string    `json:"sync_mode"`         // 同步模式: manual/instant/cron
	SourceAccount    int       `json:"source_account"`    // 源账号ID
	TargetAccount    int       `json:"target_account"`    // 目标账号ID
	TargetDirectory  string    `json:"target_directory"`  // 目标目录
	AutoCleanup      bool      `json:"auto_cleanup"`      // 自动清理
	CleanupThreshold int       `json:"cleanup_threshold"` // 清理阈值
	CleanupPolicy    string    `json:"cleanup_policy"`    // 清理策略
	MaxConcurrency   int       `json:"max_concurrency"`   // 最大并发数
	CreateTime       time.Time `json:"create_time"`
	UpdateTime       time.Time `json:"update_time"`
}

// SystemConfig 系统配置信息。
type SystemConfig struct {
	ID         int       `json:"id"`
	ConfigKey  string    `json:"config_key"` // 配置键
	ConfigVal  string    `json:"config_val"` // 配置值
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// StrmFile 已生成的STRM文件记录。
type StrmFile struct {
	ID            int       `json:"id"`
	StrmConfigID  int       `json:"strm_config_id"`  // 关联的STRM配置ID
	FileName      string    `json:"file_name"`       // 文件名
	FilePath      string    `json:"file_path"`       // 相对路径
	PickCode      string    `json:"pick_code"`       // 115文件pickcode
	Sha1          string    `json:"sha1"`            // 文件SHA1
	FileSize      int64     `json:"file_size"`       // 文件大小
	LocalStrmPath string    `json:"local_strm_path"` // 本地STRM文件路径
	CreateTime    time.Time `json:"create_time"`
	UpdateTime    time.Time `json:"update_time"`
}

// CronTask 定时任务配置。
type CronTask struct {
	ID             int        `json:"id"`
	TaskName       string     `json:"task_name"`        // 任务名：账号id+"增量更新任务"
	TaskType       string     `json:"task_type"`        // 任务类型：incremental_sync
	Cloud115ID     int        `json:"cloud115_id"`      // 关联的115账号ID
	StrmConfigID   int        `json:"strm_config_id"`   // 关联的STRM配置ID
	CronExpr       string     `json:"cron_expr"`        // cron表达式
	Status         string     `json:"status"`           // enabled/disabled
	LastRunTime    *time.Time `json:"last_run_time"`    // 上次执行时间
	NextRunTime    *time.Time `json:"next_run_time"`    // 下次执行时间
	LastRunStatus  string     `json:"last_run_status"`  // 上次执行状态
	LastRunMessage string     `json:"last_run_message"` // 上次执行消息
	CreateTime     time.Time  `json:"create_time"`
	UpdateTime     time.Time  `json:"update_time"`
}
