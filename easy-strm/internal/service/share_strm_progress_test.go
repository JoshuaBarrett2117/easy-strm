package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type shareProgressFaultHook struct {
	failProgress, failMetadata         bool
	progressAttempts, metadataAttempts int
}

func (hook *shareProgressFaultHook) BeforeProcess(ctx context.Context, command redis.Cmder) (context.Context, error) {
	if command.Name() != "set" {
		return ctx, nil
	}
	callers := make([]uintptr, 32)
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers)])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, "(*TaskRedisDAO).UpdateProgress") {
			hook.progressAttempts++
			if hook.failProgress {
				return ctx, errors.New("进度持久化故障")
			}
			break
		}
		if strings.HasSuffix(frame.Function, "(*TaskRedisDAO).UpdateMetadata") {
			hook.metadataAttempts++
			if hook.failMetadata && hook.metadataAttempts > 1 {
				return ctx, errors.New("元数据持久化故障")
			}
			break
		}
		if !more {
			break
		}
	}
	return ctx, nil
}

func (hook *shareProgressFaultHook) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (hook *shareProgressFaultHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (hook *shareProgressFaultHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func shareProgressFaultTasks(t *testing.T, hook *shareProgressFaultHook) *TaskService {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { client.Close() })
	client.AddHook(hook)
	return NewTaskService(dao.NewTaskRedisDAO(client))
}

func TestShareStrmPhase01BatchedPersistence(t *testing.T) {
	for _, count := range []int{0, 1, 99, 100, 101, 205} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			fixture := newPhaseFixture(t, true)
			for id := 1; id <= count; id++ {
				fixture.store.sources = append(fixture.store.sources, phaseSource(id))
			}
			result := fixture.round("batch", domain.ShareLibraryQuery{}, true)
			batches := (count + shareStrmProgressBatchSize - 1) / shareStrmProgressBatchSize
			if result.Counts.ProgressWrites != batches || result.Counts.MetadataWrites != batches+1 || result.Processed != count || result.Added != count || result.Error != "" {
				t.Fatalf("批次边界%d: %+v", count, result)
			}
		})
	}
}

func TestShareStrmPhase01FinalPersistenceFailureDoesNotFinishSnapshot(t *testing.T) {
	for _, scenario := range []string{"progress", "metadata", "both"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPhaseFixture(t, true)
			hook := &shareProgressFaultHook{failProgress: scenario != "metadata", failMetadata: scenario != "progress"}
			fixture.service.tasks = shareProgressFaultTasks(t, hook)
			fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
			result := fixture.round("persistence_failure", domain.ShareLibraryQuery{}, false)
			if result.Error == "" || len(result.Stale) != 0 || hook.progressAttempts != 1 || hook.metadataAttempts != 2 {
				t.Fatalf("失败收尾或重复刷新: %+v / %+v", result, hook)
			}
			if hook.failProgress && !strings.Contains(result.Error, "进度持久化故障") {
				t.Fatal(result.Error)
			}
			if hook.failMetadata && !strings.Contains(result.Error, "元数据持久化故障") {
				t.Fatal(result.Error)
			}
			if !hook.failProgress && (result.Processed != 1 || result.Success != 1) {
				t.Fatal("成功进度丢失")
			}
			if !hook.failMetadata && result.Added != 1 {
				t.Fatal("成功元数据丢失")
			}
		})
	}
}

type shareExportFaultStore struct {
	*strmMemory
	readErr error
	cancel  context.CancelFunc
}

func (store *shareExportFaultStore) StrmSources(ctx context.Context, query domain.ShareLibraryQuery, after int) ([]domain.ShareStrmSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if after > 0 && len(store.entries) > 0 && store.readErr != nil {
		return nil, store.readErr
	}
	return store.strmMemory.StrmSources(ctx, query, after)
}

func TestShareStrmPhase01LastSourceTaskFlagCancellationCanFinishSnapshot(t *testing.T) {
	fixture := newPhaseFixture(t, true)
	fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
	fixture.store.cancelAfter, fixture.store.flagCancel = 1, true
	result := fixture.round("last_source_cancel", domain.ShareLibraryQuery{}, true)
	cancelled := fixture.service.tasks.IsCancelled("last_source_cancel")
	if result.Error != "" || result.Processed != 1 || result.Success != 1 || result.Added != 1 || !cancelled {
		t.Fatalf("保留原版最后一条任务标记取消仍可完成导出的行为: %+v / cancelled=%v", result, cancelled)
	}
	if len(result.Stale) != 1 || result.Stale[0] != "sentinel" || result.Counts.Deletes != 0 {
		t.Fatalf("保留原版快照收尾标记旧状态但不删除文件的行为: %+v", result)
	}
	t.Logf("原版边界保留: cancelled=%v error=%q processed=%d added=%d stale=%v deletes=%d", cancelled, result.Error, result.Processed, result.Added, result.Stale, result.Counts.Deletes)
}

func (store *shareExportFaultStore) SaveExportedStrmFile(ctx context.Context, file domain.StrmFile) error {
	if store.cancel != nil {
		store.cancel()
	}
	return store.strmMemory.SaveExportedStrmFile(ctx, file)
}

func TestShareStrmPhase01ExitErrorsArePreserved(t *testing.T) {
	for _, scenario := range []string{"read", "cancel", "source_failure"} {
		t.Run(scenario, func(t *testing.T) {
			service, memory, _ := strmFixture(t)
			hook := &shareProgressFaultHook{failProgress: true, failMetadata: true}
			service.tasks = shareProgressFaultTasks(t, hook)
			if err := service.tasks.Create("fault", "strm_generate", "受控退出"); err != nil {
				t.Fatal(err)
			}
			memory.sources = []domain.ShareStrmSource{phaseSource(1)}
			memory.entries = map[string]domain.ShareStrmEntry{}
			store := &shareExportFaultStore{strmMemory: memory}
			ctx := context.Background()
			original := errors.New("来源分页故障")
			switch scenario {
			case "read":
				store.readErr = original
			case "cancel":
				ctx, store.cancel = context.WithCancel(ctx)
				defer store.cancel()
				original = context.Canceled
			case "source_failure":
				memory.fileErr = errors.New("文件清单故障")
			}
			service.store = store
			cfg, err := service.Settings()
			if err != nil {
				t.Fatal(err)
			}
			err = service.export(ctx, cfg, domain.ShareLibraryQuery{}, "fault")
			if scenario != "source_failure" && !errors.Is(err, original) {
				t.Fatalf("原始退出错误丢失: %v", err)
			}
			if scenario == "source_failure" && !strings.Contains(err.Error(), "1个本地记录失败") {
				t.Fatalf("来源失败摘要丢失: %v", err)
			}
			if err == nil || !strings.Contains(err.Error(), "进度持久化故障") || !strings.Contains(err.Error(), "元数据持久化故障") || hook.progressAttempts != 1 || hook.metadataAttempts != 2 {
				t.Fatalf("刷新错误未合并或重复提交: %v / %+v", err, hook)
			}
		})
	}
}
