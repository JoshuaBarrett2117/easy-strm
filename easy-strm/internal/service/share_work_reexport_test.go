package service

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redis/v8"
)

type workReexportMemory struct {
	*selectedFilesystemStore
	works        map[string][]domain.ShareWorkReexportSelection
	queried      []string
	savedSources []int
	onSelections func(string)
	onSave       func(domain.ShareStrmEntry) error
	readErr      error
}

func (store *workReexportMemory) WorkReexportSelections(_ context.Context, work string) ([]domain.ShareWorkReexportSelection, error) {
	store.queried = append(store.queried, work)
	if store.onSelections != nil {
		store.onSelections(work)
	}
	return append([]domain.ShareWorkReexportSelection(nil), store.works[work]...), store.readErr
}

func (store *workReexportMemory) StrmSources(context.Context, domain.ShareLibraryQuery, int) ([]domain.ShareStrmSource, error) {
	return nil, fmt.Errorf("显式重导禁止枚举全部来源")
}

func (store *workReexportMemory) SaveStrmEntry(ctx context.Context, entry domain.ShareStrmEntry) error {
	if store.onSave != nil {
		if err := store.onSave(entry); err != nil {
			return err
		}
	}
	store.savedSources = append(store.savedSources, entry.MediaID)
	return store.strmMemory.SaveStrmEntry(ctx, entry)
}

type workReexportCheckpoint struct {
	input       domain.ShareExportInput
	store       *workReexportMemory
	inputReads  int
	countReads  int
	onInput     func(int)
	countErrAt  int
	schemaError error
	preflights  int
	references  []workReexportReference
	onPreflight func(int)
}

type workReexportReference struct {
	work, item, mapping, relative string
	media                         int
}

func (store *workReexportCheckpoint) WorkReexportPreflight(_ context.Context, _ string, works []string, entries []domain.ShareStrmEntry, selections []domain.ShareSelection) error {
	store.preflights++
	if store.onPreflight != nil {
		store.onPreflight(store.preflights)
	}
	scope := map[string]bool{}
	for _, work := range works {
		scope[work] = true
	}
	media := map[string]int{}
	for index, entry := range entries {
		if previous, exists := media[entry.ID]; exists && previous != entry.MediaID {
			return dao.ErrShareExportChanged
		}
		media[entry.ID] = entry.MediaID
		for _, reference := range store.references {
			mappingMatch := reference.mapping == entry.ID
			pathMatch := reference.relative == selections[index].RelativePath
			if (mappingMatch || pathMatch) && (!scope[reference.work] || (mappingMatch && reference.media != entry.MediaID) || (pathMatch && reference.item != selections[index].ItemKey)) {
				return dao.ErrShareExportChanged
			}
		}
	}
	return nil
}

func (store *workReexportCheckpoint) CheckSchema(context.Context) error { return store.schemaError }
func (store *workReexportCheckpoint) ReadInput(context.Context) (domain.ShareExportInput, error) {
	store.inputReads++
	if store.onInput != nil {
		store.onInput(store.inputReads)
	}
	return store.input, nil
}
func (store *workReexportCheckpoint) WorkReexportGlobalInvalid(context.Context, string) (int64, error) {
	store.countReads++
	if store.countReads == store.countErrAt {
		return 0, fmt.Errorf("全局计数读取失败")
	}
	var invalid int64
	for _, values := range store.store.works {
		for _, value := range values {
			source, err := store.store.GetStrmSource(context.Background(), value.SourceID)
			if err != nil {
				invalid++
				continue
			}
			expected, err := domain.BuildShareStrmEntry(source)
			if err != nil {
				return 0, err
			}
			if actual := store.store.entries[expected.ID]; actual.MediaID != value.MediaID {
				invalid++
			}
		}
	}
	return invalid, nil
}

