package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type incrementalMemory struct {
	fixture                     *phaseFixture
	input                       domain.ShareExportInput
	dirty                       map[string]domain.ShareExportDirty
	acked                       map[string]int64
	states                      map[int]domain.ShareExportSourceState
	reads, completions, fanouts int
	planErr, commitErr          error
	completeBump                bool
	afterBatch                  func()
	beforeComplete              func()
	observations                map[string][]domain.ShareExportSourceState
}

func (store *incrementalMemory) CheckSchema(context.Context) error { return nil }
func (store *incrementalMemory) ReadInput(context.Context) (domain.ShareExportInput, error) {
	return store.input, nil
}
func (store *incrementalMemory) RequireBaseline(context.Context) error {
	store.input.ConfigRevision++
	store.input.BaselineState = "required"
	return nil
}
func (store *incrementalMemory) MarkLegacyReconciled(_ context.Context, revision int64) error {
	if revision != store.input.ConfigRevision {
		return dao.ErrShareExportChanged
	}
	store.input.LegacyOutputsReconciled = true
	return nil
}
func (store *incrementalMemory) bump(key string) {
	dirty := store.dirty[key]
	dirty.WorkKey = key
	dirty.Revision++
	store.dirty[key] = dirty
}
func (store *incrementalMemory) Prepare(_ context.Context, input domain.ShareExportInput, fingerprint string) error {
	if input.ConfigRevision != store.input.ConfigRevision {
		return dao.ErrShareExportChanged
	}
	if store.input.PreparedRevision != input.ConfigRevision || store.input.Fingerprint != fingerprint || store.input.BaselineState == "required" {
		keys := map[string]bool{}
		for _, source := range store.fixture.store.sources {
			keys[source.WorkKey] = true
		}
		for _, state := range store.states {
			keys[state.WorkKey] = true
		}
		for key := range store.dirty {
			keys[key] = true
		}
		for key := range keys {
			store.bump(key)
		}
		store.fanouts++
		store.input.PreparedRevision = input.ConfigRevision
		store.input.Fingerprint = fingerprint
		store.input.BaselineState = "building"
	}
	return nil
}
func (store *incrementalMemory) DirtyBatch(_ context.Context, attempted []string) ([]domain.ShareExportDirty, error) {
	if store.afterBatch != nil {
		store.afterBatch()
	}
	excluded := map[string]bool{}
	for _, key := range attempted {
		excluded[key] = true
	}
	rows := []domain.ShareExportDirty{}
	for key, dirty := range store.dirty {
		if dirty.Revision > store.acked[key] && !excluded[key] {
			rows = append(rows, dirty)
		}
	}
	sort.Slice(rows, func(first, second int) bool { return rows[first].WorkKey < rows[second].WorkKey })
	return rows, nil
}
func (store *incrementalMemory) ObserveWork(_ context.Context, key string) ([]domain.ShareExportSourceState, error) {
	store.reads++
	if rows, ok := store.observations[key]; ok {
		return append([]domain.ShareExportSourceState{}, rows...), nil
	}
	rows := []domain.ShareExportSourceState{}
	for _, source := range store.fixture.store.sources {
		if source.WorkKey == key {
			rows = append(rows, domain.ShareExportSourceState{SourceFileID: source.ID, ShareID: source.ShareID, MediaID: source.MediaID, WorkKey: key, FileVersion: source.FileVersion, ShareVersion: source.ShareVersion, Available: true, State: "active"})
		}
	}
	return rows, nil
}
func (store *incrementalMemory) WorkKeys(_ context.Context, key string) ([]string, error) {
	keys := append([]string{}, store.dirty[key].PendingKeys...)
	for _, state := range store.states {
		if state.WorkKey == key {
			keys = append(keys, state.ExportKeys...)
		}
	}
	return sortedIncrementalKeys(keys), nil
}
func sortedIncrementalKeys(keys []string) []string {
	set := map[string]bool{}
	for _, key := range keys {
		set[key] = true
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func (store *incrementalMemory) TargetSnapshot(_ context.Context, keys []string) (dao.ExportSnapshot, error) {
	snapshot := dao.ExportSnapshot{}
	for _, key := range keys {
		if output, ok := store.fixture.states[key]; ok {
			snapshot[key] = output.Run
		}
	}
	return snapshot, nil
}
func (store *incrementalMemory) RecordPlannedKeys(_ context.Context, key string, revision int64, keys []string) error {
	if store.planErr != nil {
		return store.planErr
	}
	dirty := store.dirty[key]
	if dirty.Revision != revision || revision == store.acked[key] {
		return dao.ErrShareExportChanged
	}
	dirty.PendingKeys = sortedIncrementalKeys(append(dirty.PendingKeys, keys...))
	store.dirty[key] = dirty
	return nil
}
func (store *incrementalMemory) CompleteWork(_ context.Context, dirty domain.ShareExportDirty, revision int64, run string, states []domain.ShareExportSourceState, snapshot dao.ExportSnapshot) (int, error) {
	if store.beforeComplete != nil {
		store.beforeComplete()
	}
	if store.completeBump {
		store.bump(dirty.WorkKey)
	}
	if store.commitErr != nil {
		return 0, store.commitErr
	}
	if revision != store.input.ConfigRevision || dirty.Revision != store.dirty[dirty.WorkKey].Revision {
		return 0, dao.ErrShareExportChanged
	}
	newKeys := map[string]bool{}
	for sourceID, state := range store.states {
		if state.WorkKey == dirty.WorkKey {
			state.State = "stale"
			store.states[sourceID] = state
		}
	}
	for _, state := range states {
		if state.State == "stale" {
			state.ExportKeys = store.states[state.SourceFileID].ExportKeys
		}
		store.states[state.SourceFileID] = state
		if state.State == "active" {
			for _, key := range state.ExportKeys {
				newKeys[key] = true
			}
		}
	}
	marked := 0
	for key, previous := range snapshot {
		if !newKeys[key] && store.fixture.states[key].Run == previous {
			store.fixture.stale[key] = true
			marked++
		}
	}
	store.acked[dirty.WorkKey] = dirty.Revision
	current := store.dirty[dirty.WorkKey]
	current.PendingKeys = nil
	store.dirty[dirty.WorkKey] = current
	store.completions++
	return marked, nil
}
func (store *incrementalMemory) FinishBaseline(_ context.Context, revision int64) (bool, error) {
	if revision != store.input.ConfigRevision {
		return false, nil
	}
	for key, dirty := range store.dirty {
		if dirty.Revision > store.acked[key] {
			return false, nil
		}
	}
	store.input.BaselineState = "ready"
	return true, nil
}

func newIncrementalFixture(t *testing.T) (*phaseFixture, *incrementalMemory, *ShareStrmIncrementalService) {
	t.Helper()
	fixture := newPhaseFixture(t, true)
	fixture.db.SetMaxOpenConns(2)
	fixture.service.tasks = nil
	fixture.cfg.Cloud115ID = 1
	fixture.cfg.TransferPath = "/synthetic"
	store := &incrementalMemory{fixture: fixture, input: domain.ShareExportInput{Settings: fixture.cfg, ConfigRevision: 1, PreparedRevision: 1, BaselineState: "ready", ProtocolVersion: 1, LegacyOutputsReconciled: true}, dirty: map[string]domain.ShareExportDirty{}, acked: map[string]int64{}, states: map[int]domain.ShareExportSourceState{}}
	store.input.Fingerprint, _ = shareExportFingerprint(store.input)
	return fixture, store, NewShareStrmIncrementalService(fixture.service, store)
}

func runIncrementalFixture(t *testing.T, fixture *phaseFixture, service *ShareStrmIncrementalService, id string) error {
	t.Helper()
	input, inputErr := service.checkpoints.ReadInput(context.Background())
	if inputErr != nil {
		t.Fatal(inputErr)
	}
	fingerprint, _ := shareExportFingerprint(input)
	if !input.LegacyOutputsReconciled || input.BaselineState != "ready" || input.PreparedRevision != input.ConfigRevision || input.Fingerprint != fingerprint {
		err := service.Run(context.Background(), id)
		if expectationErr := fixture.mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
		return err
	}
	for directory := fixture.cfg.OutputPath; ; directory = filepath.Dir(directory) {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		if filepath.Dir(directory) == directory {
			break
		}
	}
	fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock\\(hashtextextended\\('share:default',34982\\)\\)").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	ctx := context.Background()
	if fixture.store.cancelAfter > 0 {
		ctx, fixture.store.cancel = context.WithCancel(ctx)
		defer fixture.store.cancel()
	}
	err := service.Run(ctx, id)
	if expectationErr := fixture.mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
	if fixture.db.Stats().InUse != 0 {
		t.Fatalf("增量连接未释放：%+v", fixture.db.Stats())
	}
	return err
}

// TestShareStrmIncrementalDedupeCountsAndRecovery 验证淘汰来源不中断增量、逐集映射选优键，登记失败保留计数并可恢复。
func TestShareStrmIncrementalDedupeCountsAndRecovery(t *testing.T) {
	for _, reread := range []bool{false, true} {
		for _, failRegistration := range []bool{false, true} {
			t.Run(fmt.Sprintf("reread=%t/fail-registration=%t", reread, failRegistration), func(t *testing.T) {
				fixture, checkpoints, service := newIncrementalFixture(t)
				client := setupTaskRedisMock(t)
				t.Cleanup(func() { _ = client.Close() })
				fixture.service.tasks = NewTaskService(dao.NewTaskRedisDAO(client))
				loser := filesystemSource(11001, 501, 100, filesystemEpisode(1, 1), filesystemEpisode(2, 1), filesystemEpisode(2, 1))
				mixed := filesystemSource(11002, 502, 300, filesystemEpisode(1, 1), filesystemEpisode(1, 2), filesystemEpisode(2, 1), filesystemEpisode(2, 2))
				winner := filesystemSource(11003, 503, 400, filesystemEpisode(1, 1), filesystemEpisode(2, 1))
				loser.MediaID, mixed.MediaID, winner.MediaID = 8801, 8802, 8803
				fixture.store.sources = []domain.ShareStrmSource{loser, mixed, winner}
				memory := &strmMemory{sources: fixture.store.sources, entries: map[string]domain.ShareStrmEntry{}}
				fault := &filesystemFaultStore{strmMemory: memory, failure: errors.New("增量清单登记故障")}
				if failRegistration {
					fault.failAt = 2
				}
				fixture.service.store = fault
				if reread {
					fixture.service.store = filesystemFaultReader{filesystemFaultStore: fault}
				}
				fixture.cfg.DedupeExport = true
				checkpoints.input.Settings = fixture.cfg
				checkpoints.input.Fingerprint, _ = shareExportFingerprint(checkpoints.input)
				checkpoints.bump(loser.WorkKey)
				want := map[string][]byte{filesystemPath(1, 2, ""): filesystemContent(mixed), filesystemPath(2, 2, ""): filesystemContent(mixed)}
				run := func(id string, written, skipped int, wantFailure bool) {
					t.Helper()
					fixture.expectPaths(domain.ShareStrmEntry{Episodes: []domain.ShareEpisode{filesystemEpisode(1, 2), filesystemEpisode(2, 2)}})
					if !wantFailure {
						fixture.expectPaths(domain.ShareStrmEntry{Episodes: winner.Episodes})
					}
					if err := fixture.service.tasks.Create(id, "strm_generate", "增量去重回归"); err != nil {
						t.Fatal(err)
					}
					connection, err := fixture.db.Conn(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					output := &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: fixture.cfg.OutputPath, Owner: "share:default", Run: id, Paths: map[string]bool{}}
					err = service.runPrepared(context.Background(), id, checkpoints.input, checkpoints.input.Fingerprint, output)
					if closeErr := connection.Close(); closeErr != nil {
						t.Fatal(closeErr)
					}
					if expectationErr := fixture.mock.ExpectationsWereMet(); expectationErr != nil {
						t.Fatalf("SQL模拟未消费：%v；导出错误：%v", expectationErr, err)
					}
					if wantFailure != (err != nil) || wantFailure && !errors.Is(err, fault.failure) {
						t.Fatalf("增量导出错误不符合预期：%v", err)
					}
					task, err := fixture.service.tasks.Get(id)
					if err != nil {
						t.Fatal(err)
					}
					metadata := task["metadata"].(map[string]interface{})
					assertFilesystemMetadata(t, metadata, written, skipped)
					pending, affected := float64(0), float64(3)
					if wantFailure {
						pending, affected = 1, 2
						if checkpoints.completions != 0 || len(checkpoints.dirty[loser.WorkKey].PendingKeys) != 2 {
							t.Fatal("失败任务提交了检查点或丢失计划键")
						}
					}
					if metadata["pending_works"] != pending || metadata["affected_sources"] != affected || task["failed_files"] != pending {
						t.Fatalf("普通去重被误计为失败，或失败来源计数丢失：%v / %v", task, metadata)
					}
				}
				if failRegistration {
					run("registration-failure", 2, 4, true)
					assertFilesystemCollection(t, fixture.cfg.OutputPath, want)
					fault.failAt = 0
				}
				written := 4
				if failRegistration {
					written = 2
				}
				run("complete", written, 4, false)
				want[filesystemPath(1, 1, "")] = filesystemContent(winner)
				want[filesystemPath(2, 1, "")] = filesystemContent(winner)
				assertFilesystemCollection(t, fixture.cfg.OutputPath, want)
				wantKeys := map[int][]string{
					loser.ID:  {"8803:1:1", "8803:2:1"},
					mixed.ID:  {"8803:1:1", "8802:1:2", "8803:2:1", "8802:2:2"},
					winner.ID: {"8803:1:1", "8803:2:1"},
				}
				for sourceID, keys := range wantKeys {
					if got := checkpoints.states[sourceID].ExportKeys; !reflect.DeepEqual(got, keys) {
						t.Fatalf("来源%d没有映射实际选优键：got=%v want=%v", sourceID, got, keys)
					}
				}
				if checkpoints.completions != 1 || len(checkpoints.dirty[loser.WorkKey].PendingKeys) != 0 {
					t.Fatal("去重后没有完成并清理作品检查点")
				}
				checkpoints.bump(loser.WorkKey)
				run("unchanged", 0, 4, false)
				assertFilesystemCollection(t, fixture.cfg.OutputPath, want)
			})
		}
	}
}

type filesystemFaultReader struct{ *filesystemFaultStore }

// GetStrmSource 为登记故障模拟叠加内存重读能力，覆盖生产导出的来源重读分支。
func (store filesystemFaultReader) GetStrmSource(ctx context.Context, id int) (domain.ShareStrmSource, error) {
	return (filesystemReader{store.strmMemory}).GetStrmSource(ctx, id)
}

func TestShareStrmIncrementalInitialBaselineRequiresExplicitFull(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "trusted", true: "unregistered"}[unknown], func(t *testing.T) {
			fixture, store, service := newIncrementalFixture(t)
			source := phaseSource(1)
			fixture.store.sources = []domain.ShareStrmSource{source}
			store.input.LegacyOutputsReconciled = false
			store.input.PreparedRevision = 0
			store.input.BaselineState = "required"
			fixture.run = "initial-baseline"
			fixture.finish = !unknown
			fixture.states["old-sentinel"] = dao.ExportState{Owner: "share:default", Key: "old-sentinel", Run: "previous"}
			if unknown {
				current := source
				current.Result.SeasonNumber, current.Result.EpisodeNumber = 1, 1
				relative, err := fixture.service.strmRelativePath(current, domain.ShareFileInfo{Path: source.FileName}, nil)
				if err != nil {
					t.Fatal(err)
				}
				outputPath := filepath.Join(fixture.cfg.OutputPath, relative)
				if err = os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(outputPath, []byte("preserve\n"), 0644); err != nil {
					t.Fatal(err)
				}
				fixture.unregistered = true
			}
			err := runIncrementalFixture(t, fixture, service, fixture.run)
			if !errors.Is(err, dao.ErrShareExportBaselineRequired) || store.input.LegacyOutputsReconciled || store.completions != 0 || store.fanouts != 0 || fixture.stale["old-sentinel"] || fixture.entries != 0 {
				t.Fatalf("未拒绝未知基线或擅自调用旧全量：%v", err)
			}
		})
	}
}

