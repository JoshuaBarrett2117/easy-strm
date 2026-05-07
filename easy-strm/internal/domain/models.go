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

// TransferMethod 秒传方式常量
const (
	TransferMethod115Driver = "115driver"
	TransferMethodGo115     = "go115"
	TransferMethodAlist     = "alist"
)

// AccountType 账号类型枚举
const (
	AccountTypeResource = "resource"
	AccountTypeVIP      = "vip"
	AccountTypeBoth     = "both"
)

// AccountStatus 账号状态枚举
const (
	AccountStatusActive   = "active"
	AccountStatusCooling  = "cooling"
	AccountStatusDisabled = "disabled"
)

// StrmConfig 领域模型：STRM文件配置
type StrmConfig struct {
	ID               int       `json:"id"`
	Cloud115Id       int       `json:"cloud115_id"`
	NetDiskPath      string    `json:"net_disk_path"`
	LocalPath        string    `json:"local_path"`
	Cron             string    `json:"cron"`
	Extension        string    `json:"extension"`
	DirTreeFile      string    `json:"dir_tree_file"`
	SyncMode         string    `json:"sync_mode"`
	SourceAccount    int       `json:"source_account"`
	TargetAccount    int       `json:"target_account"`
	TargetDirectory  string    `json:"target_directory"`
	AutoCleanup      bool      `json:"auto_cleanup"`
	CleanupThreshold int       `json:"cleanup_threshold"`
	CleanupPolicy    string    `json:"cleanup_policy"`
	MaxConcurrency   int       `json:"max_concurrency"`
	CreateTime       time.Time `json:"create_time"`
	UpdateTime       time.Time `json:"update_time"`
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
	TaskTypeSyncFull        TaskType = "sync_full"
	TaskTypeSyncTransfer    TaskType = "sync_transfer"
	TaskTypeCleanup         TaskType = "cleanup"
	TaskTypeProxyRefresh    TaskType = "proxy_refresh"
	TaskTypeOrganize        TaskType = "organize"
	TaskTypeScrape          TaskType = "scrape"
	TaskTypeEmbyRefresh     TaskType = "emby_refresh"
	TaskTypeLibrarySync     TaskType = "library_sync"
	TaskTypeLibraryPipeline TaskType = "library_pipeline"
)

// TaskTypeNames 任务类型中文名称映射
var TaskTypeNames = map[TaskType]string{
	TaskTypeStrmGenerate:    "STRM文件生成",
	TaskTypeIncrementalSync: "增量同步",
	TaskTypeSyncFull:        "全量同步",
	TaskTypeSyncTransfer:    "秒传同步",
	TaskTypeCleanup:         "空间清理",
	TaskTypeProxyRefresh:    "直链刷新",
	TaskTypeOrganize:        "媒体整理",
	TaskTypeScrape:          "NFO刮削",
	TaskTypeEmbyRefresh:     "Emby库刷新",
	TaskTypeLibrarySync:     "媒体库同步",
	TaskTypeLibraryPipeline: "媒体入库流水线",
}

// TaskStatus 任务运行状态
type TaskStatus struct {
	TaskID         string                 `json:"task_id"`
	TaskType       TaskType               `json:"task_type"`
	TaskName       string                 `json:"task_name"`
	Status         string                 `json:"status"`
	Priority       int                    `json:"priority"`
	Progress       int                    `json:"progress"`
	TotalFiles     int                    `json:"total_files"`
	ProcessedFiles int                    `json:"processed_files"`
	SuccessFiles   int                    `json:"success_files"`
	FailedFiles    int                    `json:"failed_files"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	ErrorMessage   string                 `json:"error_message"`
	CreateTime     string                 `json:"create_time"`
	UpdateTime     string                 `json:"update_time"`
}

// NotificationConfig 通知配置领域模型
type NotificationConfig struct {
	ID        int       `json:"id"`
	Channel   string    `json:"channel"`
	Config    string    `json:"config"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FileInfo 文件信息领域模型（用于秒传服务）
type FileInfo struct {
	FileID   string `json:"file_id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Sha1     string `json:"sha1"`
	PickCode string `json:"pick_code"`
}

// 任务状态常量
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
	TaskStatusSkipped   = "skipped"
)

// FileListRequest 文件列表请求
type FileListRequest struct {
	SourceID  int    `json:"source_id"`
	Path      string `json:"path"`       // 当前目录路径
	Page      int    `json:"page"`       // 页码
	PageSize  int    `json:"page_size"`  // 每页数量
	SortField string `json:"sort_field"` // 排序字段
	SortOrder string `json:"sort_order"` // asc | desc
	Filter    string `json:"filter"`     // 过滤条件
	Search    string `json:"search"`     // 搜索关键词
}

// FileItem 文件项
type FileItem struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	IsDirectory  bool      `json:"is_directory"`
	Size         int64     `json:"size"`
	ModifiedTime time.Time `json:"modified_time"`
	FileType     string    `json:"file_type"` // video | audio | image | other
	SourceID     int       `json:"source_id"`
	SourceName   string    `json:"source_name"`
}
