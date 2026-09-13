package domain

import "time"

// PlaybackMetadata 提供播放记录的标题、海报和文件级季集信息。
type PlaybackMetadata struct {
	Title    string
	Poster   string
	Episodes []ShareEpisode
}

// PlaybackRecord 表示由一次或多次连续直链解析归并成的 STRM 播放会话，并不代表实际观看时长。
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
