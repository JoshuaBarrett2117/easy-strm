package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type selectionBudgetStore struct {
	selectionServiceFake
	checkContext context.Context
	syncContext  context.Context
}

func (store *selectionBudgetStore) CheckSelectionSchema(ctx context.Context) error {
	store.checkContext = ctx
	return ctx.Err()
}

func (store *selectionBudgetStore) SyncSelections(ctx context.Context, work string) error {
	store.syncContext = ctx
	return ctx.Err()
}

func TestSelectionRefreshTotalBudgetAndCancellation(t *testing.T) {
	store := &selectionBudgetStore{}
	service := NewShareSelectionService(store)
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline, bounded := store.checkContext.Deadline()
	if !bounded || time.Until(deadline) > 15*time.Minute || store.syncContext != store.checkContext {
		t.Fatal("结构检查和同步必须共享总预算")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store.syncContext = nil
	if err := service.Refresh(ctx); !errors.Is(err, context.Canceled) || store.syncContext != nil {
		t.Fatalf("已取消刷新不应执行同步：%v", err)
	}
}
