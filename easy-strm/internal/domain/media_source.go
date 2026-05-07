package domain

import "time"

// MediaSource 媒体源配置领域模型
// 支持本地存储和115云盘两种媒体源类型
type MediaSource struct {
	ID                 int       `json:"id"`                   // 主键ID
	Name               string    `json:"name"`                 // 媒体源名称
	SourceType         string    `json:"source_type"`          // 媒体源类型: local | cloud115
	Path               string    `json:"path"`                 // 本地路径或115目录CID
	WatchPath          string    `json:"watch_path"`           // 监控目录
	Cloud115ID         *int      `json:"cloud115_id"`          // 115账号ID（仅cloud115类型）
	Priority           int       `json:"priority"`             // 优先级（数值越小优先级越高）
	Enabled            bool      `json:"enabled"`              // 是否启用
	OrganizeTargetPath string    `json:"organize_target_path"` // 整理目标目录
	MediaType          string    `json:"media_type"`           // all | movie | tv
	ConflictPolicy     string    `json:"conflict_policy"`      // skip | overwrite | suffix
	OperationMode      string    `json:"operation_mode"`       // move | copy | hardlink | symlink
	AutoOrganize       bool      `json:"auto_organize"`        // 是否自动整理
	WatchEnabled       bool      `json:"watch_enabled"`        // 是否启用监控
	WatchInterval      int       `json:"watch_interval"`       // 轮询间隔（秒）
	EmbyLibraryID      string    `json:"emby_library_id"`      // Emby 媒体库 ID
	CreateTime         time.Time `json:"create_time"`          // 创建时间
	UpdateTime         time.Time `json:"update_time"`          // 更新时间
}

// SourceType 媒体源类型常量
const (
	SourceTypeLocal    = "local"    // 本地存储
	SourceTypeCloud115 = "cloud115" // 115云盘
)

// 文件操作类型常量
const (
	FileOperationMove   = "move"   // 移动操作
	FileOperationCopy   = "copy"   // 复制操作
	FileOperationDelete = "delete" // 删除操作
	FileOperationRename = "rename" // 重命名操作
)

// SourceTypeNames 媒体源类型中文名称映射
var SourceTypeNames = map[string]string{
	SourceTypeLocal:    "本地存储",
	SourceTypeCloud115: "115云盘",
}

// MediaFile 媒体文件信息模型
// 用于统一表示本地和115云盘的文件信息
type MediaFile struct {
	ID           string    `json:"id"`            // 文件唯一标识（本地为路径，115为pickcode）
	Name         string    `json:"name"`          // 文件名
	Path         string    `json:"path"`          // 文件路径
	Size         int64     `json:"size"`          // 文件大小（字节）
	Type         string    `json:"type"`          // 文件类型: dir | video | audio | image | subtitle | file
	IsDirectory  bool      `json:"is_directory"`  // 是否为目录
	Extension    string    `json:"extension"`     // 文件扩展名
	ModifyTime   time.Time `json:"modify_time"`   // 修改时间
	ModifiedTime time.Time `json:"modified_time"` // 修改时间（兼容字段）
	SourceID     int       `json:"source_id"`     // 所属媒体源ID
	SourceType   string    `json:"source_type"`   // 媒体源类型
	PickCode     string    `json:"pick_code"`     // 115文件pickcode（仅cloud115类型）
	CID          string    `json:"cid"`           // 115目录CID（仅cloud115类型）
	SHA1         string    `json:"sha1"`          // 文件SHA1（仅cloud115类型）
	TmdbTitle    string    `json:"tmdb_title"`    // TMDB识别标题（从缓存中获取）
}

// FileOperationResult 文件操作结果
type FileOperationResult struct {
	Success bool   `json:"success"`  // 操作是否成功
	Message string `json:"message"`  // 结果消息
	Source  string `json:"source"`   // 源文件路径
	Target  string `json:"target"`   // 目标文件路径（移动/复制操作）
	NewPath string `json:"new_path"` // 新路径（重命名/移动操作后）
}

// FileOperationRequest 通用文件操作请求
type FileOperationRequest struct {
	SourceType string `json:"source_type"` // 媒体源类型
	SourceID   int    `json:"source_id"`   // 源媒体源ID
	Operation  string `json:"operation"`   // 操作类型: move | copy | delete | rename
	SourcePath string `json:"source_path"` // 源文件路径
	TargetPath string `json:"target_path"` // 目标路径（移动/复制操作）
	NewName    string `json:"new_name"`    // 新文件名（重命名操作）
}

