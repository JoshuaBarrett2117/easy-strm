package domain

// ShareBaselineAdoptionRequest 只接受人工确认、独立核验引用和完整快照护栏，不包含凭据。
type ShareBaselineAdoptionRequest struct {
	ManualConfirmation string           `json:"manual_confirmation"`
	Reason             string           `json:"reason"`
	DBAAttestation     string           `json:"dba_attestation"`
	ExpectedOutputPath string           `json:"expected_output_path"`
	Expected           map[string]int64 `json:"expected"`
	ActorUserID        int              `json:"-"`
}

// ShareBaselineAdoptionResult 返回人工采纳的记账摘要；AlreadyReady 时绝不消费新增待办。
type ShareBaselineAdoptionResult struct {
	ManualConfirmation string `json:"manual_confirmation"`
	TaskID             string `json:"task_id"`
	BaselineState      string `json:"baseline_state"`
	CompletedRevision  int64  `json:"completed_revision"`
	AcknowledgedWorks  int64  `json:"acknowledged_works"`
	CheckpointSources  int64  `json:"checkpoint_sources"`
	SelectionCount     int64  `json:"selection_count"`
	OutputCount        int64  `json:"output_count"`
	AlreadyReady       bool   `json:"already_ready"`
}
