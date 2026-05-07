package service

import (
	"testing"
	"time"
)

func TestFilterCloud115LifeEventsUsesCursorAndIgnoresBrowseEvents(t *testing.T) {
	events := []Cloud115LifeEvent{
		{ID: 101, UpdateTime: 1700000101, Type: 8, EventName: "browse_video"},
		{ID: 102, UpdateTime: 1700000102, Type: 2, EventName: "upload_file"},
		{ID: 99, UpdateTime: 1700000099, Type: 22, EventName: "delete_file"},
		{ID: 103, UpdateTime: 1700000103, Type: 24, EventName: "file_rename"},
	}

	filtered := filterCloud115LifeEvents(events, cloud115LifeCursor{LastEventID: 100})

	if len(filtered) != 2 {
		t.Fatalf("expected 2 useful events, got %d: %#v", len(filtered), filtered)
	}
	if filtered[0].ID != 102 || filtered[1].ID != 103 {
		t.Fatalf("unexpected filtered events: %#v", filtered)
	}
}

func TestFilterCloud115LifeEventsFallsBackToUpdateTime(t *testing.T) {
	events := []Cloud115LifeEvent{
		{ID: 0, UpdateTime: 1700000100, Type: 2, EventName: "upload_file"},
		{ID: 0, UpdateTime: 1700000200, Type: 6, EventName: "move_file"},
	}

	filtered := filterCloud115LifeEvents(events, cloud115LifeCursor{LastUpdateTime: 1700000100})

	if len(filtered) != 1 || filtered[0].UpdateTime != 1700000200 {
		t.Fatalf("expected only newer update_time event, got %#v", filtered)
	}
}

func TestCursorFromLifeEventsKeepsLatestEvent(t *testing.T) {
	fallback := time.Unix(1700000000, 0)
	cursor := cursorFromLifeEvents(&Cloud115LifeEventResp{
		Events: []Cloud115LifeEvent{
			{ID: 201, UpdateTime: 1700000201},
			{ID: 205, UpdateTime: 1700000199},
			{ID: 203, UpdateTime: 1700000300},
		},
	}, fallback)

	if cursor.LastEventID != 205 {
		t.Fatalf("expected max event id 205, got %d", cursor.LastEventID)
	}
	if cursor.LastUpdateTime != 1700000300 {
		t.Fatalf("expected max update_time 1700000300, got %d", cursor.LastUpdateTime)
	}
	if cursor.LastReconciledAt != fallback.Unix() {
		t.Fatalf("expected fallback reconciled timestamp, got %d", cursor.LastReconciledAt)
	}
}

func TestSummarizeCloud115LifeEventTypes(t *testing.T) {
	summary := summarizeCloud115LifeEventTypes([]Cloud115LifeEvent{
		{Type: 2, EventName: "upload_file"},
		{Type: 2, EventName: "upload_file"},
		{Type: 22},
	})

	if summary["upload_file"] != 2 || summary["22"] != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
