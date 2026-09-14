package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"easy-strm/internal/domain"
)

// OrganizeSnapshotSelection 描述执行时必须与预览保持一致的识别及路径策略。
type OrganizeSnapshotSelection struct {
	SourceID    int                             `json:"source_id"`
	SourcePath  string                          `json:"source_path"`
	TargetPath  string                          `json:"target_path"`
	MediaType   string                          `json:"media_type"`
	Template    string                          `json:"template"`
	UseCategory bool                            `json:"use_category"`
	FileIDs     []string                        `json:"file_ids"`
	ManualItems []domain.OrganizeManualOverride `json:"manual_items"`
}

// validateOrganizeSnapshot 只接受已完成且策略未变化的服务端预览，允许执行选中子集。
func validateOrganizeSnapshot(task *OrganizePreviewTask, selection OrganizeSnapshotSelection, now time.Time) ([]OrganizePreview, error) {
	if task == nil || task.Status != "completed" || task.Result == nil || task.Selection == nil {
		return nil, fmt.Errorf("预览快照不存在或尚未完成，请重新预览")
	}
	if now.Sub(task.UpdatedAt) > 24*time.Hour || task.UpdatedAt.After(now.Add(time.Minute)) {
		return nil, fmt.Errorf("预览快照已过期，请重新预览")
	}
	expected, actual := *task.Selection, selection
	expected.FileIDs, actual.FileIDs = nil, nil
	// JSON归一化避免nil与空手动覆盖列表的表示差异。
	if len(expected.ManualItems) == 0 {
		expected.ManualItems = nil
	}
	if len(actual.ManualItems) == 0 {
		actual.ManualItems = nil
	}
	if !reflect.DeepEqual(expected, actual) {
		return nil, fmt.Errorf("整理策略或人工识别已变化，请重新预览")
	}
	rows := make(map[string]OrganizePreview, len(task.Result.Previews))
	for _, row := range task.Result.Previews {
		rows[row.FileID] = row
	}
	selected := selection.FileIDs
	if len(selected) == 0 {
		for _, row := range task.Result.Previews {
			selected = append(selected, row.FileID)
		}
	}
	seen := make(map[string]bool, len(selected))
	result := make([]OrganizePreview, 0, len(selected))
	for _, id := range selected {
		row, ok := rows[id]
		if !ok || seen[id] {
			return nil, fmt.Errorf("执行文件不在预览中或重复，请重新预览")
		}
		seen[id] = true
		result = append(result, row)
	}
	return result, nil
}

// cloneSnapshotSelection 脱离调用方切片，防止后台任务创建后请求对象被修改。
func cloneSnapshotSelection(selection OrganizeSnapshotSelection) *OrganizeSnapshotSelection {
	data, _ := json.Marshal(selection)
	var result OrganizeSnapshotSelection
	_ = json.Unmarshal(data, &result)
	return &result
}