func TestShareStrmIncrementalLastSourceCancellationRetainsPending(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.bump(source.WorkKey)
	fixture.store.cancelAfter = 1
	if err := runIncrementalFixture(t, fixture, service, "cancel"); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未传播：%v", err)
	}
	if store.completions != 0 || len(store.dirty[source.WorkKey].PendingKeys) != 1 || len(store.states) != 0 {
		t.Fatal("取消时错误确认或清除了 pending")
	}
	if err := runIncrementalFixture(t, fixture, service, "restart"); err != nil {
		t.Fatal(err)
	}
	if store.completions != 1 {
		t.Fatal("取消后无法恢复")
	}
}

func TestShareStrmIncrementalBuildingResumeDoesNotRepeatFanout(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.input.PreparedRevision = 0
	store.input.BaselineState = "required"
	store.planErr = errors.New("temporary planned-key failure")
	if err := runIncrementalFixture(t, fixture, service, "building"); err == nil {
		t.Fatal("忽略计划键失败")
	}
	if store.fanouts != 0 || store.input.BaselineState != "required" {
		t.Fatal("增量擅自创建基线")
	}
	store.planErr = nil
	store.input.BaselineState = "building"
	if err := runIncrementalFixture(t, fixture, service, "resume"); !errors.Is(err, dao.ErrShareExportBaselineRequired) {
		t.Fatalf("building 必须显式全量：%v", err)
	}
	if store.fanouts != 0 || store.input.BaselineState != "building" {
		t.Fatal("恢复期间重复全量 fanout")
	}
}

