package domain

import "time"

const (
	SyncStatusActive  = "active"
	SyncStatusMissing = "missing"
	SyncStatusDeleted = "deleted"
)

const (
	IdentityStatusUnknown    = "unknown"
	IdentityStatusIdentified = "identified"
	IdentityStatusFailed     = "failed"
)

// MediaSyncIndex 记录源文件、本地媒体库文件、STRM 与媒体服务器之间的映射。
type MediaSyncIndex struct {
	ID                   int        `json:"id"`
	SourceID             int        `json:"source_id"`
	SourceType           string     `json:"source_type"`
	SourceFileID         string     `json:"source_file_id"`
	SourcePath           string     `json:"source_path"`
	SourceName           string     `json:"source_name"`
	SourcePickCode       string     `json:"source_pick_code"`
	SourceSHA1           string     `json:"source_sha1"`
	SourceSize           int64      `json:"source_size"`
	SourceModifiedTime   *time.Time `json:"source_modified_time"`
	TargetPath           string     `json:"target_path"`
	StrmPath             string     `json:"strm_path"`
	MetadataPath         string     `json:"metadata_path"`
	MediaServerType      string     `json:"media_server_type"`
	MediaServerLibraryID string     `json:"media_server_library_id"`
	TmdbID               int        `json:"tmdb_id"`
	MediaType            string     `json:"media_type"`
	IdentityStatus       string     `json:"identity_status"`
	SyncStatus           string     `json:"sync_status"`
	LastChangeType       string     `json:"last_change_type"`
	LastTaskID           string     `json:"last_task_id"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// TaskStep 是长任务的步骤化执行记录。
type TaskStep struct {
	ID            int        `json:"id"`
	TaskID        string     `json:"task_id"`
	StepKey       string     `json:"step_key"`
	StepName      string     `json:"step_name"`
	Status        string     `json:"status"`
	SortOrder     int        `json:"sort_order"`
	InputSummary  string     `json:"input_summary"`
	OutputSummary string     `json:"output_summary"`
	ErrorMessage  string     `json:"error_message"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PendingMediaItem 承接识别失败、人工修正和重新入库。
type PendingMediaItem struct {
	ID            int       `json:"id"`
	SourceKind    string    `json:"source_kind"`
	SourceID      int       `json:"source_id"`
	SourceFileID  string    `json:"source_file_id"`
	SourcePath    string    `json:"source_path"`
	Title         string    `json:"title"`
	Year          int       `json:"year"`
	MediaType     string    `json:"media_type"`
	Season        int       `json:"season"`
	Episode       int       `json:"episode"`
	TmdbID        int       `json:"tmdb_id"`
	Status        string    `json:"status"`
	Reason        string    `json:"reason"`
	RelatedTaskID string    `json:"related_task_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MediaLibraryItem 是媒体库视图使用的聚合条目。
type MediaLibraryItem struct {
	MediaSyncIndex
	HasStrm          bool   `json:"has_strm"`
	HasMetadata      bool   `json:"has_metadata"`
	HealthStatus     string `json:"health_status"`
	LatestTaskID     string `json:"latest_task_id"`
	LatestTaskStatus string `json:"latest_task_status"`
}
