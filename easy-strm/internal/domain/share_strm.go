package domain

// ShareStrmSettings 配置服务器导出位置及播放时的目标账号，不保存账号凭据副本。
type ShareStrmSettings struct {
	OutputPath   string `json:"output_path"`
	BaseURL      string `json:"base_url"`
	Cloud115ID   int    `json:"cloud115_id"`
	TransferPath string `json:"transfer_path"`
}

// ShareStrmSource 是可导出来源及作品识别信息。
type ShareStrmSource struct {
	MediaID      int
	Remaining    int
	ID           int
	ShareID      int
	ShareName    string
	WorkKey      string
	URL          string
	Password     string
	FileName     string
	RemoteFileID string
	Episodes     []ShareEpisode
	Result       TmdbIdentifyResult
}

// ShareStrmEntry 保存单个视频的稳定播放映射，独立于分享管理记录生命周期。
type ShareStrmEntry struct {
	ID         string         `json:"id"`
	ShareCode  string         `json:"share_code"`
	Password   string         `json:"password"`
	FileID     string         `json:"file_id"`
	FileName   string         `json:"file_name"`
	FilePath   string         `json:"file_path,omitempty"`   // 本地媒体记录中的分享相对路径，首次播放按此定位文件ID
	MediaID    int            `json:"media_id,omitempty"`    // 分享媒体主数据ID，供播放记录稳定关联元数据
	Title      string         `json:"title,omitempty"`       // 导出时的作品标题，媒体记录被清理后仍可展示
	PosterPath string         `json:"poster_path,omitempty"` // 导出时的海报路径，避免播放时依赖文件名猜测
	Episodes   []ShareEpisode `json:"episodes,omitempty"`    // 文件对应的完整季集映射，支持多集合一
}