type workReexportFixture struct {
	runner     *shareWorkReexportRunner
	store      *workReexportMemory
	checkpoint *workReexportCheckpoint
	request    domain.ShareWorkReexportRequest
	database   *sql.DB
	mock       sqlmock.Sqlmock
	tasks      *TaskService
}

func newWorkReexportFixture(test *testing.T) *workReexportFixture {
	test.Helper()
	source := filesystemSource(1, 7, 100, filesystemEpisode(1, 1), filesystemEpisode(1, 2))
	duplicate := filesystemSource(2, 8, 9999, filesystemEpisode(1, 1))
	duplicate.MediaID = 9901
	unrelated := filesystemSource(3, 9, 100, filesystemEpisode(1, 1))
	unrelated.WorkKey, unrelated.MediaID = "tmdb:tv:8802", 8802
	store := &workReexportMemory{selectedFilesystemStore: &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{source, duplicate, unrelated}, entries: map[string]domain.ShareStrmEntry{}}}, works: map[string][]domain.ShareWorkReexportSelection{}}
	root := test.TempDir()
	for _, selected := range []domain.ShareStrmSource{source, unrelated} {
		for _, episode := range selected.Episodes {
			selection := domain.ShareSelection{ItemKey: shareStrmIdentity(selected, episode), WorkKey: selected.WorkKey, Season: episode.SeasonNumber, Episode: episode.EpisodeNumber, CandidateID: int64(10 + selected.ID), Mode: "manual", Revision: 3, ExportedRevision: 2, RelativePath: fmt.Sprintf("保留旧标题/%d-%d.strm", selected.ID, episode.EpisodeNumber), Valid: true}
			store.works[selected.WorkKey] = append(store.works[selected.WorkKey], domain.ShareWorkReexportSelection{Selection: selection, SourceID: selected.ID, MediaID: selected.MediaID})
			filename := filepath.Join(root, selection.RelativePath)
			if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
				test.Fatal(err)
			}
			if err := os.WriteFile(filename, filesystemContent(selected), 0644); err != nil {
				test.Fatal(err)
			}
		}
		entry, err := domain.BuildShareStrmEntry(selected)
		if err != nil {
			test.Fatal(err)
		}
		if selected.ID == 1 {
			entry.MediaID = duplicate.MediaID
		}
		store.entries[entry.ID] = entry
	}
	client := setupTaskRedisMock(test)
	test.Cleanup(func() { _ = client.Close() })
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	exporter := NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, tasks, nil, nil, nil)
	database, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = database.Close() })
	checkpoint := &workReexportCheckpoint{store: store, input: domain.ShareExportInput{ConfigRevision: 3, BaselineState: "required", ProtocolVersion: 1, Settings: domain.ShareStrmSettings{OutputPath: root, BaseURL: "https://media.example.test", Cloud115ID: 1, TransferPath: "/media"}}}
	runner := &shareWorkReexportRunner{exporter: exporter, checkpoints: checkpoint, openOutput: func(ctx context.Context, root, id string) (*StrmOutput, error) {
		connection, err := database.Conn(ctx)
		if err != nil {
			return nil, err
		}
		return &StrmOutput{Store: &dao.StrmExportDAO{Conn: connection}, Root: root, Owner: "share:default", Run: id, Paths: map[string]bool{}, Seen: map[string]bool{}}, nil
	}}
	request := domain.ShareWorkReexportRequest{WorkKeys: []string{source.WorkKey}, Reason: "修复选中来源的旧媒体映射", ExpectedOutputPath: root, ExpectedConfigRevision: 3, ActorUserID: 7}
	return &workReexportFixture{runner: runner, store: store, checkpoint: checkpoint, request: request, database: database, mock: mock, tasks: tasks}
}

func (fixture *workReexportFixture) run(test *testing.T, ctx context.Context, id string) (domain.ShareWorkReexportResult, error) {
	test.Helper()
	if err := fixture.tasks.Create(id, "strm_generate", "显式作品重导测试"); err != nil {
		test.Fatal(err)
	}
	return fixture.runner.run(ctx, fixture.request, id)
}

