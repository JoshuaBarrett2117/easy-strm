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
}

// ShareLibraryPage 返回数据库分页结果，Data 可承载作品或来源。
type ShareLibraryPage struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
}
