package domain

// ========== 115云下载（离线下载）相关领域模型 ==========

// 云下载记录状态枚举
const (
	// OfflineStatusPending 已提交115，等待调度
	OfflineStatusPending = "pending"
	// OfflineStatusDownloading 115正在下载
	OfflineStatusDownloading = "downloading"
	// OfflineStatusCompleted 下载完成，文件已入库
	OfflineStatusCompleted = "completed"
	// OfflineStatusFailed 115离线下载失败
	OfflineStatusFailed = "failed"
	// OfflineStatusCancelled 用户取消
	OfflineStatusCancelled = "cancelled"
	// OfflineStatusRemoved 任务已不在115离线列表（被手动删除或过期清理）
	OfflineStatusRemoved = "removed"
)

// OfflineDownloadSubmitRequest 115云下载提交请求
type OfflineDownloadSubmitRequest struct {
	Cloud115ID int `json:"cloud115_id"` // 目标115账号ID
	// DownloadCloud115ID 可选的实际云下载账号；未提供且目标账号无云下载权限时自动选择VIP/兼顾账号。
	DownloadCloud115ID int      `json:"download_cloud115_id,omitempty"`
	Directory          string   `json:"directory"` // 保存目录路径（缺省为 /云下载，不存在时自动创建）
	Urls               []string `json:"urls"`      // ed2k/magnet/http/https/ftp 链接列表
}

// OfflineDownloadUrlResult 单个链接的提交结果
type OfflineDownloadUrlResult struct {
	Url      string `json:"url"`       // 原始链接
	InfoHash string `json:"info_hash"` // 115返回的任务info_hash（未接受时为空）
	Accepted bool   `json:"accepted"`  // 115是否接受该链接
	Message  string `json:"message"`   // 拒绝原因（接受时为空）
}

// OfflineDownloadSubmitResponse 云下载提交响应
type OfflineDownloadSubmitResponse struct {
	TaskId      string                     `json:"task_id"`      // 任务中心跟踪任务ID
	Total       int                        `json:"total"`        // 提交链接总数
	Accepted    int                        `json:"accepted"`     // 115已接受的链接数；后台排队时为0
	Rejected    int                        `json:"rejected"`     // 已确认被拒绝或格式非法的链接数
	Queued      bool                       `json:"queued"`       // 是否已进入后台发送队列
	QueuedCount int                        `json:"queued_count"` // 等待后台发送到115的有效链接数
	Results     []OfflineDownloadUrlResult `json:"results"`      // 同步提交时的逐链接结果明细
}

// OfflineDownloadTask 云下载记录（对应 t_offline_download_task 表）
type OfflineDownloadTask struct {
	ID           int64   `json:"id"`            // 主键ID
	TaskId       string  `json:"task_id"`       // 提交批次任务ID（任务中心）
	Cloud115ID   int     `json:"cloud115_id"`   // 执行下载的115账号ID
	AccountName  string  `json:"account_name"`  // 账号名称（查询时回填，不落库）
	Url          string  `json:"url"`           // 原始下载链接
	InfoHash     string  `json:"info_hash"`     // 115离线任务info_hash
	Name         string  `json:"name"`          // 任务/文件名称（115回填）
	Size         int64   `json:"size"`          // 文件大小（字节，115回填）
	Status       string  `json:"status"`        // 记录状态，见 OfflineStatus* 常量
	Percent      float64 `json:"percent"`       // 下载进度（0-100）
	ErrorMessage string  `json:"error_message"` // 错误信息
	SaveDirID    string  `json:"save_dir_id"`   // 保存目录cid
	CreateTime   string  `json:"create_time"`   // 创建时间
	UpdateTime   string  `json:"update_time"`   // 更新时间
}

// OfflineDownloadTaskListResponse 云下载记录列表响应（保持 data+total 稳定结构）
type OfflineDownloadTaskListResponse struct {
	Data  []OfflineDownloadTask `json:"data"`
	Total int                   `json:"total"`
}