func TestShareStrmIncrementalTargetedNoChangeAndConflictRenaming(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	first := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{first, phaseSource(99)}
	store.bump(first.WorkKey)
	if err := runIncrementalFixture(t, fixture, service, "initial"); err != nil {
		t.Fatal(err)
	}
	if store.reads != 1 || store.completions != 1 || len(store.states) != 1 || fixture.counts.SourcePages != 0 {
		t.Fatalf("扫描了非目标来源：%+v", fixture.counts)
	}
	before := phaseFiles(t, fixture.cfg.OutputPath)
	reads, writes := fixture.counts.MemoryReads, fixture.counts.MemoryWrites
	if err := runIncrementalFixture(t, fixture, service, "no-change"); err != nil {
		t.Fatal(err)
	}
	if store.reads != 1 || fixture.counts.MemoryReads != reads || fixture.counts.MemoryWrites != writes {
		t.Fatal("稳定态读取了作品/来源")
	}
	for name, file := range phaseFiles(t, fixture.cfg.OutputPath) {
		if file.Modified != before[name].Modified {
			t.Fatal("稳定态改写文件")
		}
	}
	second := first
	second.ID, second.ShareID = 2, 2
	second.ShareName = "分享2"
	second.URL = "https://115.com/s/share2"
	fixture.store.sources = append(fixture.store.sources, second)
	store.bump(first.WorkKey)
	if err := runIncrementalFixture(t, fixture, service, "conflict"); err != nil {
		t.Fatal(err)
	}
	if fixture.stale["1:1:1"] || len(store.states) != 2 {
		t.Fatalf("旧键未 targeted stale：%v", fixture.stale)
	}
	if len(phaseFiles(t, fixture.cfg.OutputPath)) != 1 {
		t.Fatal("stale 文件被删除或冲突后缀未生成")
	}
	fixture.store.sources = []domain.ShareStrmSource{first}
	store.bump(first.WorkKey)
	if err := runIncrementalFixture(t, fixture, service, "restore"); err != nil {
		t.Fatal(err)
	}
	if fixture.stale["1:1:1"] || len(phaseFiles(t, fixture.cfg.OutputPath)) != 1 {
		t.Fatalf("恢复单源键不正确：%v", fixture.stale)
	}
}

