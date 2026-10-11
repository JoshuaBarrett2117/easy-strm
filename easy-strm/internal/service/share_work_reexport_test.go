package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

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
	states       map[string]dao.ExportState
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
	for index, entry := range entries {
		for _, reference := range store.references {
			mappingMatch := reference.mapping == entry.ID
			pathMatch := reference.relative == selections[index].RelativePath
			if (mappingMatch || pathMatch) && (!scope[reference.work] || (pathMatch && reference.item != selections[index].ItemKey)) {
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
			_, err := store.store.GetStrmSource(context.Background(), value.SourceID)
			if err != nil {
				invalid++
				continue
			}
			if actual := store.store.entries[store.store.states[value.Selection.ItemKey].Playback]; actual.MediaID != value.MediaID {
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
	store := &workReexportMemory{selectedFilesystemStore: &selectedFilesystemStore{strmMemory: &strmMemory{sources: []domain.ShareStrmSource{source, duplicate, unrelated}, entries: map[string]domain.ShareStrmEntry{}}}, works: map[string][]domain.ShareWorkReexportSelection{}, states: map[string]dao.ExportState{}}
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
		payload, _ := json.Marshal(entry)
		for _, value := range store.works[selected.WorkKey] {
			filename := filepath.Join(root, value.Selection.RelativePath)
			store.states[value.Selection.ItemKey] = dao.ExportState{Owner: "share:default", Key: value.Selection.ItemKey, Path: filename, Content: fmt.Sprintf("%x", sha256.Sum256(append([]byte(filename+"\x00"), filesystemContent(selected)...))), Mapping: fmt.Sprintf("%x", sha256.Sum256(payload)), Playback: entry.ID, Run: "old-run"}
		}
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
	entries, states := maps.Clone(fixture.store.entries), maps.Clone(fixture.store.states)
	if err := fixture.tasks.Create(id, "strm_generate", "显式作品重导测试"); err != nil {
		test.Fatal(err)
	}
	result, err := fixture.runner.run(ctx, fixture.request, id)
	if err != nil && result.ProcessedRows == 0 {
		fixture.store.entries, fixture.store.states = entries, states
	}
	return result, err
}

func (fixture *workReexportFixture) expectClose() {
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 1))
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
