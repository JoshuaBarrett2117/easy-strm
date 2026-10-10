package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestShareStrmIncrementalEmptyBatchCancellation 验证零待办时也不得发布取消任务的 ready。
func TestShareStrmIncrementalEmptyBatchCancellation(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	fixture.service.tasks = newPhaseFixture(t, true).service.tasks
	if err := fixture.service.tasks.Create("zero-cancel", "strm_generate", "zero-cancel"); err != nil {
		t.Fatal(err)
	}
	store.input.BaselineState = "ready"
	store.afterBatch = func() {
		if err := fixture.service.tasks.Cancel("zero-cancel"); err != nil {
			t.Fatal(err)
		}
	}
	if err := runIncrementalFixture(t, fixture, service, "zero-cancel"); !errors.Is(err, context.Canceled) {
		t.Fatalf("零待办取消未传播：%v", err)
	}
	if store.input.BaselineState != "ready" || store.completions != 0 || store.reads != 0 {
		t.Fatal("取消任务发布了 ready 或扫描了来源")
	}
}

// TestShareStrmIncrementalMappedSharedKeys 验证去重来源记录实际共享键，而非要求单独写盘。
func TestShareStrmIncrementalMappedSharedKeys(t *testing.T) {
	service, _, _ := strmFixture(t)
	first := phaseSource(1)
	second := first
	second.ID, second.MediaID = 2, 2
	attempt := &shareIncrementalAttempt{keys: map[int][]string{}, record: func(context.Context, string) error {
		t.Fatal("去重分支不应重复计划或写盘")
		return nil
	}}
	ctx := context.WithValue(context.Background(), shareIncrementalAttemptContext{}, attempt)
	seen := map[string]bool{shareStrmIdentity(first, first.Episodes[0]): true}
	for _, source := range []domain.ShareStrmSource{first, second} {
		result := service.exportLocalStrm(ctx, domain.ShareStrmSettings{OutputPath: t.TempDir()}, source, nil, seen, map[string]*shareStrmConflict{})
		if result.Err != nil || result.Written != 0 {
			t.Fatalf("共享键去重不正确：%+v", result)
		}
		if !reflect.DeepEqual(attempt.keys[source.ID], []string{"1:1:1"}) {
			t.Fatalf("source %d 未记录实际映射键：%v", source.ID, attempt.keys)
		}
	}
}

// TestShareStrmIncrementalWrittenSharedKeys 验证成功写盘后，去重来源仍记录首次写入的实际共享键。
func TestShareStrmIncrementalWrittenSharedKeys(t *testing.T) {
	service, store, _ := strmFixture(t)
	first := phaseSource(1)
	second := first
	second.ID, second.MediaID = 2, 2
	attempt := &shareIncrementalAttempt{keys: map[int][]string{}, record: func(context.Context, string) error {
		t.Fatal("普通写盘和去重分支不应计划数据库输出")
		return nil
	}}
	ctx := context.WithValue(context.Background(), shareIncrementalAttemptContext{}, attempt)
	cfg := domain.ShareStrmSettings{OutputPath: t.TempDir(), BaseURL: "https://media.example.test"}
	seen := map[string]bool{}
	result := service.exportLocalStrm(ctx, cfg, first, nil, seen, map[string]*shareStrmConflict{})
	if result.Err != nil || result.Written != 1 || len(store.files) != 1 {
		t.Fatalf("首次写盘失败：%+v，文件清单：%v", result, store.files)
	}
	var outputPath string
	for filePath := range store.files {
		outputPath = filePath
	}
	before, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	result = service.exportLocalStrm(ctx, cfg, second, nil, seen, map[string]*shareStrmConflict{})
	if result.Err != nil || result.Written != 0 || len(store.files) != 1 {
		t.Fatalf("共享键去重不正确：%+v，文件清单：%v", result, store.files)
	}
	after, err := os.ReadFile(outputPath)
	if err != nil || string(before) != string(after) {
		t.Fatalf("去重改写了文件：%v", err)
	}
	for _, source := range []domain.ShareStrmSource{first, second} {
		if !reflect.DeepEqual(attempt.keys[source.ID], []string{"1:1:1"}) {
			t.Fatalf("source %d 未记录实际映射键：%v", source.ID, attempt.keys)
		}
	}
}

