package domain

import "time"

// PlaybackRecord 表示一次成功解析 STRM 直链的调用，并不代表实际观看时长。
type PlaybackRecord struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Poster   string    `json:"poster"`
	URL      string    `json:"url"`
	Time     time.Time `json:"time"`
	IP       string    `json:"ip"`
	Location string    `json:"location"`
	Method   string    `json:"method"`
}
