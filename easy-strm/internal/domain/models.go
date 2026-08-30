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
	CookieSource      string     `json:"cookie_source"`
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
	TaskTypeEmbyServer      TaskType = "emby_server"
	TaskTypeEmbyUser        TaskType = "emby_user"
	TaskTypeEmbyLibrary     TaskType = "emby_library"
	TaskTypeEmbyCover       TaskType = "emby_cover"
	TaskTypeEmbyPlugin      TaskType = "emby_plugin"
	TaskTypeOfflineDownload TaskType = "offline_download"
	TaskTypeFileTransfer    TaskType = "file_transfer"
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
	TaskTypeEmbyServer:      "Emby实例管理",
	TaskTypeEmbyUser:        "Emby用户管理",
	TaskTypeEmbyLibrary:     "Emby媒体库管理",
	TaskTypeEmbyCover:       "Emby媒体库封面",
	TaskTypeEmbyPlugin:      "神医助手任务",
	TaskTypeOfflineDownload: "115云下载",
	TaskTypeFileTransfer:    "文件传输",
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
	TaskStatusPending        = "pending"
	TaskStatusRunning        = "running"
	TaskStatusCompleted      = "completed"
	TaskStatusSuccess        = "success"
	TaskStatusPartialSuccess = "partial_success"
	TaskStatusUnknown        = "unknown"
	TaskStatusFailed         = "failed"
	TaskStatusCancelled      = "cancelled"
	TaskStatusSkipped        = "skipped"
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

// ========== 115分享链接转存相关领域模型 ==========

// ShareFileInfo 分享文件信息（支持嵌套目录结构）
type ShareFileInfo struct {
	Name     string          `json:"name"`      // 文件名
	Size     int64           `json:"size"`      // 文件大小（字节）
	Type     string          `json:"type"`      // 文件类型：video/audio/image/folder/other
	Path     string          `json:"path"`      // 文件在分享中的路径
	PickCode string          `json:"pick_code"` // 文件pickcode（分享场景通常为空）
	Fid      string          `json:"fid"`       // 分享文件ID（115 file_id，转存时使用）
	Sha1     string          `json:"sha1"`      // 文件SHA1
	IsDir    bool            `json:"is_dir"`    // 是否为目录
	Children []ShareFileInfo `json:"children"`  // 子文件/子目录列表
}

// ParseShareRequest 解析分享链接请求
type ParseShareRequest struct {
	URL      string `json:"url"`      // 115分享链接
	Password string `json:"password"` // 分享密码（可选）
}

// ParseShareResponse 解析分享链接响应
type ParseShareResponse struct {
	ShareCode  string          `json:"share_code"`  // 分享码
	FolderName string          `json:"folder_name"` // 分享文件夹名称
	Files      []ShareFileInfo `json:"files"`       // 文件列表
	TotalFiles int             `json:"total_files"` // 文件总数
	TotalSize  int64           `json:"total_size"`  // 总大小（字节）
}

// ShareTransferFileItem 转存文件项（用于提交转存请求）
type ShareTransferFileItem struct {
	PickCode string `json:"pick_code"` // 文件pickcode（分享场景通常为空）
	Fid      string `json:"fid"`       // 分享文件ID（115 file_id，转存时使用）
	Name     string `json:"name"`      // 文件名
	Size     int64  `json:"size"`      // 文件大小（字节）
}