func TestShareStrmIncrementalPendingSurvivesFailureCASAndRemap(t *testing.T) {
	for _, failure := range []string{"manifest", "commit", "concurrent"} {
		t.Run(failure, func(t *testing.T) {
			fixture, store, service := newIncrementalFixture(t)
			source := phaseSource(1)
			fixture.store.sources = []domain.ShareStrmSource{source}
			store.bump(source.WorkKey)
			if failure == "manifest" {
				fixture.store.fileErr = errors.New("manifest failed")
			}
			if failure == "commit" {
				store.commitErr = errors.New("commit failed")
			}
			if failure == "concurrent" {
				store.completeBump = true
			}
			if err := runIncrementalFixture(t, fixture, service, failure); err == nil {
				t.Fatal("失败被吞掉")
			}
			if len(store.dirty[source.WorkKey].PendingKeys) != 1 || store.acked[source.WorkKey] != 0 || len(store.states) != 0 {
				t.Fatal("半成功被错误确认/丢失计划键")
			}
			fixture.store.fileErr = nil
			store.commitErr = nil
			store.completeBump = false
			source.Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}}
			fixture.store.sources = []domain.ShareStrmSource{source}
			store.bump(source.WorkKey)
			if err := runIncrementalFixture(t, fixture, service, "retry"); err != nil {
				t.Fatal(err)
			}
			if !fixture.stale["1:1:1"] || len(store.dirty[source.WorkKey].PendingKeys) != 0 || len(phaseFiles(t, fixture.cfg.OutputPath)) != 2 {
				t.Fatal("重映射无法找回半成功键")
			}
		})
	}
}

