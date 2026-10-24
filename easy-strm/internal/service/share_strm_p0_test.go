package service

import (
	"bytes"
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type p0SelectionStore struct {
	*phaseStore
	selectionServiceFake
	checks, resolves int
	syncWorks        []string
	onSync           func()
	onSyncWork       func(string) error
	onResolveWork    func(string) error
	resolveWorks     []string
	prepareErr       error
}

func (store *p0SelectionStore) CheckSelectionSchema(ctx context.Context) error {
	store.checks++
	return ctx.Err()
}

func (store *p0SelectionStore) SyncSelections(ctx context.Context, work string) error {
	store.syncWorks = append(store.syncWorks, work)
	if store.onSyncWork != nil {
		if err := store.onSyncWork(work); err != nil {
			return err
		}
	}
	if store.onSync != nil {
		store.onSync()
	}
	return errors.Join(ctx.Err(), store.prepareErr)
}

func (store *p0SelectionStore) ResolveSelections(ctx context.Context, work string) (int, error) {
	store.resolves++
	store.resolveWorks = append(store.resolveWorks, work)
	if store.onResolveWork != nil {
		if err := store.onResolveWork(work); err != nil {
			return 0, err
		}
	}
	return 0, ctx.Err()
}

func (store *p0SelectionStore) ResolveSelection(context.Context, string) (domain.ShareSelection, int, error) {
	return domain.ShareSelection{}, 0, errors.New("测试不应解析空作品的来源")
}

func (store *p0SelectionStore) GetStrmSource(context.Context, int) (domain.ShareStrmSource, error) {
	return domain.ShareStrmSource{}, errors.New("测试不应读取空作品的来源")
}

func (store *p0SelectionStore) AnchorSelectionPath(_ context.Context, _, relative string) (string, error) {
	return relative, nil
}

type p0CheckpointStore struct {
	*phase03Checkpoints
	batchCalls   int
	pageSize     int
	afterRead    func(string)
	afterFinish  func()
	probeErr     error
	afterObserve func(string)
}

func (store *p0CheckpointStore) ObserveWork(ctx context.Context, work string) ([]domain.ShareExportSourceState, error) {
	states, err := store.incrementalMemory.ObserveWork(ctx, work)
	if store.afterObserve != nil {
		store.afterObserve(work)
	}
	return states, err
}

func (store *p0CheckpointStore) FinishBaseline(ctx context.Context, revision int64) (bool, error) {
	ready, err := store.incrementalMemory.FinishBaseline(ctx, revision)
	if ready && err == nil && store.afterFinish != nil {
		store.afterFinish()
	}
	return ready, err
}

func (store *p0CheckpointStore) DirtyBatch(ctx context.Context, after string) ([]domain.ShareExportDirty, error) {
	store.batchCalls++
	if store.probeErr != nil {
		return nil, store.probeErr
	}
	batch, err := store.incrementalMemory.DirtyBatch(ctx, after)
	if store.pageSize > 0 && len(batch) > store.pageSize {
		batch = batch[:store.pageSize]
	}
	if store.afterRead != nil {
		store.afterRead(after)
	}
	return batch, err
}

func p0Fixture(t *testing.T) (*phaseFixture, *p0CheckpointStore, *p0SelectionStore, *ShareStrmScheduledService) {
	t.Helper()
	fixture, checkpoints, runner := phase03Fixture(t)
	store := &p0CheckpointStore{phase03Checkpoints: checkpoints}
	selections := &p0SelectionStore{phaseStore: fixture.store}
	fixture.service.store = selections
	runner.store, runner.worker.checkpoints = store, store
	return fixture, store, selections, runner
}

func p0Metadata(t *testing.T, fixture *phaseFixture, id string) (map[string]interface{}, map[string]interface{}) {
	t.Helper()
	task, err := fixture.service.tasks.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return task, task["metadata"].(map[string]interface{})
}

func TestShareStrmP0RunEmptyQueueDoesNotPrepareSelections(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	if err := fixture.service.tasks.Create("p0-empty", "strm_generate", "空队列"); err != nil {
		t.Fatal(err)
	}
	if err := runIncrementalFixture(t, fixture, runner.worker, "p0-empty"); err != nil {
		t.Fatal(err)
	}
	if selections.checks != 1 || len(selections.syncWorks) != 0 || selections.resolves != 0 || store.batchCalls > 3 || store.reads != 0 {
		t.Fatalf("空队列执行了来源准备或重复消费：calls=%d selection=%+v", store.batchCalls, selections)
	}
}

func TestShareStrmP0WorkScopedPreparationAcrossPages(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	store.pageSize = 2
	for index := 0; index < 5; index++ {
		store.bump(fmt.Sprintf("work:%02d", index))
	}
	if err := phase03Run(t, fixture, runner, "incremental", "p0-pages", context.Background()); err != nil {
		t.Fatal(err)
	}
	wanted := []string{"work:00", "work:01", "work:02", "work:03", "work:04"}
	// 候选同步按页摊销，但必须保留逐作品解析与一次 schema 检查。
	if !reflect.DeepEqual(selections.syncWorks, []string{"", "", ""}) || !reflect.DeepEqual(selections.resolveWorks, wanted) || selections.checks != 1 {
		t.Fatalf("每轮同步一次，仅解析目标作品：sync=%v schema=%d resolve=%v", selections.syncWorks, selections.checks, selections.resolveWorks)
	}
	_, metadata := p0Metadata(t, fixture, "p0-pages")
	if store.completions != 5 || metadata["consumed_works"] != float64(5) || metadata["pending_works"] != float64(0) || metadata["phase"] != "completed" {
		t.Fatalf("消费未推进/未写终态：completions=%d metadata=%v", store.completions, metadata)
	}
}

func TestShareStrmP0PreparationCanEmptyQueue(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	store.bump("gone")
	selections.onSync = func() { store.acked["gone"] = store.dirty["gone"].Revision }
	if err := phase03Run(t, fixture, runner, "incremental", "p0-gone", context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(selections.syncWorks) != 1 || store.completions != 0 || store.batchCalls > 5 {
		t.Fatalf("准备后空队列未有界终止：sync=%v calls=%d", selections.syncWorks, store.batchCalls)
	}
}

func TestShareStrmP0PreparationRereadsDirtyRevisions(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	store.bump("work:a")
	selections.onSync = func() {
		if len(selections.syncWorks) == 1 {
			store.bump("work:a")
			store.bump("work:b")
		} else {
			store.bump("work:b")
		}
	}
	if err := phase03Run(t, fixture, runner, "incremental", "p0-revisions", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.acked["work:a"] != 2 || store.acked["work:b"] != 2 || !reflect.DeepEqual(selections.syncWorks, []string{"", ""}) || !reflect.DeepEqual(selections.resolveWorks, []string{"work:a", "work:b"}) {
		t.Fatalf("准备生成的 dirty revision/新作品丢失：acked=%v sync=%v", store.acked, selections.syncWorks)
	}
}

func TestShareStrmP0ProgressReportsBoundedPendingProbe(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	store.pageSize = 100
	for index := 0; index < 205; index++ {
		store.bump(fmt.Sprintf("work:%03d", index))
	}
	checked := false
	store.afterRead = func(after string) {
		if after != "work:099" {
			return
		}
		_, metadata := p0Metadata(t, fixture, "p0-progress")
		if metadata["consumed_works"] != float64(100) || metadata["round_consumed_works"] != float64(100) || metadata["pending_works"] != float64(100) || metadata["pending_works_exact"] != false || metadata["consumption_rounds"] != float64(1) {
			t.Fatalf("待办超过探测页时必须报告下界，不能伪造准确计数：%v", metadata)
		}
		checked = true
	}
	if err := phase03Run(t, fixture, runner, "incremental", "p0-progress", context.Background()); err != nil {
		t.Fatal(err)
	}
	_, metadata := p0Metadata(t, fixture, "p0-progress")
	if !checked || metadata["consumed_works"] != float64(205) || metadata["round_consumed_works"] != float64(5) || metadata["pending_works_exact"] != true || len(selections.syncWorks) != 3 || selections.resolves != 205 {
		t.Fatalf("消费进度未按有界页推进：%v", metadata)
	}
}

func TestShareStrmP0DisappearingBatchAdvancesCursor(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	store.bump("work:a")
	store.bump("work:b")
	store.pageSize = 1
	selections.onSync = func() { store.acked["work:a"] = store.dirty["work:a"].Revision }
	if err := phase03Run(t, fixture, runner, "incremental", "p0-disappeared", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.completions != 1 || store.acked["work:b"] != 1 || !reflect.DeepEqual(selections.syncWorks, []string{"", ""}) || !reflect.DeepEqual(selections.resolveWorks, []string{"work:a", "work:b"}) {
		t.Fatalf("消失页阻塞后续游标：completed=%d sync=%v", store.completions, selections.syncWorks)
	}
}

func TestShareStrmP0ScheduledEmptyCompletedPhase(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	if err := phase03Run(t, fixture, runner, "incremental", "p0-complete", context.Background()); err != nil {
		t.Fatal(err)
	}
	_, metadata := p0Metadata(t, fixture, "p0-complete")
	if metadata["phase"] != "completed" || metadata["pending_works"] != float64(0) || store.reads != 0 || len(selections.syncWorks) != 0 {
		t.Fatalf("空队列成功后仍显示 consuming：%v", metadata)
	}
}

func TestShareStrmP0FullMetricsRetainedAndPreparationNotRepeated(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	full := runner.full
	runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
		if err := fixture.service.prepareSelectionWork(ctx, selections, ""); err != nil {
			return err
		}
		if err := fixture.service.tasks.UpdateProgressContext(ctx, id, 440098, 440098, 440098, 0); err != nil {
			return err
		}
		if err := fixture.service.updateExportMetadata(ctx, id, map[string]interface{}{"written": 155023, "exported_files": 155023, "skipped_dedupe": 287521}); err != nil {
			return err
		}
		if err := full(ctx, input, id); err != nil {
			return err
		}
		store.bump("work:a")
		store.bump("work:b")
		return nil
	}
	if err := phase03Run(t, fixture, runner, "reconciliation", "p0-full", context.Background()); err != nil {
		t.Fatal(err)
	}
	task, metadata := p0Metadata(t, fixture, "p0-full")
	if task["processed_files"] != float64(440098) || task["total_files"] != float64(440098) || metadata["written"] != float64(155023) || metadata["exported_files"] != float64(155023) || metadata["skipped_dedupe"] != float64(287521) {
		t.Errorf("作品消费覆盖了全量来源/输出指标：task=%v metadata=%v", task, metadata)
	}
	if len(selections.syncWorks) != 1 || store.completions != 2 || metadata["phase"] != "completed" {
		t.Errorf("全量完成后重复准备或丢失检查点消费：sync=%v completions=%d metadata=%v", selections.syncWorks, store.completions, metadata)
	}
}

func TestShareStrmP0CancelledFailureAndConcurrentDirtyRemainPending(t *testing.T) {
	for _, mode := range []string{"cancelled", "preparation-failed", "commit-failed", "concurrent-same", "concurrent-prefix"} {
		t.Run(mode, func(t *testing.T) {
			fixture, store, selections, runner := p0Fixture(t)
			store.bump("work:b")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("p0 checkpoint failure")
			wantErr := error(dao.ErrShareExportChanged)
			switch mode {
			case "cancelled":
				selections.onSync = cancel
				wantErr = context.Canceled
			case "preparation-failed":
				selections.prepareErr = failure
				wantErr = failure
			case "commit-failed":
				store.commitErr = failure
				wantErr = failure
			case "concurrent-same":
				store.completeBump = true
			case "concurrent-prefix":
				store.beforeComplete = func() { store.bump("work:a") }
			}
			err := phase03Run(t, fixture, runner, "incremental", "p0-"+mode, ctx)
			if !errors.Is(err, wantErr) {
				t.Fatalf("错误丢失：%v", err)
			}
			_, metadata := p0Metadata(t, fixture, "p0-"+mode)
			if metadata["phase"] == "completed" || metadata["pending_works"] != float64(1) {
				t.Fatalf("取消/失败/并发变更被误报完成或隐藏待办：%v", metadata)
			}
			if mode != "concurrent-prefix" && store.acked["work:b"] != 0 {
				t.Fatal("未成功的作品被确认")
			}
		})
	}
}

func TestShareStrmP0FullMetricsRetainedOnSameTaskRecovery(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	full := runner.full
	runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
		if err := fixture.service.prepareSelectionWork(ctx, selections, ""); err != nil {
			return err
		}
		if err := fixture.service.tasks.UpdateProgressContext(ctx, id, 8, 8, 8, 0); err != nil {
			return err
		}
		if err := fixture.service.updateExportMetadata(ctx, id, map[string]interface{}{"written": 3, "exported_files": 3, "skipped_dedupe": 5}); err != nil {
			return err
		}
		if err := full(ctx, input, id); err != nil {
			return err
		}
		store.bump("work:a")
		return nil
	}
	store.commitErr = errors.New("消费中断")
	if err := phase03Run(t, fixture, runner, "reconciliation", "p0-recovery", context.Background()); !errors.Is(err, store.commitErr) {
		t.Fatal(err)
	}
	store.commitErr = nil
	for directory := fixture.cfg.OutputPath; ; directory = filepath.Dir(directory) {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		if filepath.Dir(directory) == directory {
			break
		}
	}
	fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock\\(hashtextextended\\('share:default',34982\\)\\)").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := runner.Run(context.Background(), "reconciliation", "p0-recovery"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	task, metadata := p0Metadata(t, fixture, "p0-recovery")
	if store.fulls != 1 || store.completions != 1 || len(selections.syncWorks) != 2 || task["processed_files"] != float64(8) || metadata["written"] != float64(3) || metadata["exported_files"] != float64(3) || metadata["skipped_dedupe"] != float64(5) || metadata["phase"] != "completed" {
		t.Fatalf("同任务恢复重复全量/丢失统计或待办：task=%v sync=%v", task, selections.syncWorks)
	}
}

func TestShareStrmP0RoundsBoundContinuousDirtyTail(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.bump("work:00000")
	store.beforeComplete = func() {
		store.bump(fmt.Sprintf("work:%05d", store.completions+1))
		if store.completions >= 1000 {
			cancel()
		}
	}
	err := phase03Run(t, fixture, runner, "incremental", "p0-bound", ctx)
	if !errors.Is(err, dao.ErrShareExportChanged) || store.completions > 1000 {
		t.Fatalf("连续尾部入队未有界返回并保留待办：err=%v completions=%d", err, store.completions)
	}
}

func TestShareStrmP0UnrelatedInvalidManualSelectionIsIsolated(t *testing.T) {
	for _, mode := range []string{"incremental", "reconciliation"} {
		t.Run(mode, func(t *testing.T) {
			fixture, store, selections, runner := p0Fixture(t)
			if mode == "reconciliation" {
				store.input.BaselineState = "building"
			}
			store.bump("work:target")
			// 手选不可用属于解析语义，SQL 候选同步不校验无关手选。
			selections.onResolveWork = func(work string) error {
				if work != "work:target" {
					return dao.ErrShareSelectionUnavailable
				}
				return nil
			}
			if err := phase03Run(t, fixture, runner, mode, "p0-isolation", context.Background()); err != nil {
				t.Fatalf("无关手选失效不应阻止目标消费：%v", err)
			}
			if !reflect.DeepEqual(selections.syncWorks, []string{""}) || !reflect.DeepEqual(selections.resolveWorks, []string{"work:target"}) || store.fulls != 0 {
				t.Fatalf("增量/恢复扩大手选解析范围：sync=%v resolve=%v", selections.syncWorks, selections.resolveWorks)
			}
		})
	}
}

func TestShareStrmP0PreparationNewPrefixRemainsPending(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	store.bump("work:b")
	selections.onSync = func() {
		store.bump("work:b")
		store.bump("work:a")
	}
	err := phase03Run(t, fixture, runner, "incremental", "p0-new-prefix", context.Background())
	if !errors.Is(err, dao.ErrShareExportChanged) || store.acked["work:b"] != 2 || store.acked["work:a"] != 0 {
		t.Fatalf("刷新只能确认同作品；游标前新作品需留待下次：err=%v ack=%v", err, store.acked)
	}
}

func TestShareStrmP0ReportsProgressDuringPreparation(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	for index := 0; index < 20; index++ {
		store.bump(fmt.Sprintf("work:%02d", index))
	}
	selections.onResolveWork = func(work string) error {
		if work == "work:10" {
			_, metadata := p0Metadata(t, fixture, "p0-small-progress")
			if metadata["consumed_works"] != float64(10) || metadata["round_consumed_works"] != float64(10) {
				t.Fatalf("准备期间必须按作品或小批报告进度：%v", metadata)
			}
		}
		return nil
	}
	if err := phase03Run(t, fixture, runner, "incremental", "p0-small-progress", context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShareStrmP0FullSourceChangeCannotAckEmptyOutputs(t *testing.T) {
	fixture, store, selections, runner := p0Fixture(t)
	full := runner.full
	runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
		if err := fixture.service.prepareSelectionWork(ctx, selections, ""); err != nil {
			return err
		}
		if err := full(ctx, input, id); err != nil {
			return err
		}
		source := phaseSource(7)
		source.Result.MediaType = "tv"
		source.Episodes = nil
		source.FileVersion++
		fixture.store.sources = []domain.ShareStrmSource{source}
		store.bump(source.WorkKey)
		return nil
	}
	err := phase03Run(t, fixture, runner, "reconciliation", "p0-full-empty-change", context.Background())
	_, metadata := p0Metadata(t, fixture, "p0-full-empty-change")
	if !errors.Is(err, dao.ErrShareExportChanged) || store.acked[fixture.store.sources[0].WorkKey] != 0 || metadata["phase"] == "completed" || len(selections.syncWorks) != 1 {
		t.Fatalf("全量准备后的来源变化被空输出确认：err=%v ack=%v metadata=%v sync=%v", err, store.acked, metadata, selections.syncWorks)
	}
}

func TestShareStrmP0ChangedSourceBetweenObservationAndConsumption(t *testing.T) {
	for _, mode := range []string{"incremental", "reconciliation"} {
		t.Run(mode, func(t *testing.T) {
			fixture, store, selections, runner := p0Fixture(t)
			source := phaseSource(7)
			fixture.store.sources = []domain.ShareStrmSource{source}
			store.bump(source.WorkKey)
			if mode == "reconciliation" {
				full := runner.full
				runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
					if err := fixture.service.prepareSelectionWork(ctx, selections, ""); err != nil {
						return err
					}
					return full(ctx, input, id)
				}
			}
			store.afterObserve = func(string) {
				fixture.store.sources[0].FileVersion++
				store.afterObserve = nil
			}
			err := phase03Run(t, fixture, runner, mode, "p0-observed-change", context.Background())
			if !errors.Is(err, dao.ErrShareExportChanged) || store.acked[source.WorkKey] != 0 || len(selections.syncWorks) != 1 {
				t.Fatalf("消费不应使用与观察不符的来源：err=%v ack=%v sync=%v", err, store.acked, selections.syncWorks)
			}
		})
	}
}

func TestShareStrmP0CompletionBoundaryAllowsConcurrentRequeue(t *testing.T) {
	fixture, store, _, runner := p0Fixture(t)
	store.afterFinish = func() { store.bump("work:next") }
	if err := phase03Run(t, fixture, runner, "incremental", "p0-boundary", context.Background()); err != nil {
		t.Fatal(err)
	}
	_, metadata := p0Metadata(t, fixture, "p0-boundary")
	if metadata["phase"] != "completed" || metadata["pending_works"] != float64(1) || metadata["completion_boundary"] != "finish_baseline_cas" || store.acked["work:next"] != 0 {
		t.Fatalf("完成指 CAS 边界快照，不应隐藏后来入队：%v", metadata)
	}
}

func TestShareStrmP0FinalProbeFailureDoesNotPersistCompleted(t *testing.T) {
	fixture, store, _, runner := p0Fixture(t)
	failure := errors.New("最终进度探测失败")
	store.afterFinish = func() { store.probeErr = failure }
	err := phase03Run(t, fixture, runner, "incremental", "p0-persist-failure", context.Background())
	_, metadata := p0Metadata(t, fixture, "p0-persist-failure")
	if !errors.Is(err, failure) || metadata["phase"] != "failed" || metadata["pending_works_exact"] != false || metadata["pending_probe_failed"] != true {
		t.Fatalf("状态持久化失败不能报告 completed：err=%v metadata=%v", err, metadata)
	}
}

type p0PersistFailureHook struct {
	armed   atomic.Bool
	mode    string
	failure error
}

func (hook *p0PersistFailureHook) taskPhase(command redis.Cmder) string {
	if command.Name() != "set" || len(command.Args()) < 3 {
		return ""
	}
	var task struct {
		Metadata map[string]interface{} `json:"metadata"`
	}
	var raw []byte
	switch value := command.Args()[2].(type) {
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return ""
	}
	if err := json.Unmarshal(raw, &task); err != nil {
		return ""
	}
	phase, _ := task.Metadata["phase"].(string)
	return phase
}

func (hook *p0PersistFailureHook) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (hook *p0PersistFailureHook) AfterProcess(_ context.Context, command redis.Cmder) error {
	phase := hook.taskPhase(command)
	if hook.armed.Load() && (hook.mode == "progress" && phase == "consuming" || hook.mode == "metadata" && phase == "completed") && hook.armed.CompareAndSwap(true, false) {
		return hook.failure
	}
	return nil
}

func (hook *p0PersistFailureHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (hook *p0PersistFailureHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestShareStrmP0FinalRedisFailureDoesNotLeaveCompleted(t *testing.T) {
	for _, mode := range []string{"progress", "metadata"} {
		t.Run(mode, func(t *testing.T) {
			fixture, store, _, runner := p0Fixture(t)
			client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
			t.Cleanup(func() { client.Close() })
			hook := &p0PersistFailureHook{mode: mode, failure: errors.New("最终 Redis 写入失败")}
			client.AddHook(hook)
			fixture.service.tasks = NewTaskService(dao.NewTaskRedisDAO(client))
			store.afterFinish = func() { hook.armed.Store(true) }
			err := phase03Run(t, fixture, runner, "incremental", "p0-redis-failure", context.Background())
			_, metadata := p0Metadata(t, fixture, "p0-redis-failure")
			if err == nil || !strings.Contains(err.Error(), hook.failure.Error()) || metadata["phase"] != "failed" {
				t.Fatalf("持久化成功后响应失败也需降级终态：err=%v metadata=%v", err, metadata)
			}
		})
	}
}

func TestShareStrmP0SourceObservationComparison(t *testing.T) {
	original := domain.ShareExportSourceState{SourceFileID: 1, ShareID: 2, MediaID: 3, WorkKey: "work", FileVersion: 4, ShareVersion: 5, Available: true, EpisodeSignature: "1:1", State: "active"}
	for _, field := range []string{"SourceFileID", "ShareID", "MediaID", "WorkKey", "FileVersion", "ShareVersion", "Available", "Cancelled", "EpisodeSignature", "State"} {
		t.Run(field, func(t *testing.T) {
			changed := original
			value := reflect.ValueOf(&changed).Elem().FieldByName(field)
			switch value.Kind() {
			case reflect.Int:
				value.SetInt(value.Int() + 1)
			case reflect.String:
				value.SetString(value.String() + "-changed")
			case reflect.Bool:
				value.SetBool(!value.Bool())
			}
			if sameShareExportSourceStates([]domain.ShareExportSourceState{original}, []domain.ShareExportSourceState{changed}) {
				t.Fatalf("来源观察字段 %s 变化未被拒绝", field)
			}
		})
	}
	withKeys := original
	withKeys.ExportKeys = []string{"output"}
	if !sameShareExportSourceStates([]domain.ShareExportSourceState{original}, []domain.ShareExportSourceState{withKeys}) || sameShareExportSourceStates(nil, []domain.ShareExportSourceState{original}) {
		t.Fatal("输出关联不属于来源观察，来源增删必须拒绝")
	}
}

func TestShareStrmP0ProgressEventIncludesRoundDelta(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "progress-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { output.Close() })
	logger.SetFormat("json")
	logger.SetOutputs(output, output, output, output)
	t.Cleanup(func() {
		logger.SetFormat("")
		logger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr)
	})
	fixture, store, _, runner := p0Fixture(t)
	store.pageSize = 2
	for _, work := range []string{"work:a", "work:b", "work:c"} {
		store.bump(work)
	}
	if err := phase03Run(t, fixture, runner, "incremental", "p0-progress-events", context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	var totals, deltas []int
	for _, line := range bytes.Split(raw, []byte("\n")) {
		var entry struct {
			Fields map[string]interface{} `json:"fields"`
		}
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Fields["event"] != "consumption_progress" {
			continue
		}
		count, ok := entry.Fields["round_consumed_works"].(float64)
		if !ok {
			t.Fatalf("进度事件缺少本轮确认数：%s", line)
		}
		deltas = append(deltas, int(count))
		totals = append(totals, int(entry.Fields["consumed_works"].(float64)))
	}
	if !reflect.DeepEqual(totals, []int{2, 3, 3}) || !reflect.DeepEqual(deltas, []int{2, 1, 1}) {
		t.Fatalf("事件的累计值与本轮增量混淆：totals=%v deltas=%v", totals, deltas)
	}
}
