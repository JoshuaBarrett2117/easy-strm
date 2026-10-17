package service

import (
	"context"
	"easy-strm/internal/domain"
	"testing"
)

type selectionServiceFake struct {
	reads, changes, refreshes int
	work                      string
}

func (fake *selectionServiceFake) CheckSelectionSchema(context.Context) error { return nil }
func (fake *selectionServiceFake) ListSelections(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error) {
	fake.reads++
	return nil, 0, nil
}
func (fake *selectionServiceFake) SelectionDetail(context.Context, string) (domain.ShareSelectionDetail, error) {
	fake.reads++
	return domain.ShareSelectionDetail{}, nil
}
func (fake *selectionServiceFake) ChangeSelection(context.Context, domain.ShareSelectionChange) (string, error) {
	fake.changes++
	return fake.work, nil
}
func (fake *selectionServiceFake) SyncSelections(context.Context, string) error {
	fake.refreshes++
	return nil
}

func TestSelectionServiceValidationAndExplicitRefresh(t *testing.T) {
	fake := &selectionServiceFake{}
	service := NewShareSelectionService(fake)
	ctx := context.Background()
	for _, query := range []domain.ShareSelectionQuery{{Page: -1}, {PageSize: 101}, {Page: 1000001}} {
		if _, _, err := service.List(ctx, query); err == nil {
			t.Fatal("invalid pagination accepted")
		}
	}
	if _, _, err := service.List(ctx, domain.ShareSelectionQuery{}); err != nil {
		t.Fatal(err)
	}
	if fake.refreshes != 0 {
		t.Fatal("list discovered sources")
	}
	if _, err := service.Change(ctx, domain.ShareSelectionChange{ItemKey: "w", CandidateID: 1}); err == nil {
		t.Fatal("missing CAS accepted")
	}
	if err := service.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.reads != 1 || fake.changes != 0 || fake.refreshes != 1 {
		t.Fatalf("fake=%+v", fake)
	}
}

func TestSelectionChangeAppliesImmediately(t *testing.T) {
	fake := &selectionServiceFake{work: "tmdb:movie:10728"}
	service := NewShareSelectionService(fake)
	applied := ""
	service.SetImmediateExport(func(_ context.Context, work string) (string, error) {
		applied = work
		return "share_strm_immediate", nil
	})
	taskID, err := service.Change(context.Background(), domain.ShareSelectionChange{ItemKey: "tmdb:movie:10728", CandidateID: 7, ExpectedRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	if applied != "tmdb:movie:10728" || taskID != "share_strm_immediate" {
		t.Fatalf("改选未立即重建: applied=%q task=%q", applied, taskID)
	}
	if fake.changes != 1 {
		t.Fatalf("changes=%d", fake.changes)
	}
}
