package domain

import "time"

// EmbyServer 表示一个可管理的 Emby 服务实例。
type EmbyServer struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	BaseURL    string    `json:"base_url"`
	APIKey     string    `json:"-"`
	APIKeyMask string    `json:"api_key_mask"`
	Enabled    bool      `json:"enabled"`
	IsDefault  bool      `json:"is_default"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// EmbyUserPolicy 是 easy-strm 首期支持维护的 Emby 常用权限集合。
type EmbyUserPolicy struct {
	IsAdministrator                 bool     `json:"IsAdministrator"`
	IsDisabled                      bool     `json:"IsDisabled"`
	EnableRemoteAccess              bool     `json:"EnableRemoteAccess"`
	EnableMediaPlayback             bool     `json:"EnableMediaPlayback"`
	EnableVideoPlaybackTranscoding  bool     `json:"EnableVideoPlaybackTranscoding"`
	EnableAudioPlaybackTranscoding  bool     `json:"EnableAudioPlaybackTranscoding"`
	EnableContentDownloading        bool     `json:"EnableContentDownloading"`
	EnableSubtitleDownloading       bool     `json:"EnableSubtitleDownloading"`
	EnableSubtitleManagement        bool     `json:"EnableSubtitleManagement"`
	EnableRemoteControlOfOtherUsers bool     `json:"EnableRemoteControlOfOtherUsers"`
	EnableAllFolders                bool     `json:"EnableAllFolders"`
	EnabledFolders                  []string `json:"EnabledFolders"`
}

// EmbyUser 是管理页面使用的 Emby 用户摘要。
type EmbyUser struct {
	ID                    string         `json:"Id"`
	Name                  string         `json:"Name"`
	HasPassword           bool           `json:"HasPassword"`
	HasConfiguredPassword bool           `json:"HasConfiguredPassword"`
	PrimaryImageTag       string         `json:"PrimaryImageTag"`
	Policy                EmbyUserPolicy `json:"Policy"`
}

// EmbyMediaPath 表示 Emby 媒体库中的一个媒体目录。
type EmbyMediaPath struct {
	Path string `json:"Path"`
}

// EmbyLibraryInput 表示新增或修改媒体库时允许设置的常用参数。
type EmbyLibraryInput struct {
	Name                  string          `json:"name"`
	CollectionType        string          `json:"collection_type"`
	Paths                 []EmbyMediaPath `json:"paths"`
	MetadataLanguage      string          `json:"metadata_language"`
	MetadataCountry       string          `json:"metadata_country"`
	EnableRealtimeMonitor bool            `json:"enable_realtime_monitor"`
}