// TestShareStrmFullRecoveryHookBoundary 保留完整、筛选、失败和末来源取消的原完成条件。
func TestShareStrmFullRecoveryHookBoundary(t *testing.T) {
	for _, scenario := range []string{"empty", "full", "filtered", "manifest-failure", "last-source-task-cancel", "revision-race"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPhaseFixture(t, true)
			fixture.recovery = true
			query := domain.ShareLibraryQuery{}
			finish := true
			if scenario != "empty" {
				fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
			}
			switch scenario {
			case "filtered":
				query.WorkKey, finish = phaseSource(1).WorkKey, false
			case "manifest-failure":
				fixture.store.fileErr, finish = errors.New("manifest-failure"), false
			case "last-source-task-cancel":
				fixture.store.cancelAfter, fixture.store.flagCancel = 1, true
			case "revision-race":
				fixture.recoveryRace = true
			}
			result := fixture.round(scenario, query, finish)
			wantRecovery := finish && scenario != "revision-race"
			if fixture.recovered != wantRecovery {
				t.Fatalf("凭证条件错误：%+v", result)
			}
			if finish && len(result.Stale) == 0 {
				t.Fatal("原 FinishSnapshot 没有执行")
			}
			if !finish && len(result.Stale) != 0 {
				t.Fatal("筛选或失败错误执行 FinishSnapshot")
			}
			if (scenario == "manifest-failure" || scenario == "revision-race") != (result.Error != "") {
				t.Fatalf("错误传播与原成功边界不一致：%+v", result)
			}
		})
	}
}

// TestShareStrmIncrementalOldFullInterruptedRemap 验证旧全量半成功的 K2 在增量重映射 K3 时可找回。
func TestShareStrmIncrementalOldFullInterruptedRemap(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	fixture.service.tasks = newPhaseFixture(t, true).service.tasks
	fixture.recovery = true
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	if result := fixture.round("old-seed", domain.ShareLibraryQuery{}, true); result.Error != "" || !fixture.recovered {
		t.Fatalf("旧全量未建立凭证：%+v", result)
	}
	source.Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 2}}
	fixture.store.sources = []domain.ShareStrmSource{source}
	fixture.store.fileErr = errors.New("after K2 file effect")
	if result := fixture.round("old-interrupted", domain.ShareLibraryQuery{}, false); result.Error == "" || fixture.recovered {
		t.Fatalf("旧全量半成功错误确认：%+v", result)
	}
	fixture.recovery = false
	fixture.store.fileErr = nil
	fixture.service.tasks = nil
	source.Episodes = []domain.ShareEpisode{{SeasonNumber: 3, EpisodeNumber: 3}}
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.bump(source.WorkKey)
	dirty := store.dirty[source.WorkKey]
	dirty.PendingKeys = fixture.externalPlans[source.WorkKey]
	store.dirty[source.WorkKey] = dirty
	if err := runIncrementalFixture(t, fixture, service, "K3"); err != nil {
		t.Fatal(err)
	}
	if !fixture.stale["1:2:2"] || fixture.stale["1:3:3"] || len(store.dirty[source.WorkKey].PendingKeys) != 0 || len(phaseFiles(t, fixture.cfg.OutputPath)) != 3 {
		t.Fatal("未找回 K2、误失效 K3、误删文件或未正确 ack")
	}
}

// TestShareStrmIncrementalConfigurationCAS 验证写盘后配置竞争不会推进检查点或清空计划键。
func TestShareStrmIncrementalConfigurationCAS(t *testing.T) {
	fixture, store, service := newIncrementalFixture(t)
	source := phaseSource(1)
	fixture.store.sources = []domain.ShareStrmSource{source}
	store.bump(source.WorkKey)
	store.beforeComplete = func() { store.input.ConfigRevision++ }
	if err := runIncrementalFixture(t, fixture, service, "config-race"); !errors.Is(err, dao.ErrShareExportChanged) {
		t.Fatalf("配置竞争错误：%v", err)
	}
	if store.completions != 0 || len(store.states) != 0 || len(store.dirty[source.WorkKey].PendingKeys) != 1 {
		t.Fatal("配置竞争误 ack 或丢失 pending")
	}
}