// BatchOperationResult 批量操作结果
type BatchOperationResult struct {
	Total   int                   `json:"total"`   // 总操作数
	Success int                   `json:"success"` // 成功数
	Failed  int                   `json:"failed"`  // 失败数
	Results []FileOperationResult `json:"results"` // 详细结果
}

// FileMoveRequest 文件移动请求
type FileMoveRequest struct {
	SourceID   int      `json:"source_id"`   // 源媒体源ID
	FileIDs    []string `json:"file_ids"`    // 文件ID列表
	TargetPath string   `json:"target_path"` // 目标路径
}

// FileCopyRequest 文件复制请求
type FileCopyRequest struct {
	SourceID     int      `json:"source_id"`     // 源媒体源ID
	TargetID     int      `json:"target_id"`     // 目标媒体源ID
	FileIDs      []string `json:"file_ids"`      // 文件ID列表
	TargetPath   string   `json:"target_path"`   // 目标路径
	DeleteSource bool     `json:"delete_source"` // 是否删除源文件（移动操作）
}

// FileDeleteRequest 文件删除请求
type FileDeleteRequest struct {
	SourceID int      `json:"source_id"` // 媒体源ID
	FileIDs  []string `json:"file_ids"`  // 文件ID列表
}

// FileRenameRequest 文件重命名请求
type FileRenameRequest struct {
	SourceID int    `json:"source_id"` // 媒体源ID
	FileID   string `json:"file_id"`   // 文件ID
	NewName  string `json:"new_name"`  // 新文件名
}

// BatchOperationRequest 批量操作请求
type BatchOperationRequest struct {
	Operation string               `json:"operation"` // 操作类型: move | copy | delete | rename
	SourceID  int                  `json:"source_id"` // 源媒体源ID
	TargetID  int                  `json:"target_id"` // 目标媒体源ID（复制操作需要）
	Items     []BatchOperationItem `json:"items"`     // 操作项列表
}

// BatchOperationItem 批量操作项
type BatchOperationItem struct {
	FileID     string `json:"file_id"`     // 文件ID
	TargetPath string `json:"target_path"` // 目标路径
	NewName    string `json:"new_name"`    // 新文件名（重命名操作）
}

// FileListQuery 文件列表查询参数
type FileListQuery struct {
	SourceID  int    `json:"source_id"`  // 媒体源ID
	Path      string `json:"path"`       // 当前路径
	Page      int    `json:"page"`       // 页码
	PageSize  int    `json:"page_size"`  // 每页数量
	SortField string `json:"sort_field"` // 排序字段
	SortOrder string `json:"sort_order"` // 排序方向: asc | desc
	Filter    string `json:"filter"`     // 过滤条件
	Search    string `json:"search"`     // 搜索关键词
}

// FileListResult 文件列表结果
type FileListResult struct {
	Total      int         `json:"total"`      // 总数
	Page       int         `json:"page"`       // 当前页
	PageSize   int         `json:"page_size"`  // 每页数量
	Files      []MediaFile `json:"files"`      // 文件列表
	Breadcrumb []PathItem  `json:"breadcrumb"` // 面包屑导航
}

// PathItem 路径项（面包屑导航）
type PathItem struct {
	Name string `json:"name"` // 目录名
	Path string `json:"path"` // 完整路径
}

// ============================================
// TMDB 相关模型
// ============================================

