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
	resolveErr   error
	syncDuration time.Duration
}

// SyncSelections 保留既有假实现并模拟可取消的固定同步成本，不访问真实 SQL。
func (store *consumingPerfSelections) SyncSelections(ctx context.Context, work string) error {
	started := time.Now()
	defer func() { store.syncDuration += time.Since(started) }()
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
				var err error
				if path == "direct" {
					if err = fixture.service.tasks.Create("perf", "strm_generate", "消费探针"); err != nil {
						t.Fatal(err)
					}
					err = runIncrementalFixture(t, fixture, runner.worker, "perf")
				} else {
					err = phase03Run(t, fixture, runner, mode, "perf", context.Background())
				}
				t.Logf("LOCAL SIMULATED fixed_sync_cost=%s path=%s N=%d page_size=100 sync_calls=%d cumulative_measured_sync_duration=%s resolve_calls=%d PRODUCTION EXTRAPOLATION ONLY per_call=130s estimated_sync_total=%s (not production measurement; excludes whole-run elapsed)", consumingSimulatedSyncCost, path, count, len(selections.syncWorks), instrumented.syncDuration, selections.resolves, time.Duration(len(selections.syncWorks))*130*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				wantSync, wantResolve := (count+99)/100, count
				if path == "full" {
					wantSync, wantResolve = 1, 1
				}
				if path == "recovery" && count > 0 {
					wantSync = 1
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

func TestShareStrmRecoveryLaterPagePreparation(t *testing.T) {
	for _, reason := range []string{"dirty-reread", "source-change", "cas-change", "resolve-error", "cancel-resolve", "cancel-observe", "queue-empty"} {
		t.Run(reason, func(t *testing.T) {
			fixture, checkpoints, selections, runner := p0Fixture(t)
			checkpoints.input.BaselineState = "building"
			checkpoints.pageSize = 2
			for index := 0; index < 5; index++ {
				checkpoints.bump(fmt.Sprintf("work:%02d", index))
			}
			const target = "work:02"
			checkpoints.observations = map[string][]domain.ShareExportSourceState{
				target: {{SourceFileID: 7, WorkKey: target, FileVersion: 1, State: "stale"}},
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("后页解析故障")
			var wantErr error
			wantCompletions, wantPending, wantRevision := 5, 0, int64(1)
			wantPhase := "completed"
			switch reason {
			case "source-change", "cas-change":
				wantErr, wantCompletions, wantPending, wantRevision = dao.ErrShareExportChanged, 4, 1, 0
				wantPhase = "pending"
			case "resolve-error":
				wantErr, wantCompletions, wantPending, wantRevision = failure, 2, 2, 0
				wantPhase = "failed"
			case "cancel-resolve", "cancel-observe":
				wantErr, wantCompletions, wantPending, wantRevision = context.Canceled, 2, 2, 0
				wantPhase = "cancelled"
			case "dirty-reread":
				wantRevision = 2
			case "queue-empty":
				wantCompletions = 2
			}
			observedTarget := false
			checkpoints.afterObserve = func(work string) {
				if work != target || observedTarget {
					return
				}
				observedTarget = true
				if checkpoints.completions != 2 {
					t.Fatalf("来源观察必须按页有界，不能预读全部队列：completions=%d", checkpoints.completions)
				}
				if reason == "cancel-observe" {
					cancel()
				}
			}
			selections.onResolveWork = func(work string) error {
				if work != target {
					return nil
				}
				_, metadata := p0Metadata(t, fixture, "recovery-later-page")
				if !observedTarget || metadata["consumed_works"] != float64(2) || metadata["round_consumed_works"] != float64(2) {
					t.Fatalf("后页解析前必须保留来源观察与前页进度：observed=%v metadata=%v", observedTarget, metadata)
				}
				switch reason {
				case "dirty-reread":
					checkpoints.bump(target)
				case "source-change":
					checkpoints.observations[target][0].FileVersion++
				case "cas-change":
					checkpoints.beforeComplete = func() {
						checkpoints.bump(target)
						checkpoints.beforeComplete = nil
					}
				case "resolve-error":
					return failure
				case "cancel-resolve":
					cancel()
				case "queue-empty":
					for index := 2; index < 5; index++ {
						work := fmt.Sprintf("work:%02d", index)
						checkpoints.acked[work] = checkpoints.dirty[work].Revision
					}
				}
				return nil
			}
			err := phase03Run(t, fixture, runner, "reconciliation", "recovery-later-page", ctx)
			_, metadata := p0Metadata(t, fixture, "recovery-later-page")
			if !errors.Is(err, wantErr) || len(selections.syncWorks) != 1 || selections.checks != 1 || checkpoints.fulls != 0 || checkpoints.fanouts != 0 {
				t.Fatalf("恢复只能同步一次且不能跳过后页准备或重新全量/播种：err=%v want=%v sync=%v schema=%d full=%d fanout=%d", err, wantErr, selections.syncWorks, selections.checks, checkpoints.fulls, checkpoints.fanouts)
			}
			if !observedTarget || checkpoints.completions != wantCompletions || checkpoints.acked[target] != wantRevision || metadata["consumed_works"] != float64(wantCompletions) || metadata["pending_works"] != float64(wantPending) || metadata["phase"] != wantPhase {
				t.Fatalf("后页围栏/刷新/进度/终态不符：observed=%v completions=%d ack=%v metadata=%v", observedTarget, checkpoints.completions, checkpoints.acked, metadata)
			}
			wantResolves := 5
			if reason == "resolve-error" || reason == "cancel-resolve" {
				wantResolves = 3
			} else if reason == "cancel-observe" {
				wantResolves = 2
			} else if reason == "queue-empty" {
				wantResolves = 4
			}
			if selections.resolves != wantResolves {
				t.Fatalf("恢复仍需逐作品解析：resolve=%d want=%d", selections.resolves, wantResolves)
			}
		})
	}
}

func TestShareStrmRecoverySyncIsInvocationScoped(t *testing.T) {
	for _, reason := range []string{"sync-error", "resolve-error", "cancel-resolve"} {
		t.Run(reason, func(t *testing.T) {
			fixture, checkpoints, selections, runner := p0Fixture(t)
			checkpoints.input.BaselineState = "building"
			checkpoints.pageSize = 2
			for index := 0; index < 5; index++ {
				checkpoints.bump(fmt.Sprintf("work:%02d", index))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("恢复准备故障")
			wantErr := failure
			if reason == "sync-error" {
				selections.prepareErr = failure
			} else {
				if reason == "cancel-resolve" {
					wantErr = context.Canceled
				}
				selections.onResolveWork = func(work string) error {
					if work == "work:02" {
						if reason == "cancel-resolve" {
							cancel()
							return nil
						}
						return failure
					}
					return nil
				}
			}
			if err := phase03Run(t, fixture, runner, "reconciliation", "recovery-interrupted", ctx); !errors.Is(err, wantErr) {
				t.Fatal(err)
			}
			if len(selections.syncWorks) != 1 {
				t.Fatalf("中断调用重复同步：sync=%v", selections.syncWorks)
			}
			selections.prepareErr, selections.onResolveWork = nil, nil
			if err := phase03Run(t, fixture, runner, "reconciliation", "recovery-retry", context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(selections.syncWorks) != 2 || checkpoints.completions != 5 || checkpoints.fulls != 0 || checkpoints.fanouts != 0 || checkpoints.input.BaselineState != "ready" {
				t.Fatalf("新调用必须重新同步一次并仅恢复待办：sync=%v completions=%d full=%d fanout=%d state=%s", selections.syncWorks, checkpoints.completions, checkpoints.fulls, checkpoints.fanouts, checkpoints.input.BaselineState)
			}
		})
	}
}
