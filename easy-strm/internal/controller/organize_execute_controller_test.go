package controller

import (
	"testing"

	"easy-strm/internal/service"
)

func TestNormalizeOrganizeExecuteRequest(t *testing.T) {
	moveFiles := false
	req := organizeExecuteRequest{
		SourceID:   1,
		TargetPath: "D:/media",
		MoveFiles:  &moveFiles,
	}

	if err := normalizeOrganizeExecuteRequest(&req); err != nil {
		t.Fatalf("normalizeOrganizeExecuteRequest() error = %v", err)
	}
	if req.MediaType != "all" {
		t.Fatalf("MediaType = %q, want all", req.MediaType)
	}
	if req.ConflictPolicy != "skip" {
		t.Fatalf("ConflictPolicy = %q, want skip", req.ConflictPolicy)
	}
	if req.OperationMode != "copy" {
		t.Fatalf("OperationMode = %q, want copy", req.OperationMode)
	}
}

func TestNormalizeOrganizeExecuteRequestRequiresTargetWhenNoCategory(t *testing.T) {
	req := organizeExecuteRequest{SourceID: 1}
	if err := normalizeOrganizeExecuteRequest(&req); err == nil {
		t.Fatal("normalizeOrganizeExecuteRequest() expected target path error")
	}
}

func TestSummarizeAndClassifyOrganizeResults(t *testing.T) {
	results := []service.OrganizeResult{
		{Success: true},
		{Skipped: true},
		{Success: false, Message: "TMDB 识别失败"},
	}
	success, skipped, failed := summarizeOrganizeResults(results)
	if success != 1 || skipped != 1 || failed != 1 {
		t.Fatalf("summarizeOrganizeResults() = (%d,%d,%d), want (1,1,1)", success, skipped, failed)
	}

	if got := classifyOrganizeTaskFailure(&service.OrganizeResult{Skipped: true}); got != "conflict_skipped" {
		t.Fatalf("classify skipped = %q", got)
	}
	if got := classifyOrganizeTaskFailure(&service.OrganizeResult{Message: "TMDB 识别失败"}); got != "identify_failed" {
		t.Fatalf("classify identify = %q", got)
	}
	if got := classifyOrganizeTaskFailure(&service.OrganizeResult{Message: "file already exists"}); got != "conflict_skipped" {
		t.Fatalf("classify conflict = %q", got)
	}
}