func TestShareStrmIncrementalPlanCASStopsBeforeFiles(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.bump(source.WorkKey)
	store.planErr = dao.ErrShareExportChanged
	if err := runIncrementalFixture(t, fixture, service, "plan-CAS"); !errors.Is(err, dao.ErrShareExportChanged) {
		t.Fatalf("%v", err)
	}
	if len(phaseFiles(t, fixture.cfg.OutputPath)) != 0 || fixture.counts.MemoryWrites != 0 || store.completions != 0 {
		t.Fatal("计划未提交却执行文件副作用")
	}
}

func TestShareStrmIncrementalEmptyWorkInvalidationAndConfigBaseline(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.bump(source.WorkKey)
	if err := runIncrementalFixture(t, fixture, service, "initial"); err != nil {
		t.Fatal(err)
	}
	fixture.store.sources = nil
	store.bump(source.WorkKey)
	if err := runIncrementalFixture(t, fixture, service, "cancelled-source"); err != nil {
		t.Fatal(err)
	}
	if !fixture.stale["1:1:1"] || store.states[1].State != "stale" || len(phaseFiles(t, fixture.cfg.OutputPath)) != 1 {
		t.Fatal("空作品未失效或删除了磁盘文件")
	}
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.input.ConfigRevision++
	store.input.BaselineState = "required"
	store.input.Settings.BaseURL = "https://new.example.test"
	if err := runIncrementalFixture(t, fixture, service, "config"); !errors.Is(err, dao.ErrShareExportBaselineRequired) {
		t.Fatalf("配置变化必须显式全量：%v", err)
	}
	if store.fanouts != 0 {
		t.Fatal("增量配置失效不得播种")
	}
	fingerprint, _ := shareExportFingerprint(store.input)
	if err := store.Prepare(context.Background(), store.input, fingerprint); err != nil {
		t.Fatal(err)
	}
	store.input.BaselineState = "ready"
	if err := runIncrementalFixture(t, fixture, service, "explicit-baseline-fixture"); err != nil {
		t.Fatal(err)
	}
	if store.fanouts != 1 || store.input.BaselineState != "ready" {
		t.Fatal("配置没有原子建立基线")
	}
	for _, file := range phaseFiles(t, fixture.cfg.OutputPath) {
		if file.Content != "https://new.example.test/share-strm/"+fixture.states["1:1:1"].Playback+"\n" {
			t.Fatalf("配置未生效：%s", file.Content)
		}
	}
	if err := runIncrementalFixture(t, fixture, service, "resume-ready"); err != nil {
		t.Fatal(err)
	}
	if store.fanouts != 1 {
		t.Fatal("重复 seed 已完成基线")
	}
}