// TestShareStrmIncrementalSourceChanges 覆盖来源变更、失效和恢复，始终保留未受影响作品与磁盘旧文件。
func TestShareStrmIncrementalSourceChanges(t *testing.T) {
	for _, scenario := range []string{"file-version", "share-version", "file-name", "remote-file", "title", "episodes", "available", "cancel", "delete", "share-delete", "same-share-files", "same-named-shares"} {
		t.Run(scenario, func(t *testing.T) {
			fixture, store, service := newIncrementalFixture(t)
			source := phaseSource(1)
			fixture.store.sources = []domain.ShareStrmSource{source, phaseSource(99)}
			store.bump(source.WorkKey)
			if err := runIncrementalFixture(t, fixture, service, "seed"); err != nil {
				t.Fatal(err)
			}
			initialReads := store.reads
			expectedKeys := []string{"1:1:1"}
			switch scenario {
			case "file-version":
				source.FileVersion++
			case "share-version":
				source.ShareVersion++
			case "file-name":
				source.FileName = "Show1/renamed.S01E01.mkv"
			case "remote-file":
				source.RemoteFileID = "synthetic-remote-new"
			case "title":
				source.Result.Title = "New Title"
			case "episodes":
				source.Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}, {SeasonNumber: 2, EpisodeNumber: 4}}
				expectedKeys = []string{"1:2:3", "1:2:4"}
			}
			fixture.store.sources = []domain.ShareStrmSource{source, phaseSource(99)}
			invalid := scenario == "available" || scenario == "cancel" || scenario == "delete" || scenario == "share-delete"
			if invalid {
				fixture.store.sources = []domain.ShareStrmSource{phaseSource(99)}
				expectedKeys = nil
				if scenario == "available" || scenario == "cancel" {
					observation := store.states[1]
					observation.ExportKeys, observation.State = nil, "stale"
					observation.Available, observation.Cancelled = scenario != "available", scenario == "cancel"
					store.observations = map[string][]domain.ShareExportSourceState{source.WorkKey: {observation}}
				}
			}
			if scenario == "same-share-files" || scenario == "same-named-shares" {
				second := source
				second.ID, second.FileName = 2, "Show1/second.S01E01.mkv"
				if scenario == "same-named-shares" {
					second.ShareID, second.URL = 2, "https://115.com/s/share2"
				}
				fixture.store.sources = []domain.ShareStrmSource{source, second, phaseSource(99)}
				expectedKeys = []string{"1:1:1"}
			}
			store.bump(source.WorkKey)
			if err := runIncrementalFixture(t, fixture, service, scenario); err != nil {
				t.Fatal(err)
			}
			if store.reads != initialReads+1 || store.states[99].SourceFileID != 0 || store.completions != 2 {
				t.Fatal("没有只重算受影响作品")
			}
			if invalid {
				if !fixture.stale["1:1:1"] || store.states[1].State != "stale" || !reflect.DeepEqual(store.states[1].ExportKeys, []string{"1:1:1"}) {
					t.Fatal("失效时丢失历史键或未定向 stale")
				}
				fixture.store.sources, store.observations = []domain.ShareStrmSource{source}, nil
				store.bump(source.WorkKey)
				if err := runIncrementalFixture(t, fixture, service, "restore"); err != nil {
					t.Fatal(err)
				}
				if fixture.stale["1:1:1"] || store.states[1].State != "active" {
					t.Fatal("恢复未重新激活")
				}
			} else if !reflect.DeepEqual(sortedIncrementalKeys(store.states[1].ExportKeys), expectedKeys) || store.states[1].FileVersion != source.FileVersion || store.states[1].ShareVersion != source.ShareVersion {
				t.Fatalf("观察或实际输出键未提交：%+v", store.states[1])
			}
			if scenario == "same-share-files" && !reflect.DeepEqual(store.states[2].ExportKeys, []string{"1:1:1"}) {
				t.Fatal("同分享多文件未整作品重算")
			}
			if scenario == "same-named-shares" {
				for path := range phaseFiles(t, fixture.cfg.OutputPath) {
					if strings.Contains(path, "分享") && !strings.Contains(path, "-分享") {
						t.Fatal("重名分享缺少稳定分享 ID 后缀")
					}
				}
			}
			if len(phaseFiles(t, fixture.cfg.OutputPath)) == 0 {
				t.Fatal("旧磁盘文件被删除")
			}
		})
	}
}

// TestShareStrmIncrementalRebindBothOrders 验证 B 先提交也不会使 A 失去 OLD 键。
func TestShareStrmIncrementalRebindBothOrders(t *testing.T) {
	for _, nextWork := range []string{"aaa", "zzz"} {
		t.Run(nextWork, func(t *testing.T) {
			fixture, store, service := newIncrementalFixture(t)
			source := phaseSource(1)
			fixture.store.sources = []domain.ShareStrmSource{source}
			store.bump(source.WorkKey)
			if err := runIncrementalFixture(t, fixture, service, "seed"); err != nil {
				t.Fatal(err)
			}
			oldWork := source.WorkKey
			store.bump(oldWork)
			dirty := store.dirty[oldWork]
			dirty.PendingKeys = append(dirty.PendingKeys, store.states[1].ExportKeys...)
			store.dirty[oldWork] = dirty
			source.WorkKey, source.MediaID, source.Result.TmdbID, source.Result.Title = nextWork, 2, 2, "Rebound"
			fixture.store.sources = []domain.ShareStrmSource{source}
			store.bump(nextWork)
			if err := runIncrementalFixture(t, fixture, service, "rebind"); err != nil {
				t.Fatal(err)
			}
			if !fixture.stale["1:1:1"] || fixture.stale["2:1:1"] || store.states[1].WorkKey != nextWork || store.states[1].State != "active" || len(store.dirty[oldWork].PendingKeys) != 0 || store.completions != 3 {
				t.Fatal("跨作品顺序丢失 OLD 键或覆写新归属")
			}
			if len(phaseFiles(t, fixture.cfg.OutputPath)) != 2 {
				t.Fatal("跨作品误删旧文件")
			}
		})
	}
}
