package domain

import "time"

// EmbyPlaybackEvent 表示 easy-strm 采集到的一段连续播放记录。
type EmbyPlaybackEvent struct {
	ID             int64      `json:"id"`
	ServerID       int        `json:"server_id"`
	SessionID      string     `json:"session_id"`
	UserID         string     `json:"user_id"`
	UserName       string     `json:"user_name"`
	ItemID         string     `json:"item_id"`
	ItemType       string     `json:"item_type"`
	ItemName       string     `json:"item_name"`
	SeriesID       string     `json:"series_id"`
	SeriesName     string     `json:"series_name"`
	ImageTag       string     `json:"image_tag"`
	ClientName     string     `json:"client_name"`
	DeviceName     string     `json:"device_name"`
	PlaybackMethod string     `json:"playback_method"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	WatchedSeconds int64      `json:"watched_seconds"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	Paused         bool       `json:"paused"`
}

// EmbyPlaybackSample 是采集器写入播放状态所需的最小数据。
type EmbyPlaybackSample struct {
	ServerID       int
	SessionID      string
	UserID         string
	UserName       string
	ItemID         string
	ItemType       string
	ItemName       string
	SeriesID       string
	SeriesName     string
	ImageTag       string
	ClientName     string
	DeviceName     string
	PlaybackMethod string
	Paused         bool
}

// EmbyMonitorItem 表示 Emby 媒体及其首次发现时间。
type EmbyMonitorItem struct {
	ServerID    int       `json:"server_id"`
	ItemID      string    `json:"item_id"`
	ItemType    string    `json:"item_type"`
	Name        string    `json:"name"`
	SeriesName  string    `json:"series_name,omitempty"`
	Year        int       `json:"year,omitempty"`
	ImageTag    string    `json:"image_tag,omitempty"`
	DateCreated time.Time `json:"date_created"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	Baseline    bool      `json:"baseline"`
}

// EmbyMonitorState 记录单个实例的采集状态。
type EmbyMonitorState struct {
	ServerID            int        `json:"server_id"`
	BaselineCompletedAt *time.Time `json:"baseline_completed_at,omitempty"`
	LastSessionPollAt   *time.Time `json:"last_session_poll_at,omitempty"`
	LastItemPollAt      *time.Time `json:"last_item_poll_at,omitempty"`
	PluginAvailable     bool       `json:"plugin_available"`
	LastError           string     `json:"last_error,omitempty"`
}

// EmbyMonitorMeta 描述一次统计结果的数据来源和时间口径。
type EmbyMonitorMeta struct {
	Source         string     `json:"source"`
	GeneratedAt    time.Time  `json:"generated_at"`
	Timezone       string     `json:"timezone"`
	HistoryStartAt *time.Time `json:"history_start_at,omitempty"`
	DegradedReason string     `json:"degraded_reason,omitempty"`
}

// EmbyMonitorTrendPoint 表示按小时聚合的活跃趋势。
type EmbyMonitorTrendPoint struct {
	Time           time.Time `json:"time"`
	ActiveUsers    int       `json:"active_users"`
	WatchedSeconds int64     `json:"watched_seconds"`
}

// EmbyMonitorSummary 是概览页的核心指标。
type EmbyMonitorSummary struct {
	PeakUsers        int   `json:"peak_users"`
	TodaySeconds     int64 `json:"today_seconds"`
	WeekSeconds      int64 `json:"week_seconds"`
	MonthSeconds     int64 `json:"month_seconds"`
	TotalSeconds     int64 `json:"total_seconds"`
	TodayActiveUsers int   `json:"today_active_users"`
	WeekActiveUsers  int   `json:"week_active_users"`
	MonthActiveUsers int   `json:"month_active_users"`
}

// EmbyMonitorSession 表示当前正在观看的会话。
type EmbyMonitorSession struct {
	SessionID      string  `json:"session_id"`
	UserID         string  `json:"user_id"`
	UserName       string  `json:"user_name"`
	ItemID         string  `json:"item_id"`
	ItemName       string  `json:"item_name"`
	SeriesName     string  `json:"series_name,omitempty"`
	ImageTag       string  `json:"image_tag,omitempty"`
	ClientName     string  `json:"client_name"`
	DeviceName     string  `json:"device_name"`
	PlaybackMethod string  `json:"playback_method"`
	Paused         bool    `json:"paused"`
	Progress       float64 `json:"progress"`
}

// EmbyMonitorOverview 是监控首页响应。
type EmbyMonitorOverview struct {
	Meta     EmbyMonitorMeta         `json:"meta"`
	Summary  EmbyMonitorSummary      `json:"summary"`
	Trend    []EmbyMonitorTrendPoint `json:"trend"`
	Sessions []EmbyMonitorSession    `json:"sessions"`
}

// EmbyMonitorRankingItem 是用户、媒体和客户端排行的统一行结构。
type EmbyMonitorRankingItem struct {
	Rank           int     `json:"rank"`
	ID             string  `json:"id,omitempty"`
	Name           string  `json:"name"`
	Subtitle       string  `json:"subtitle,omitempty"`
	ImageTag       string  `json:"image_tag,omitempty"`
	WatchedSeconds int64   `json:"watched_seconds"`
	PlayCount      int     `json:"play_count"`
	Percentage     float64 `json:"percentage"`
}

// EmbyMonitorRankingResponse 是统一排行响应。
type EmbyMonitorRankingResponse struct {
	Meta  EmbyMonitorMeta          `json:"meta"`
	Data  []EmbyMonitorRankingItem `json:"data"`
	Total int                      `json:"total"`
}

// EmbyMonitorHeatmapCell 表示一个日期小时格子的观看时长。
type EmbyMonitorHeatmapCell struct {
	Date    string `json:"date"`
	Hour    int    `json:"hour"`
	Seconds int64  `json:"seconds"`
}

// EmbyMonitorUserHeatmap 表示单个用户的热力图。
type EmbyMonitorUserHeatmap struct {
	UserID       string                   `json:"user_id"`
	UserName     string                   `json:"user_name"`
	TotalSeconds int64                    `json:"total_seconds"`
	Cells        []EmbyMonitorHeatmapCell `json:"cells"`
}

// EmbyMonitorHeatmapResponse 是热力图响应。
type EmbyMonitorHeatmapResponse struct {
	Meta  EmbyMonitorMeta          `json:"meta"`
	Users []EmbyMonitorUserHeatmap `json:"users"`
}

// EmbyMonitorRecentResponse 是最近入库响应。
type EmbyMonitorRecentResponse struct {
	Meta  EmbyMonitorMeta   `json:"meta"`
	Data  []EmbyMonitorItem `json:"data"`
	Total int               `json:"total"`
}