func TestShareStrmIncrementalUnregisteredFileIsNotClaimed(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.bump(source.WorkKey)
	source.Result.SeasonNumber = source.Episodes[0].SeasonNumber
	source.Result.EpisodeNumber = source.Episodes[0].EpisodeNumber
	relative, err := fixture.service.strmRelativePath(source, domain.ShareFileInfo{Path: source.FileName}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(fixture.cfg.OutputPath, relative)
	if err = os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(outputPath, []byte("unregistered\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fixture.unregistered = true
	if err = runIncrementalFixture(t, fixture, service, "foreign-file"); err == nil {
		t.Fatal("未知文件被接管")
	}
	raw, err := os.ReadFile(outputPath)
	if err != nil || string(raw) != "unregistered\n" || store.acked[source.WorkKey] != 0 {
		t.Fatalf("文件或待办不安全：%s %v", raw, err)
	}
}

func TestShareStrmIncrementalFingerprintChanges(t *testing.T) {
	input := domain.ShareExportInput{Settings: domain.ShareStrmSettings{OutputPath: "/synthetic", BaseURL: "https://example.test"}, Templates: map[string]string{"movie_naming_template": "original"}}
	first, err := shareExportFingerprint(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Templates["movie_naming_template"] = "changed"
	second, err := shareExportFingerprint(input)
	if err != nil || first == second {
		t.Fatal("模板未包含在保守失效指纹")
	}
	input.Categories = []*domain.MediaCategory{{ID: 1, Name: "synthetic", Enabled: true}}
	third, _ := shareExportFingerprint(input)
	if second == third {
		t.Fatal("分类未包含在失效指纹")
	}
	if first == "" {
		t.Fatal("空指纹")
	}
}
