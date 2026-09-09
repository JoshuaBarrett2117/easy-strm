package domain

// ShareRecord 分享主记录；媒体信息通过 Media 建立一对多关联。
type ShareRecord struct {
	MediaCount      int          `json:"media_count"`
	IdentifiedCount int          `json:"identified_count"`
	FailedCount     int          `json:"failed_count"`
	PendingCount    int          `json:"pending_count"`
	ShareCancelled  bool         `json:"share_cancelled"` // 网盘明确返回分享已取消时标记；任务取消或超时不影响该字段
	MaskedCount     int          `json:"masked_count"`    // 脱敏媒体数，包含在媒体总数中
	ID              int          `json:"id"`
	MediaType       string       `json:"media_type"`
	Name            string       `json:"name"`
	URL             string       `json:"url"`
	Password        string       `json:"password"`
	Note            string       `json:"note"`
	Version         int          `json:"version"`
	Media           []ShareMedia `json:"media"`
	CreatedAt       string       `json:"created_at"`
	UpdatedAt       string       `json:"updated_at"`
}

// ShareMedia 一个分享中的文件及其独立识别结果。
type ShareMedia struct {
	GalleryDuplicate bool                `json:"gallery_duplicate"` // 同一分享内已识别为相同剧集的后续记录，默认海报墙折叠
	ID               int                 `json:"id"`
	ShareID          int                 `json:"share_id"`
	MediaType        string              `json:"media_type,omitempty"`
	FileName         string              `json:"file_name"`
	MetadataSource   string              `json:"metadata_source"`
	Status           string              `json:"status"`
	Result           *TmdbIdentifyResult `json:"result,omitempty"`
	Error            string              `json:"error"`
	Version          int                 `json:"version"`
}

// ShareRecordQuery 分享分页搜索条件。
type ShareRecordQuery struct {
	ShareID int
	Summary  bool
	Keyword  string
	Status   string
	Page     int
	PageSize int
}

// ShareRecordPage 分享分页结果。
type ShareRecordPage struct {
	Data  []ShareRecord `json:"data"`
	Total int           `json:"total"`
}

// ShareIdentifySummary 当前请求实际处理的媒体数量。
type ShareIdentifySummary struct {
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// ShareMediaPage 已识别媒体的数据库分页结果，重复数按整个分享计算。
type ShareMediaPage struct {
	Data           []ShareMedia `json:"data"`
	Total          int          `json:"total"`
	DuplicateCount int          `json:"duplicate_count"`
}
