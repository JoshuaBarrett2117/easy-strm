package domain

// ShareLibraryQuery 资源库作品级查询；同维度多选取并集。
type ShareLibraryQuery struct {
	Keyword   string   `form:"keyword"`
	TmdbID    int64    `form:"tmdb_id"`
	MediaType string   `form:"media_type"`
	YearMin   int      `form:"year_min"`
	YearMax   int      `form:"year_max"`
	RatingMin *float64 `form:"rating_min"`
	RatingMax *float64 `form:"rating_max"`
	Genres    string   `form:"genres"`
	Countries string   `form:"countries"`
	Available bool     `form:"available"`
	Sort      string   `form:"sort"`
	Direction string   `form:"direction"`
	Page      int      `form:"page"`
	PageSize  int      `form:"page_size"`
	FileIDs   []int    `form:"-" json:"-"` // 仅供解析完成后的内部增量导出限定本批文件
}

// ShareLibraryPage 返回数据库分页结果，Data 可承载作品或来源。
type ShareLibraryPage struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
}

// ShareLibraryTVMedia 表示资源库剧集的稳定身份，供 Service 获取外部季集元数据。
type ShareLibraryTVMedia struct {
	WorkKey        string `json:"work_key"`
	TmdbID         int64  `json:"tmdb_id"`
	Title          string `json:"title"`
	MediaType      string `json:"media_type"`
	MetadataSource string `json:"metadata_source"`
}

// ShareLibraryTVSeasonSummary 表示一季的目录信息和本地资源覆盖情况。
type ShareLibraryTVSeasonSummary struct {
	SeasonNumber        int    `json:"season_number"`
	Name                string `json:"name"`
	Overview            string `json:"overview,omitempty"`
	AirDate             string `json:"air_date,omitempty"`
	PosterPath          string `json:"poster_path,omitempty"`
	EpisodeCount        int    `json:"episode_count"`
	MatchedEpisodeCount int    `json:"matched_episode_count"`
	FileCount           int    `json:"file_count"`
}

// ShareLibraryTVDetail 返回剧集及其完整季目录。
type ShareLibraryTVDetail struct {
	WorkKey          string                        `json:"work_key"`
	TmdbID           int64                         `json:"tmdb_id,omitempty"`
	Title            string                        `json:"title"`
	Overview         string                        `json:"overview,omitempty"`
	MetadataComplete bool                          `json:"metadata_complete"`
	Warning          string                        `json:"warning,omitempty"`
	Seasons          []ShareLibraryTVSeasonSummary `json:"seasons"`
}

// ShareLibraryFile 表示某一集可关联的真实分享文件。
type ShareLibraryFile struct {
	ID             int    `json:"id"`
	ShareID        int    `json:"share_id"`
	RemoteFileID   string `json:"remote_file_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	Available      bool   `json:"available"`
	Status         string `json:"status"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Password       string `json:"password"`
	ShareCancelled bool   `json:"share_cancelled"`
}

// ShareLibraryEpisode 表示完整剧集目录中的一集及其关联文件。
type ShareLibraryEpisode struct {
	EpisodeNumber int                `json:"episode_number"`
	Name          string             `json:"name"`
	Overview      string             `json:"overview,omitempty"`
	AirDate       string             `json:"air_date,omitempty"`
	StillPath     string             `json:"still_path,omitempty"`
	Runtime       int                `json:"runtime,omitempty"`
	Files         []ShareLibraryFile `json:"files"`
}

// ShareLibraryTVSeasonDetail 返回单季完整集目录和文件映射。
type ShareLibraryTVSeasonDetail struct {
	SeasonNumber     int                   `json:"season_number"`
	Name             string                `json:"name"`
	Overview         string                `json:"overview,omitempty"`
	AirDate          string                `json:"air_date,omitempty"`
	PosterPath       string                `json:"poster_path,omitempty"`
	MetadataComplete bool                  `json:"metadata_complete"`
	Warning          string                `json:"warning,omitempty"`
	Episodes         []ShareLibraryEpisode `json:"episodes"`
}