func (fixture *workReexportFixture) expectClose() {
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestShareWorkReexportStaleMappingStablePathsIdempotent(test *testing.T) {
	for _, baseline := range []string{"required", "building"} {
		test.Run(baseline, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			fixture.checkpoint.input.BaselineState = baseline
			originalFiles := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			originalSelections := append([]domain.ShareWorkReexportSelection(nil), fixture.store.works[fixture.request.WorkKeys[0]]...)
			unrelated, _ := domain.BuildShareStrmEntry(fixture.store.sources[2])
			originalUnrelated := fixture.store.entries[unrelated.ID]
			for attempt := 0; attempt < 2; attempt++ {
				for _, value := range originalSelections {
					expectSelectedFile(test, fixture.mock, value.Selection, true, false)
				}
				fixture.expectClose()
				result, err := fixture.run(test, context.Background(), fmt.Sprintf("share_work_reexport_repeat_%d", attempt))
				before := int64(2)
				if attempt > 0 {
					before = 0
				}
				if err != nil || result.GlobalInvalidBefore == nil || *result.GlobalInvalidBefore != before || result.GlobalInvalidAfter == nil || *result.GlobalInvalidAfter != 0 || result.InvalidScope != "global" || result.ProcessedRows != 2 || result.ProcessedWorkCount != 1 || result.SelectionRows != 2 || result.Written != 0 {
					test.Fatalf("修复与重复执行结果不正确：%+v err=%v", result, err)
				}
				assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, originalFiles)
				if !reflect.DeepEqual(originalSelections, fixture.store.works[fixture.request.WorkKeys[0]]) || !reflect.DeepEqual(originalUnrelated, fixture.store.entries[unrelated.ID]) || fixture.store.syncs != 0 || fixture.store.bulkResolves != 0 {
					test.Fatal("选择、稳定路径、无关作品或准备语义被修改")
				}
				for _, work := range fixture.store.queried {
					if work != fixture.request.WorkKeys[0] {
						test.Fatalf("读取了范围外作品 %q", work)
					}
				}
				for _, mediaID := range fixture.store.savedSources {
					if mediaID != 8801 {
						test.Fatal("使用了重复媒体行或未选中来源")
					}
				}
				if err = fixture.mock.ExpectationsWereMet(); err != nil {
					test.Fatal(err)
				}
			}
		})
	}
}

