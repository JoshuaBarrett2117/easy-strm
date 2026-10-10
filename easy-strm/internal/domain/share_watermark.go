package domain

// ShareIncrementalWatermark 是消费权威的成功只读镜像，不用于跳过 dirty 待办。
type ShareIncrementalWatermark struct {
	ProtocolVersion   int    `json:"protocol_version"`
	ConfigRevision    int64  `json:"config_revision"`
	PreparedRevision  int64  `json:"prepared_revision"`
	CompletedRevision int64  `json:"completed_revision"`
	Fingerprint       string `json:"config_fingerprint"`
	RunID             string `json:"run_id"`
	LastSuccessAt     string `json:"last_success_at"`
}
