package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"testing"
	"time"
)

const consumingSimulatedSyncCost = 5 * time.Millisecond

type consumingPerfSelections struct {
	*p0SelectionStore
	resolveErr error
}

// SyncSelections 保留既有假实现并模拟可取消的固定同步成本，不访问真实 SQL。
func (store *consumingPerfSelections) SyncSelections(ctx context.Context, work string) error {
	if err := store.p0SelectionStore.SyncSelections(ctx, work); err != nil {
		return err
	}
	// 仅模拟 SQL 同步事务的固定成本，不代表真实数据库或生产耗时。
	timer := time.NewTimer(consumingSimulatedSyncCost)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ResolveSelections 记录目标作品解析次数并返回可注入的本地故障。
func (store *consumingPerfSelections) ResolveSelections(ctx context.Context, work string) (int, error) {
	count, err := store.p0SelectionStore.ResolveSelections(ctx, work)
	return count, errors.Join(err, store.resolveErr)
}

func TestShareStrmConsumingPreparationCost(t *testing.T) {
	for _, path := range []string{"direct", "incremental", "recovery", "full"} {
		for _, count := range []int{0, 1, 10, 100, 205} {
			t.Run(fmt.Sprintf("%s/N=%d", path, count), func(t *testing.T) {
				fixture, checkpoints, selections, runner := p0Fixture(t)
				instrumented := &consumingPerfSelections{p0SelectionStore: selections}
				fixture.service.store = instrumented
				checkpoints.pageSize = 100
				for index := 0; index < count; index++ {
					checkpoints.bump(fmt.Sprintf("work:%03d", index))
				}
				mode := "incremental"
				if path == "recovery" {
					mode = "reconciliation"
					checkpoints.input.BaselineState = "building"
				}
				if path == "full" {
					mode = "reconciliation"
					phase03ExpectFull(t, fixture, checkpoints.phase03Checkpoints)
					runner.full = runner.reconcile
				}
				started := time.Now()
				var err error
				if path == "direct" {
					if err = fixture.service.tasks.Create("perf", "strm_generate", "消费探针"); err != nil {
						t.Fatal(err)
					}
					err = runIncrementalFixture(t, fixture, runner.worker, "perf")
				} else {
					err = phase03Run(t, fixture, runner, mode, "perf", context.Background())
				}
				elapsed := time.Since(started)
				perWork := time.Duration(0)
				if count > 0 {
					perWork = elapsed / time.Duration(count)
				}
				t.Logf("LOCAL SIMULATED fixed_sync_cost=%s path=%s N=%d page_size=100 sync_calls=%d resolve_calls=%d elapsed=%s per_work=%s", consumingSimulatedSyncCost, path, count, len(selections.syncWorks), selections.resolves, elapsed, perWork)
				if err != nil {
					t.Fatal(err)
				}
				wantSync, wantResolve := (count+99)/100, count
				if path == "full" {
					wantSync, wantResolve = 1, 1
				}
				if len(selections.syncWorks) != wantSync || selections.resolves != wantResolve {
					t.Errorf("昂贵准备不得逐作品重复；解析仍需隔离作品：sync=%d want=%d resolve=%d want=%d", len(selections.syncWorks), wantSync, selections.resolves, wantResolve)
				}
				_, metadata := p0Metadata(t, fixture, "perf")
				if checkpoints.completions != count || metadata["phase"] != "completed" || metadata["consumed_works"] != float64(count) {
					t.Fatalf("分页消费丢失或没有有界完成：completions=%d metadata=%v", checkpoints.completions, metadata)
				}
				if path == "recovery" && (checkpoints.fulls != 0 || checkpoints.fanouts != 0) {
					t.Fatal("恢复重复执行全量或播种")
				}
			})
		}
	}
}

func TestShareStrmConsumingRoundSourceFence(t *testing.T) {
	for _, path := range []string{"incremental", "recovery"} {
		t.Run(path, func(t *testing.T) {
			fixture, checkpoints, selections, runner := p0Fixture(t)
			checkpoints.bump("work:a")
			source := phaseSource(7)
			source.WorkKey = "work:b"
			fixture.store.sources = []domain.ShareStrmSource{source}
			checkpoints.bump(source.WorkKey)
			mode := "incremental"
			if path == "recovery" {
				mode = "reconciliation"
				checkpoints.input.BaselineState = "building"
			}
			selections.onSync = func() { fixture.store.sources = nil }
			err := phase03Run(t, fixture, runner, mode, "perf-fence", context.Background())
			if !errors.Is(err, dao.ErrShareExportChanged) || checkpoints.acked[source.WorkKey] != 0 || checkpoints.acked["work:a"] != 1 {
				t.Fatalf("同步期间后续作品来源消失不得被空输出确认：err=%v ack=%v", err, checkpoints.acked)
			}
		})
	}
}

func TestShareStrmConsumingPreparationFailures(t *testing.T) {
	for _, reason := range []string{"cancel-sync", "sync-error", "resolve-error"} {
		t.Run(reason, func(t *testing.T) {
			fixture, checkpoints, selections, runner := p0Fixture(t)
			instrumented := &consumingPerfSelections{p0SelectionStore: selections}
			fixture.service.store = instrumented
			checkpoints.input.BaselineState = "building"
			for index := 0; index < 10; index++ {
				checkpoints.bump(fmt.Sprintf("work:%02d", index))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("本地准备故障")
			want := failure
			switch reason {
			case "cancel-sync":
				selections.onSync = cancel
				want = context.Canceled
			case "sync-error":
				selections.prepareErr = failure
			case "resolve-error":
				instrumented.resolveErr = failure
			}
			err := phase03Run(t, fixture, runner, "reconciliation", "perf-failure", ctx)
			_, metadata := p0Metadata(t, fixture, "perf-failure")
			if !errors.Is(err, want) || checkpoints.completions != 0 || len(selections.syncWorks) != 1 || metadata["phase"] == "completed" || metadata["pending_works"] != float64(10) {
				t.Fatalf("故障应中止且保留全部待办：err=%v sync=%v metadata=%v", err, selections.syncWorks, metadata)
			}
		})
	}
}
