package service

import (
	"testing"
	"time"
)

func TestOrganizeSnapshotRejectsChangedSelection(t *testing.T) {
	now := time.Now()
	selection := OrganizeSnapshotSelection{SourceID: 1, TargetPath: "target", MediaType: "movie", FileIDs: []string{"a", "b"}}
	task := &OrganizePreviewTask{Status: "completed", UpdatedAt: now, Selection: cloneSnapshotSelection(selection), Result: &TaskResult{Previews: []OrganizePreview{{FileID: "a", Title: "A"}, {FileID: "b", Title: "B"}}}}
	selection.FileIDs = []string{"b"}
	rows, err := validateOrganizeSnapshot(task, selection, now)
	if err != nil || len(rows) != 1 || rows[0].Title != "B" {
		t.Fatalf("subset failed: %+v %v", rows, err)
	}
	for _, change := range []func(*OrganizeSnapshotSelection){
		func(s *OrganizeSnapshotSelection) { s.TargetPath = "other" },
		func(s *OrganizeSnapshotSelection) { s.SourceID = 2 },
		func(s *OrganizeSnapshotSelection) { s.MediaType = "tv" },
		func(s *OrganizeSnapshotSelection) { s.FileIDs = []string{"unknown"} },
		func(s *OrganizeSnapshotSelection) { s.FileIDs = []string{"a", "a"} },
	} {
		changed := selection
		change(&changed)
		if _, err := validateOrganizeSnapshot(task, changed, now); err == nil {
			t.Fatalf("accepted changed selection: %+v", changed)
		}
	}
	if _, err := validateOrganizeSnapshot(task, selection, now.Add(25*time.Hour)); err == nil {
		t.Fatal("accepted expired snapshot")
	}
	task.Selection = nil
	if _, err := validateOrganizeSnapshot(task, selection, now); err == nil {
		t.Fatal("accepted legacy snapshot without policy")
	}
}