// TransferRequest 转存任务请求
type TransferRequest struct {
	ShareCode          string                  `json:"share_code"`           // 分享码
	Password           string                  `json:"password"`             // 分享密码
	TargetCloud115Id   int                     `json:"target_cloud115_id"`   // 目标115账号ID
	TargetDirectory    string                  `json:"target_directory"`     // 目标目录路径
	Files              []ShareTransferFileItem `json:"files"`                // 待转存文件列表
	ConflictStrategy   string                  `json:"conflict_strategy"`    // 冲突策略：skip/overwrite/rename
	AutoOrganize       bool                    `json:"auto_organize"`        // 是否转存完成后自动整理
	AutoScrape         bool                    `json:"auto_scrape"`          // 是否整理完成后自动刮削（115 云盘目标本期优雅降级为 skipped）
	OrganizeSourceID   int                     `json:"organize_source_id"`   // 复用已有媒体源的规则（0=使用临时/方案B源）
	OrganizeTargetPath string                  `json:"organize_target_path"` // 整理目标路径（可选，缺省=TargetDirectory）
}

// TransferResponse 转存任务提交响应
type TransferResponse struct {
	TaskId        string `json:"task_id"`        // 任务ID
	TotalFiles    int    `json:"total_files"`    // 总文件数
	EstimatedSize int64  `json:"estimated_size"` // 预估总大小（字节）
}

// TransferProgressResponse 转存任务进度响应
type TransferProgressResponse struct {
	TaskId             string       `json:"task_id"`             // 任务ID
	Status             string       `json:"status"`              // 任务状态
	Progress           int          `json:"progress"`            // 进度百分比
	TotalFiles         int          `json:"total_files"`         // 总文件数
	ProcessedFiles     int          `json:"processed_files"`     // 已处理文件数
	SuccessFiles       int          `json:"success_files"`       // 成功文件数
	FailedFiles        int          `json:"failed_files"`        // 失败文件数
	SkippedFiles       int          `json:"skipped_files"`       // 跳过文件数
	FailedItems        []FailedItem `json:"failed_items"`        // 失败文件明细
	CurrentFile        string       `json:"current_file"`        // 当前处理文件名
	EstimatedRemaining string       `json:"estimated_remaining"` // 预计剩余时间
	CreateTime         string       `json:"create_time"`         // 创建时间
	UpdateTime         string       `json:"update_time"`         // 更新时间
}

// FailedItem 失败文件项
type FailedItem struct {
	Name      string `json:"name"`      // 文件名
	Error     string `json:"error"`     // 错误信息
	Retryable bool   `json:"retryable"` // 是否可重试
}

// CancelResponse 取消转存响应
type CancelResponse struct {
	TaskId         string `json:"task_id"`         // 任务ID
	Status         string `json:"status"`          // 最终状态
	CompletedFiles int    `json:"completed_files"` // 已完成文件数
	CancelledFiles int    `json:"cancelled_files"` // 被取消的文件数
	Message        string `json:"message"`         // 提示信息
}

// RetryResponse 重试转存响应
type RetryResponse struct {
	TaskId       string `json:"task_id"`       // 任务ID
	RetriedFiles int    `json:"retried_files"` // 重试文件数
	Message      string `json:"message"`       // 提示信息
}

// ShareTransferLog 分享转存日志（对应 t_share_transfer_log 表）
type ShareTransferLog struct {
	ID               int    `json:"id"`                 // 主键ID
	TaskId           string `json:"task_id"`            // 任务ID
	ShareCode        string `json:"share_code"`         // 分享码
	ShareFolderName  string `json:"share_folder_name"`  // 分享文件夹名称
	FileName         string `json:"file_name"`          // 文件名
	FilePickCode     string `json:"file_pick_code"`     // 文件pickcode
	FileSize         int64  `json:"file_size"`          // 文件大小
	FileSha1         string `json:"file_sha1"`          // 文件SHA1
	Cloud115Id       int    `json:"cloud115_id"`        // 目标115账号ID
	TargetDirectory  string `json:"target_directory"`   // 目标目录路径
	Status           string `json:"status"`             // 状态
	ErrorMessage     string `json:"error_message"`      // 错误信息
	IsSecondTransfer bool   `json:"is_second_transfer"` // 是否二传
	CreateTime       string `json:"create_time"`        // 创建时间
	UpdateTime       string `json:"update_time"`        // 更新时间
}