func TestShareWorkReexportFailClosedAndAudit(test *testing.T) {
	for _, scenario := range []string{"empty", "unknown-work", "selection-invalid", "media-mismatch", "path-empty", "path-traversal", "path-collision", "selection-cap", "read-error", "schema-error", "config-drift", "fingerprint-drift", "source-drift", "selection-drift", "scope-drift", "cancel-before", "cancel-midway", "mapping-write-failure", "after-count-failure"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			work := fixture.request.WorkKeys[0]
			values := fixture.store.works[work]
			opened := true
			switch scenario {
			case "empty":
				fixture.request.WorkKeys = nil
				opened = false
			case "unknown-work":
				fixture.request.WorkKeys = append(fixture.request.WorkKeys, "unknown")
			case "selection-invalid":
				values[0].Selection.Valid = false
			case "media-mismatch":
				values[0].MediaID = 9901
			case "path-empty":
				values[0].Selection.RelativePath = ""
			case "path-traversal":
				values[0].Selection.RelativePath = "../outside.strm"
			case "path-collision":
				values[1].Selection.RelativePath = values[0].Selection.RelativePath
			case "selection-cap":
				fixture.store.works[work] = make([]domain.ShareWorkReexportSelection, domain.ShareWorkReexportMaxSelections+1)
			case "read-error":
				fixture.store.readErr = errors.New("选择读取失败")
			case "schema-error":
				fixture.checkpoint.schemaError = errors.New("协议缺失")
				opened = false
			case "config-drift", "fingerprint-drift":
				fixture.checkpoint.onInput = func(read int) {
					if read == 2 {
						if scenario == "config-drift" {
							fixture.checkpoint.input.ConfigRevision++
						} else {
							fixture.checkpoint.input.Settings.BaseURL = "https://changed.example.test"
						}
					}
				}
			case "source-drift", "selection-drift", "scope-drift":
				fixture.store.onSelections = func(string) {
					if len(fixture.store.queried) == 2 {
						switch scenario {
						case "source-drift":
							fixture.store.sources[0].MediaID = 9901
						case "selection-drift":
							values[0].Selection.CandidateID++
						case "scope-drift":
							fixture.store.works[work] = append(values, values[0])
						}
					}
				}
			case "cancel-before":
				cancel()
				opened = false
			case "cancel-midway", "mapping-write-failure":
				expectSelectedFile(test, fixture.mock, values[0].Selection, true, false)
				fixture.store.onSave = func(domain.ShareStrmEntry) error {
					if len(fixture.store.savedSources) == 1 {
						if scenario == "cancel-midway" {
							cancel()
							return context.Canceled
						}
						return errors.New("映射写入失败")
					}
					return nil
				}
			case "after-count-failure":
				fixture.checkpoint.countErrAt = 2
				for _, value := range values {
					expectSelectedFile(test, fixture.mock, value.Selection, true, false)
				}
			}
			if opened {
				fixture.expectClose()
			}
			originalFiles := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			id := "share_work_reexport_" + scenario
			result, err := fixture.run(test, ctx, id)
			if err == nil {
				test.Fatalf("失败分支错误地确认成功：%+v", result)
			}
			if scenario == "cancel-before" || scenario == "cancel-midway" {
				if !errors.Is(err, context.Canceled) {
					test.Fatal(err)
				}
			}
			if scenario == "cancel-midway" || scenario == "mapping-write-failure" {
				if result.ProcessedRows != 1 || result.ProcessedWorkCount != 0 || result.Works[0].Status != "failed" || result.Works[0].ProcessedRows != 1 {
					test.Fatalf("部分结果丢失：%+v", result)
				}
			}
			if scenario == "after-count-failure" && (result.GlobalInvalidAfter != nil || result.AfterCountError == "") {
				test.Fatal("不能将读取失败伪装成 invalid=0")
			}
			if scenario == "selection-cap" && !strings.Contains(err.Error(), "10000") {
				test.Fatalf("未实际验证行数上限：%v", err)
			}
			assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, originalFiles)
			if scenario != "empty" {
				task, taskErr := fixture.tasks.Get(id)
				if taskErr != nil {
					test.Fatal(taskErr)
				}
				raw, _ := json.Marshal(task["metadata"])
				if !strings.Contains(string(raw), `"requested_works"`) || !strings.Contains(string(raw), `"work_reexport_request"`) || !strings.Contains(string(raw), `"error"`) {
					test.Fatalf("审计范围或失败结果丢失：%s", raw)
				}
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportValidationAndResumeGuard(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	for _, keys := range [][]string{nil, {}, {""}, {" "}, {"*"}, {"work", "work"}, {"work\n"}, {strings.Repeat("x", 201)}, make([]string, domain.ShareWorkReexportMaxWorks+1)} {
		request := fixture.request
		request.WorkKeys = keys
		if ValidateShareWorkReexport(request) == nil {
			test.Fatalf("无效范围未拒绝：%q", keys)
		}
	}
	if err := fixture.tasks.Create("share_work_reexport_no_replay", "strm_generate", "重导"); err != nil {
		test.Fatal(err)
	}
	if err := fixture.tasks.SetError("share_work_reexport_no_replay", "失败"); err != nil {
		test.Fatal(err)
	}
	if err := fixture.tasks.Resume("share_work_reexport_no_replay"); err == nil {
		test.Fatal("通用恢复不能触发全量")
	}
}

func TestShareWorkReexportOwnerLockCancellation(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	root := fixture.request.ExpectedOutputPath
	ancestors := []string{}
	for directory := root; ; directory = filepath.Dir(directory) {
		ancestors = append([]string{directory}, ancestors...)
		if directory == filepath.Dir(directory) {
			break
		}
	}
	for _, directory := range ancestors {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WithArgs(directory).WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	}
	fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock.*share:default").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	fixture.expectClose()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	fixture.runner.openOutput = func(ctx context.Context, root, id string) (*StrmOutput, error) {
		return waitStrmOutput(ctx, fixture.database, root, "share:default", id, nil, false)
	}
	result, err := fixture.run(test, ctx, "share_work_reexport_owner_busy")
	if !errors.Is(err, context.DeadlineExceeded) || result.ProcessedRows != 0 || fixture.checkpoint.countReads != 0 || len(fixture.store.savedSources) != 0 {
		test.Fatalf("owner 锁等待必须可取消且没有副作用：%+v %v", result, err)
	}
	if err = fixture.mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}

func TestShareWorkReexportAsyncTaskTracksSchemaFailure(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	fixture.runner.exporter.SetExportDatabase(fixture.database)
	fixture.mock.ExpectQuery("SELECT to_regclass.*t_share_export_source_state").WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(false))
	id, err := fixture.runner.exporter.StartWorkReexportContext(context.Background(), fixture.request)
	if err != nil || !strings.HasPrefix(id, "share_work_reexport_") {
		test.Fatalf("未返回可追踪任务：%s %v", id, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := fixture.tasks.Get(id)
		if err != nil {
			test.Fatal(err)
		}
		if task["status"] == "failed" {
			raw, _ := json.Marshal(task["metadata"])
			if !strings.Contains(string(raw), `"work_reexport_request"`) || !strings.Contains(string(raw), `"requested_works"`) {
				test.Fatalf("异步失败丢失审计：%s", raw)
			}
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	test.Fatal("异步任务没有明确失败终态")
}

func TestShareWorkReexportMultipleWorksAndGlobalInvalidScope(test *testing.T) {
	for _, scenario := range []string{"multiple-success", "second-work-failure", "global-invalid-outside-scope"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			firstWork := fixture.request.WorkKeys[0]
			secondWork := fixture.store.sources[2].WorkKey
			unrelated, _ := domain.BuildShareStrmEntry(fixture.store.sources[2])
			if scenario == "global-invalid-outside-scope" {
				unrelated.MediaID = 9999
				fixture.store.entries[unrelated.ID] = unrelated
			} else {
				fixture.request.WorkKeys = append(fixture.request.WorkKeys, secondWork)
			}
			originalFiles := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			for _, value := range fixture.store.works[firstWork] {
				expectSelectedFile(test, fixture.mock, value.Selection, true, false)
			}
			if scenario == "multiple-success" {
				expectSelectedFile(test, fixture.mock, fixture.store.works[secondWork][0].Selection, true, false)
			}
			if scenario == "second-work-failure" {
				fixture.store.onSave = func(entry domain.ShareStrmEntry) error {
					if entry.MediaID == 8802 {
						return errors.New("第二作品映射写入失败")
					}
					return nil
				}
			}
			fixture.expectClose()
			result, err := fixture.run(test, context.Background(), "share_work_reexport_"+scenario)
			switch scenario {
			case "multiple-success":
				if err != nil || result.ProcessedRows != 3 || result.ProcessedWorkCount != 2 || !reflect.DeepEqual(result.ProcessedWorks, fixture.request.WorkKeys) {
					test.Fatalf("多个明确作品未全部处理：%+v %v", result, err)
				}
			case "second-work-failure":
				if err == nil || result.ProcessedRows != 2 || result.ProcessedWorkCount != 1 || result.Works[0].Status != "completed" || result.Works[1].Status != "failed" {
					test.Fatalf("逐作品部分失败审计不完整：%+v %v", result, err)
				}
			case "global-invalid-outside-scope":
				if err != nil || result.GlobalInvalidBefore == nil || *result.GlobalInvalidBefore != 3 || result.GlobalInvalidAfter == nil || *result.GlobalInvalidAfter != 1 || !reflect.DeepEqual(unrelated, fixture.store.entries[unrelated.ID]) {
					test.Fatalf("不能为降低全局 invalid 扩大范围：%+v %v", result, err)
				}
				for _, work := range fixture.store.queried {
					if work != firstWork {
						test.Fatalf("范围外读取：%s", work)
					}
				}
			}
			assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, originalFiles)
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

func TestShareWorkReexportScopePreflightRejectsBeforeMappingMutation(test *testing.T) {
	for _, scenario := range []string{"cross-work-shared-mapping", "same-mapping-incompatible-media", "outside-scope-path", "within-scope-reference-incompatible-media", "immediate-write-recheck"} {
		test.Run(scenario, func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			first := fixture.store.sources[0]
			entry, err := domain.BuildShareStrmEntry(first)
			if err != nil {
				test.Fatal(err)
			}
			selection := fixture.store.works[first.WorkKey][0].Selection
			reference := workReexportReference{work: "tmdb:tv:outside", item: "outside:1:1", mapping: entry.ID, relative: "outside.strm", media: 7701}
			switch scenario {
			case "same-mapping-incompatible-media":
				second := first
				second.ID, second.MediaID = 4, 7701
				second.Episodes = []domain.ShareEpisode{filesystemEpisode(1, 3)}
				fixture.store.sources = append(fixture.store.sources, second)
				other := selection
				other.ItemKey, other.Episode, other.CandidateID, other.RelativePath = shareStrmIdentity(second, second.Episodes[0]), 3, 14, "different.strm"
				fixture.store.works[first.WorkKey] = append(fixture.store.works[first.WorkKey], domain.ShareWorkReexportSelection{Selection: other, SourceID: second.ID, MediaID: second.MediaID})
			case "outside-scope-path":
				reference.mapping, reference.relative = "different-mapping", fixture.store.works[first.WorkKey][1].Selection.RelativePath
				fixture.checkpoint.references = []workReexportReference{reference}
			case "within-scope-reference-incompatible-media":
				reference.work = first.WorkKey
				fixture.checkpoint.references = []workReexportReference{reference}
			case "immediate-write-recheck":
				fixture.checkpoint.onPreflight = func(read int) {
					if read >= 3 {
						fixture.checkpoint.references = []workReexportReference{reference}
					}
				}
			default:
				fixture.checkpoint.references = []workReexportReference{reference}
			}
			original := fixture.store.entries[entry.ID]
			files := filesystemCollection(test, fixture.request.ExpectedOutputPath)
			fixture.expectClose()
			result, err := fixture.run(test, context.Background(), "share_work_reexport_scope_"+scenario)
			if !errors.Is(err, dao.ErrShareExportChanged) || len(fixture.store.savedSources) != 0 || result.ProcessedRows != 0 || !reflect.DeepEqual(original, fixture.store.entries[entry.ID]) {
				test.Fatalf("必须在任何映射写入前拒绝范围冲突：saved=%v result=%+v err=%v", fixture.store.savedSources, result, err)
			}
			if scenario == "immediate-write-recheck" && fixture.checkpoint.preflights != 3 {
				test.Fatalf("必须由紧邻 SaveStrmEntry 的护栏拒绝：reads=%d", fixture.checkpoint.preflights)
			}
			assertFilesystemCollection(test, fixture.request.ExpectedOutputPath, files)
			if err = fixture.mock.ExpectationsWereMet(); err != nil {
				test.Fatal(err)
			}
		})
	}
}

type workReexportTerminalHook struct {
	observed chan error
}

func (hook *workReexportTerminalHook) BeforeProcess(ctx context.Context, command redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (hook *workReexportTerminalHook) AfterProcess(ctx context.Context, command redis.Cmder) error {
	if command.Name() != "set" || len(command.Args()) < 3 {
		return nil
	}
	payload, ok := command.Args()[2].([]byte)
	if !ok {
		payload = []byte(fmt.Sprint(command.Args()[2]))
	}
	var task map[string]interface{}
	if json.Unmarshal(payload, &task) != nil || (task["status"] != "failed" && task["status"] != "cancelled" && task["status"] != "completed") {
		return nil
	}
	deadline, exists := ctx.Deadline()
	var err error
	if !exists || time.Until(deadline) <= 0 || time.Until(deadline) > 10*time.Second || ctx.Err() != nil {
		err = fmt.Errorf("终态持久化上下文必须独立、未取消且最多十秒：deadline=%v exists=%v err=%v", deadline, exists, ctx.Err())
	}
	hook.observed <- err
	return nil
}

func (hook *workReexportTerminalHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (hook *workReexportTerminalHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestShareWorkReexportTerminalPersistenceHasDeadline(test *testing.T) {
	for _, cancelled := range []bool{false, true} {
		test.Run(fmt.Sprintf("cancelled=%v", cancelled), func(test *testing.T) {
			fixture := newWorkReexportFixture(test)
			client := setupTaskRedisMock(test)
			defer client.Close()
			hook := &workReexportTerminalHook{observed: make(chan error, 4)}
			client.AddHook(hook)
			fixture.tasks = NewTaskService(dao.NewTaskRedisDAO(client))
			fixture.runner.exporter.tasks = fixture.tasks
			fixture.runner.exporter.SetExportDatabase(fixture.database)
			query := fixture.mock.ExpectQuery("SELECT to_regclass.*t_share_export_source_state")
			if cancelled {
				query.WillDelayFor(50 * time.Millisecond)
			}
			query.WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(false))
			id, err := fixture.runner.exporter.StartWorkReexportContext(context.Background(), fixture.request)
			if err != nil {
				test.Fatal(err)
			}
			if cancelled {
				cancel, exists := fixture.tasks.cancelFuncs.Load(id)
				if !exists {
					test.Fatal("任务没有注册取消")
				}
				cancel.(context.CancelFunc)()
			}
			select {
			case err = <-hook.observed:
				if err != nil {
					test.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				test.Fatal("未观察到有界终态持久化")
			}
			if !cancelled {
				if err = fixture.mock.ExpectationsWereMet(); err != nil {
					test.Fatal(err)
				}
			}
		})
	}
}

func TestShareWorkReexportScopeAllowsMultiEpisodeSharedMapping(test *testing.T) {
	fixture := newWorkReexportFixture(test)
	source := fixture.store.sources[0]
	entry, err := domain.BuildShareStrmEntry(source)
	if err != nil {
		test.Fatal(err)
	}
	for _, value := range fixture.store.works[source.WorkKey] {
		fixture.checkpoint.references = append(fixture.checkpoint.references, workReexportReference{work: source.WorkKey, item: value.Selection.ItemKey, mapping: entry.ID, relative: value.Selection.RelativePath, media: source.MediaID})
		expectSelectedFile(test, fixture.mock, value.Selection, true, false)
	}
	fixture.expectClose()
	result, err := fixture.run(test, context.Background(), "share_work_reexport_scope_multi_episode")
	if err != nil || result.ProcessedRows != 2 || fixture.checkpoint.preflights < 3 || fixture.store.entries[entry.ID].MediaID != source.MediaID {
		test.Fatalf("同作品多集可以共享映射并修复旧媒体：%+v preflights=%d err=%v", result, fixture.checkpoint.preflights, err)
	}
	if err = fixture.mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
}
