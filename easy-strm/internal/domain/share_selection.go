package domain

// ShareSelection 表示一个作品季集的持久选择，路径与来源无关。
type ShareSelection struct {
	ItemKey          string `json:"media_item_key"`
	WorkKey          string `json:"work_key"`
	Season           int    `json:"season_number"`
	Episode          int    `json:"episode_number"`
	CandidateID      int64  `json:"selected_candidate_id"`
	Mode             string `json:"selection_mode"`
	Revision         int64  `json:"selection_revision"`
	ExportedRevision int64  `json:"exported_revision"`
	RelativePath     string `json:"stable_relative_path"`
	Title            string `json:"title"`
	Valid            bool   `json:"valid"`
}

// ShareCandidate 是不含播放地址和凭据的候选详情。
type ShareCandidate struct {
	ID        int64  `json:"candidate_id"`
	SourceID  int    `json:"source_file_id"`
	ShareID   int    `json:"share_id"`
	ShareName string `json:"share_name"`
	FileName  string `json:"file_name"`
	FileSize  int64  `json:"file_size"`
	FirstSeen int64  `json:"first_seen_seq"`
	Available bool   `json:"available"`
	State     string `json:"state"`
	Revoked   bool   `json:"revoked"`
}

// ShareSelectionDetail 返回选择和全部成员候选，包括失效的原手选。
type ShareSelectionDetail struct {
	Selection  ShareSelection   `json:"selection"`
	Candidates []ShareCandidate `json:"candidates"`
}

// ShareSelectionQuery 定义分页搜索，不允许读取播放凭据。
type ShareSelectionQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Keyword  string `form:"keyword"`
}

// ShareSelectionChange 使用版本 CAS，改选仅影响下一次导出。
type ShareSelectionChange struct {
	ItemKey          string `json:"media_item_key" binding:"required"`
	CandidateID      int64  `json:"candidate_id" binding:"required,gt=0"`
	ExpectedRevision int64  `json:"expected_revision" binding:"required,gt=0"`
}