// TmdbSearchResult TMDB 搜索结果
type TmdbSearchResult struct {
	TmdbID        int      `json:"tmdb_id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Year          int      `json:"year"`
	PosterPath    string   `json:"poster_path"`
	Overview      string   `json:"overview"`
	VoteAverage   float64  `json:"vote_average"`
	MediaType     string   `json:"media_type"` // movie | tv
	ReleaseDate   string   `json:"release_date"`
	FirstAirDate  string   `json:"first_air_date"`
	GenreIDs      []int    `json:"genre_ids"`
	Countries     []string `json:"countries"`
	Language      string   `json:"language"`
}

// TmdbIdentifyResult TMDB 识别结果
type TmdbIdentifyResult struct {
	Success       bool               `json:"success"`
	Message       string             `json:"message"`
	Filename      string             `json:"filename"`
	MediaType     string             `json:"media_type"` // movie | tv | unknown
	TmdbID        int                `json:"tmdb_id"`
	Title         string             `json:"title"`
	OriginalTitle string             `json:"original_title"`
	Year          int                `json:"year"`
	SeasonNumber  int                `json:"season_number"`
	EpisodeNumber int                `json:"episode_number"`
	Quality       string             `json:"quality"`
	Source        string             `json:"source"`
	Codec         string             `json:"codec"`
	GenreIDs      []int              `json:"genre_ids"`
	Countries     []string           `json:"countries"`
	Language      string             `json:"language"`
	Candidates    []TmdbSearchResult `json:"candidates"` // Top 3 候选
}

// TmdbSearchRequest TMDB 搜索请求
type TmdbSearchRequest struct {
	Query     string `json:"query" form:"query"`           // 搜索关键词
	Year      int    `json:"year" form:"year"`             // 年份（可选）
	MediaType string `json:"media_type" form:"media_type"` // movie | tv（可选）
}

// TmdbIdentifyRequest TMDB 识别请求
type TmdbIdentifyRequest struct {
	FileID    string `json:"file_id" binding:"required"`   // 文件ID
	TmdbID    int    `json:"tmdb_id" binding:"required"`   // TMDB ID
	TmdbType  string `json:"tmdb_type" binding:"required"` // movie | tv
	Title     string `json:"title"`                        // 标题
	Year      int    `json:"year"`                         // 年份
	PosterURL string `json:"poster_url"`                   // 海报URL
}

// TmdbCacheRepository TMDB 缓存仓库接口
type TmdbCacheRepository interface {
	GetByQueryKey(queryKey, mediaType string) (interface{}, error)
}

// ============================================
// 更名相关模型
// ============================================

// RenamePreviewRequest 更名预览请求
type RenamePreviewRequest struct {
	SourceID  int    `json:"source_id" binding:"required"` // 媒体源ID
	FileID    string `json:"file_id" binding:"required"`   // 文件ID
	TmdbID    int    `json:"tmdb_id"`                      // TMDB ID
	MediaType string `json:"media_type"`                   // movie | tv
	Title     string `json:"title"`                        // 已识别标题（可选）
	Year      int    `json:"year"`                         // 已识别年份（可选）
	Season    int    `json:"season"`                       // 已识别季数（可选）
	Episode   int    `json:"episode"`                      // 已识别集数（可选）
	Template  string `json:"template"`                     // 更名模板
}

// RenamePreviewResult 更名预览结果
type RenamePreviewResult struct {
	FileID       string `json:"file_id"`       // 文件ID
	OriginalName string `json:"original_name"` // 原始文件名
	NewName      string `json:"new_name"`      // 新文件名
	OriginalPath string `json:"original_path"` // 原始路径
	NewPath      string `json:"new_path"`      // 新路径
	TmdbID       int    `json:"tmdb_id"`       // TMDB ID
	Title        string `json:"title"`         // 标题
	Year         int    `json:"year"`          // 年份
	MediaType    string `json:"media_type"`    // 媒体类型
	Season       int    `json:"season"`        // 季数
	Episode      int    `json:"episode"`       // 集数
	Quality      string `json:"quality"`       // 质量
}

// RenameExecuteRequest 更名执行请求
type RenameExecuteRequest struct {
	SourceID  int    `json:"source_id" binding:"required"` // 媒体源ID
	FileID    string `json:"file_id" binding:"required"`   // 文件ID
	NewName   string `json:"new_name" binding:"required"`  // 新文件名
	Overwrite bool   `json:"overwrite"`                    // 是否覆盖已存在文件
}

// RenameExecuteResult 更名执行结果
type RenameExecuteResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	OriginalPath string `json:"original_path"`
	NewPath      string `json:"new_path"`
}

// RenamePreset 更名预设
type RenamePreset struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	MediaType  string `json:"media_type"`
	Template   string `json:"template"`
	Enabled    bool   `json:"enabled"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
}

// OrganizeManualOverride 整理预览中的手动识别覆盖项
type OrganizeManualOverride struct {
	FileID        string `json:"file_id"`
	CloudID       string `json:"cloud_id"`
	MediaType     string `json:"media_type"`
	TmdbID        int    `json:"tmdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          int    `json:"year"`
	Season        int    `json:"season"`
	Episode       int    `json:"episode"`
}

// OrganizeRenameOverride 整理执行中的文件名覆盖项
type OrganizeRenameOverride struct {
	FileID  string `json:"file_id"`
	CloudID string `json:"cloud_id"`
	NewName string `json:"new_name"`
}
