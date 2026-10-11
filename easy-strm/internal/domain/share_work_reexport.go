package domain

// ShareWorkReexportMaxWorks 限制一次人工重导的作品范围，禁止空范围与全库回退。
const ShareWorkReexportMaxWorks = 100

// ShareWorkReexportMaxSelections 限制一次任务的选择行数，超限必须拆分请求。
const ShareWorkReexportMaxSelections = 10000

// ShareWorkReexportRequest 固定人工范围、输出目录与配置版本，不包含基线采纳授权。
type ShareWorkReexportRequest struct {
	WorkKeys               []string `json:"work_keys"`
	Reason                 string   `json:"reason"`
	ExpectedOutputPath     string   `json:"expected_output_path"`
	ExpectedConfigRevision int64    `json:"expected_config_revision"`
	ActorUserID            int      `json:"-"`
}

// ShareWorkReexportSelection 将现有选择绑定到候选的实际文件和媒体，禁止按作品猜测媒体行。
type ShareWorkReexportSelection struct {
	Selection ShareSelection
	SourceID  int
	MediaID   int
}

// ShareWorkReexportWorkResult 保留逐作品进度与部分失败，失败任务不会自动重试或扩大范围。
type ShareWorkReexportWorkResult struct {
	WorkKey       string `json:"work_key"`
	SelectionRows int    `json:"selection_rows"`
	ProcessedRows int    `json:"processed_rows"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

// ShareWorkReexportResult 明确区分请求范围、已完成范围及全局只读 invalid 计数。
type ShareWorkReexportResult struct {
	RequestedWorks      []string                      `json:"requested_works"`
	RequestedWorkCount  int                           `json:"requested_work_count"`
	ProcessedWorks      []string                      `json:"processed_works"`
	ProcessedWorkCount  int                           `json:"processed_work_count"`
	SelectionRows       int                           `json:"selection_rows"`
	ProcessedRows       int                           `json:"processed_rows"`
	Written             int                           `json:"written"`
	GlobalInvalidBefore *int64                        `json:"global_invalid_before"`
	GlobalInvalidAfter  *int64                        `json:"global_invalid_after"`
	InvalidScope        string                        `json:"invalid_scope"`
	BaselineState       string                        `json:"baseline_state"`
	Works               []ShareWorkReexportWorkResult `json:"works"`
	Error               string                        `json:"error,omitempty"`
	AfterCountError     string                        `json:"after_count_error,omitempty"`
}
