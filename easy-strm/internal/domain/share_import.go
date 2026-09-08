package domain

// ShareImportEntry 保存解析结果及去重后的候选，供用户确认后使用已有创建接口导入。
type ShareImportEntry struct {
	ShareCode string   `json:"share_code"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Password  string   `json:"password"`
	Names     []string `json:"names"`
	Passwords []string `json:"passwords"`
	Lines     []int    `json:"lines"`
	Warnings  []string `json:"warnings"`
}

// ShareImportPreview 只描述本次文本的解析结果，不访问或修改已保存的分享。
type ShareImportPreview struct {
	Records    []ShareImportEntry `json:"records"`
	Duplicates int                `json:"duplicates"`
	Ignored    []string           `json:"ignored"`
}
