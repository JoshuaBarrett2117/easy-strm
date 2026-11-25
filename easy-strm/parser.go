package main

// VideoFile 表示一个视频文件

type VideoFile struct {
	Path         string `json:"path"`         // 完整路径
	Filename     string `json:"filename"`     // 文件名
	CID          string `json:"cid"`          // 云盘目录ID
	FID          string `json:"fid"`          // 云盘文件ID（pickcode）
	Size         int    `json:"size"`         // 文件大小
	Extension    string `json:"extension"`    // 文件扩展名
	Sha1         string `json:"sha1"`         // SHA1哈希值（也用于存储网盘绝对路径）
	Cloud115ID   int    `json:"cloud115Id"`   // 115账号ID
	RelativePath string `json:"relativePath"` // 相对路径
	PickCode     string `json:"pickCode"`     // 文件pickcode
	Name         string `json:"name"`         // 文件名（同Filename）
}

// VideoCollection 视频文件集合
type VideoCollection struct {
	Videos []VideoFile `json:"videos"`
	Total  int         `json:"total"`
}
