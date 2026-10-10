package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type phaseCounts struct {
	MemoryReads, MemoryWrites, SQLReads, SQLWrites int
	SourcePages, ConflictPages, FreshReads         int
	ProgressWrites, MetadataWrites                 int
	Creates, Rewrites, Deletes                     int
}

type phaseFile struct {
	Content  string
	Modified int64
	Info     os.FileInfo `json:"-"`
}

type phaseResult struct {
	Scenario                                                       string
	ElapsedNS                                                      int64
	Counts                                                         phaseCounts
	Processed, Success, Failed, Added, Updated, Skipped, Conflicts int
	Total, Progress, ExportedFiles                                 int
	Status                                                         string
	SkippedSources                                                 int
	Errors                                                         []string
	Error                                                          string
	Files                                                          map[string]phaseFile
	States                                                         map[string]dao.ExportState
	Stale                                                          []string
}

type phaseArgument func(driver.Value) bool

func (match phaseArgument) Match(value driver.Value) bool { return match(value) }

var phaseDriverID atomic.Int64

type phaseDriver struct {
	driver.Conn
	counts *phaseCounts
}

func (connection *phaseDriver) Open(string) (driver.Conn, error) { return connection, nil }
func (connection *phaseDriver) Close() error                     { return nil }
func (connection *phaseDriver) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	connection.counts.SQLReads++
	return connection.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (connection *phaseDriver) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	connection.counts.SQLWrites++
	return connection.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

type phaseTaskHook struct{ counts *phaseCounts }

func (hook phaseTaskHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "set" {
		callers := make([]uintptr, 32)
		frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers)])
		for {
			frame, more := frames.Next()
			if strings.HasSuffix(frame.Function, "(*TaskRedisDAO).UpdateProgress") {
				hook.counts.ProgressWrites++
				break
			}
			if strings.HasSuffix(frame.Function, "(*TaskRedisDAO).UpdateMetadata") {
				hook.counts.MetadataWrites++
				break
			}
			if !more {
				break
			}
		}
	}
	return ctx, nil
}
func (hook phaseTaskHook) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (hook phaseTaskHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (hook phaseTaskHook) AfterProcessPipeline(context.Context, []redis.Cmder) error { return nil }

type phaseStore struct {
	*strmMemory
	fixture     *phaseFixture
	cancelAfter int
	cancel      context.CancelFunc
	readErr     error
	fresh       func(domain.ShareStrmSource) (domain.ShareStrmSource, error)
	flagCancel  bool
}

func (store *phaseStore) StrmSources(ctx context.Context, query domain.ShareLibraryQuery, after int) ([]domain.ShareStrmSource, error) {
	store.fixture.counts.MemoryReads++
	if query.WorkKey == "" {
		store.fixture.counts.SourcePages++
	} else {
		store.fixture.counts.ConflictPages++
		if store.fixture.awaitConflict {
			coordinator := store.fixture.service.Coordinator()
			coordinator.mu.Lock()
			holder := coordinator.holders[fileKey(store.fixture.freshID)]
			locked := holder != nil && holder.writer != ""
			coordinator.mu.Unlock()
			if !locked {
				store.fixture.test.Fatal("冲突重算未持有来源锁")
			}
			store.fixture.awaitConflict = false
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if store.readErr != nil && after > 0 && query.WorkKey == "" && store.fixture.entries > 0 {
		return nil, store.readErr
	}
	rows := []domain.ShareStrmSource{}
	selectedWorks := map[string]bool{}
	for _, source := range store.sources {
		for _, id := range query.FileIDs {
			if source.ID == id {
				selectedWorks[source.WorkKey] = true
			}
		}
	}
	for _, source := range store.sources {
		if source.ID > after && (query.WorkKey == "" || source.WorkKey == query.WorkKey) && (len(query.FileIDs) == 0 || selectedWorks[source.WorkKey]) {
			rows = append(rows, source)
		}
	}
	remaining := len(rows)
	if remaining == 0 && query.WorkKey == "" && store.fixture.finish && store.fixture.entries > 0 {
		store.fixture.expectFinish()
	}
	if remaining == 0 && query.WorkKey != "" {
		store.fixture.freshID = 0
	}
	if len(rows) > 100 {
		rows = rows[:100]
	}
	for index := range rows {
		rows[index].Remaining = remaining
	}
	return rows, nil
}

func (store *phaseStore) SaveStrmEntry(ctx context.Context, entry domain.ShareStrmEntry) error {
	store.fixture.counts.MemoryWrites++
	if err := store.strmMemory.SaveStrmEntry(ctx, entry); err != nil {
		return err
	}
	store.fixture.expectPaths(entry)
	return nil
}

func (store *phaseStore) SaveExportedStrmFile(ctx context.Context, file domain.StrmFile) error {
	store.fixture.counts.MemoryWrites++
	err := store.strmMemory.SaveExportedStrmFile(ctx, file)
	if store.cancelAfter > 0 {
		store.cancelAfter--
		if store.cancelAfter == 0 {
			store.cancel()
		}
	}
	return err
}

type phaseReader struct{ *phaseStore }

func (store phaseReader) GetStrmSource(_ context.Context, id int) (domain.ShareStrmSource, error) {
	fixture := store.fixture
	fixture.counts.MemoryReads++
	fixture.counts.FreshReads++
	fixture.freshID = id
	fixture.service.Coordinator().mu.Lock()
	holder := fixture.service.Coordinator().holders[fileKey(id)]
	locked := holder != nil && holder.writer != ""
	fixture.service.Coordinator().mu.Unlock()
	if !locked {
		fixture.test.Fatal("来源重读未持有文件独占锁")
	}
	for _, source := range store.sources {
		if source.ID == id {
			fresh := source
			var err error
			if store.fresh != nil {
				fresh, err = store.fresh(source)
			}
			fixture.awaitConflict = err == nil && source.FileVersion == fresh.FileVersion && source.ShareVersion == fresh.ShareVersion && source.WorkKey == fresh.WorkKey
			return fresh, err
		}
	}
	return domain.ShareStrmSource{}, sql.ErrNoRows
}

type phaseFixture struct {
	test          testing.TB
	service       *ShareStrmService
	store         *phaseStore
	db            *sql.DB
	mock          sqlmock.Sqlmock
	cfg           domain.ShareStrmSettings
	counts        phaseCounts
	states        map[string]dao.ExportState
	history       map[string]string
	stale         map[string]bool
	foreign       bool
	unregistered  bool
	finish        bool
	run           string
	entries       int
	freshID       int
	awaitConflict bool
	recovery      bool
	recovered     bool
	recoveryRace  bool
	externalPlans map[string][]string
}

func newPhaseFixture(test testing.TB, reader bool) *phaseFixture {
	test.Helper()
	fixture := &phaseFixture{test: test, states: map[string]dao.ExportState{}, history: map[string]string{}, stale: map[string]bool{}}
	fixture.db, fixture.mock = phaseMockDatabase(test, &fixture.counts)
	mini := miniredis.NewMiniRedis()
	if err := mini.Start(); err != nil {
		test.Fatal(err)
	}
	test.Cleanup(mini.Close)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	test.Cleanup(func() { client.Close() })
	client.AddHook(phaseTaskHook{&fixture.counts})
	fixture.cfg = domain.ShareStrmSettings{OutputPath: test.TempDir(), BaseURL: "https://media.example.test"}
	fixture.store = &phaseStore{strmMemory: &strmMemory{entries: map[string]domain.ShareStrmEntry{}}, fixture: fixture}
	var store ShareStrmStore = fixture.store
	if reader {
		store = phaseReader{fixture.store}
	}
	fixture.service = NewShareStrmService(store, nil, nil, nil, &OrganizeService{}, NewTaskService(dao.NewTaskRedisDAO(client)), func() ([]*domain.MediaCategory, error) { return nil, nil }, nil, nil)
	fixture.service.SetExportDatabase(fixture.db)
	return fixture
}

func phaseMockDatabase(test testing.TB, counts *phaseCounts) (*sql.DB, sqlmock.Sqlmock) {
	test.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		test.Fatal(err)
	}
	mock.MatchExpectationsInOrder(false)
	connection, err := db.Conn(context.Background())
	if err != nil {
		test.Fatal(err)
	}
	name := fmt.Sprintf("phase01-%d", phaseDriverID.Add(1))
	err = connection.Raw(func(raw interface{}) error {
		sql.Register(name, &phaseDriver{Conn: raw.(driver.Conn), counts: counts})
		return nil
	})
	if err != nil {
		test.Fatal(err)
	}
	counted, err := sql.Open(name, "")
	if err != nil {
		test.Fatal(err)
	}
	counted.SetMaxOpenConns(1)
	test.Cleanup(func() { counted.Close(); connection.Close(); db.Close() })
	return counted, mock
}

type phaseSQLStore struct {
	ShareRecordDAO *dao.ShareRecordDAO
	fixture        *phaseFixture
	mock           sqlmock.Sqlmock
}

func (store *phaseSQLStore) GetStrmEntry(ctx context.Context, id string) (domain.ShareStrmEntry, error) {
	return store.ShareRecordDAO.GetStrmEntry(ctx, id)
}
func (store *phaseSQLStore) LockStrmPlayback(ctx context.Context, key string) (func(), error) {
	return store.ShareRecordDAO.LockStrmPlayback(ctx, key)
}

func (store *phaseSQLStore) expectSources(query domain.ShareLibraryQuery, after int) int {
	rows := sqlmock.NewRows([]string{"id", "share_id", "share_name", "work_key", "url", "password", "file_name", "remote_id", "result", "episodes"})
	sources := []domain.ShareStrmSource{}
	for _, source := range store.fixture.store.sources {
		if source.ID > after && (query.WorkKey == "" || source.WorkKey == query.WorkKey) {
			sources = append(sources, source)
		}
	}
	remaining := len(sources)
	if len(sources) > 101 {
		sources = sources[:101]
	}
	for _, source := range sources {
		raw, err := json.Marshal(source.Result)
		if err != nil {
			store.fixture.test.Fatal(err)
		}
		result := map[string]interface{}{}
		if err = json.Unmarshal(raw, &result); err != nil {
			store.fixture.test.Fatal(err)
		}
		result["_media_id"], result["_file_version"], result["_share_version"] = source.MediaID, source.FileVersion, source.ShareVersion
		raw, err = json.Marshal(result)
		if err != nil {
			store.fixture.test.Fatal(err)
		}
		episodes, err := json.Marshal(source.Episodes)
		if err != nil {
			store.fixture.test.Fatal(err)
		}
		rows.AddRow(source.ID, source.ShareID, source.ShareName, source.WorkKey, source.URL, source.Password, source.FileName, source.RemoteFileID, raw, episodes)
	}
	store.mock.ExpectQuery("SELECT f.id,f.share_id").WillReturnRows(rows)
	return remaining
}

func (store *phaseSQLStore) StrmSources(ctx context.Context, query domain.ShareLibraryQuery, after int) ([]domain.ShareStrmSource, error) {
	if store.expectSources(query, after) == 0 && query.WorkKey == "" && store.fixture.finish && store.fixture.entries > 0 {
		store.fixture.expectFinish()
	}
	return store.ShareRecordDAO.StrmSources(ctx, query, after)
}

func (store *phaseSQLStore) GetStrmSource(ctx context.Context, id int) (domain.ShareStrmSource, error) {
	store.expectSources(domain.ShareLibraryQuery{}, id-1)
	return store.ShareRecordDAO.GetStrmSource(ctx, id)
}

func (store *phaseSQLStore) SaveStrmEntry(ctx context.Context, entry domain.ShareStrmEntry) error {
	store.mock.ExpectExec("INSERT INTO t_share_strm").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.ShareRecordDAO.SaveStrmEntry(ctx, entry); err != nil {
		return err
	}
	store.fixture.expectPaths(entry)
	return nil
}

func (store *phaseSQLStore) SaveExportedStrmFile(ctx context.Context, file domain.StrmFile) error {
	store.mock.ExpectExec("INSERT INTO t_strm_file").WillReturnResult(sqlmock.NewResult(0, 1))
	return store.ShareRecordDAO.SaveExportedStrmFile(ctx, file)
}

func phaseSource(id int) domain.ShareStrmSource {
	return domain.ShareStrmSource{ID: id, MediaID: id, ShareID: id, FileVersion: 1, ShareVersion: 1, ShareName: fmt.Sprintf("分享%d", id), WorkKey: fmt.Sprintf("tmdb:tv:%d", id), URL: fmt.Sprintf("https://115.com/s/share%d", id), FileName: fmt.Sprintf("Show%d/Show.S01E01.mkv", id), Episodes: []domain.ShareEpisode{{SeasonNumber: 1, EpisodeNumber: 1}}, Result: domain.TmdbIdentifyResult{Success: true, Title: fmt.Sprintf("Show%d", id), Year: 2024, MediaType: "tv", TmdbID: id}}
}

func (fixture *phaseFixture) expectPaths(entry domain.ShareStrmEntry) {
	fixture.entries++
	for _, episode := range entry.Episodes {
		var source domain.ShareStrmSource
		fixture.service.Coordinator().mu.Lock()
		for _, candidate := range fixture.store.sources {
			holder := fixture.service.Coordinator().holders[fileKey(candidate.ID)]
			if holder != nil && holder.writer != "" {
				source = candidate
				break
			}
		}
		fixture.service.Coordinator().mu.Unlock()
		loser := false
		for _, candidate := range fixture.store.sources {
			if candidate.WorkKey != source.WorkKey || candidate.ID == source.ID {
				continue
			}
			for _, candidateEpisode := range candidate.Episodes {
				if candidateEpisode == episode && betterShareStrmSource(candidate, source) {
					loser = true
				}
			}
		}
		if loser {
			continue
		}
		fixture.mock.ExpectQuery(`SELECT pg_try_advisory_lock\(`).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		if fixture.recovery {
			var work string
			fixture.mock.ExpectExec("INSERT INTO t_share_export_dirty_work AS dirty").WithArgs(
				phaseArgument(func(value driver.Value) bool { work = value.(string); return work != "" }),
				phaseArgument(func(value driver.Value) bool {
					var keys []string
					if err := json.Unmarshal([]byte(value.(string)), &keys); err != nil {
						fixture.test.Fatal(err)
					}
					if fixture.externalPlans == nil {
						fixture.externalPlans = map[string][]string{}
					}
					fixture.externalPlans[work] = sortedIncrementalKeys(append(fixture.externalPlans[work], keys...))
					return len(keys) > 0
				}),
			).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		fixture.mock.ExpectQuery(`SELECT pg_try_advisory_lock\(`).WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		rows := sqlmock.NewRows([]string{"owner_key", "export_key"})
		fixture.mock.ExpectQuery("SELECT owner_key,export_key").WithArgs(phaseArgument(func(value driver.Value) bool {
			outputPath := value.(string)
			if fixture.foreign {
				rows.AddRow("foreign:1", "foreign")
			} else {
				for key, state := range fixture.states {
					if state.Path == outputPath {
						rows.AddRow(state.Owner, key)
					}
				}
				if owner := fixture.history[outputPath]; owner != "" {
					rows.AddRow(owner, "")
				}
			}
			return true
		})).WillReturnRows(rows)
		if !fixture.foreign && !fixture.unregistered {
			fixture.mock.ExpectExec("INSERT INTO t_strm_export_history VALUES").WithArgs(sqlmock.AnyArg(), phaseArgument(func(value driver.Value) bool { fixture.history[value.(string)] = "share:default"; return true })).WillReturnResult(sqlmock.NewResult(0, 1))
			fixture.mock.ExpectBegin()
			fixture.mock.ExpectExec("INSERT INTO t_strm_export_history SELECT").WillReturnResult(sqlmock.NewResult(0, 1))
			values := make([]string, 7)
			arguments := make([]driver.Value, 7)
			for index := range arguments {
				position := index
				arguments[index] = phaseArgument(func(value driver.Value) bool {
					values[position] = value.(string)
					if position == 6 {
						fixture.states[values[1]] = dao.ExportState{Owner: values[0], Key: values[1], Path: values[2], Content: values[3], Mapping: values[4], Playback: values[5], Run: values[6]}
						delete(fixture.stale, values[1])
					}
					return true
				})
			}
			fixture.mock.ExpectExec("INSERT INTO t_strm_export_state").WithArgs(arguments...).WillReturnResult(sqlmock.NewResult(0, 1))
			fixture.mock.ExpectCommit()
		}
		fixture.mock.ExpectExec(`SELECT pg_advisory_unlock\(`).WillReturnResult(sqlmock.NewResult(0, 0))
		fixture.mock.ExpectExec(`SELECT pg_advisory_unlock\(`).WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func phaseFiles(test testing.TB, root string) map[string]phaseFile {
	test.Helper()
	files := map[string]phaseFile{}
	err := filepath.Walk(root, func(outputPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		content, err := os.ReadFile(outputPath)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, outputPath)
		if err != nil {
			return err
		}
		files[relative] = phaseFile{Content: string(content), Modified: info.ModTime().UnixNano(), Info: info}
		return nil
	})
	if err != nil {
		test.Fatal(err)
	}
	return files
}

func (fixture *phaseFixture) round(name string, query domain.ShareLibraryQuery, finish bool) phaseResult {
	fixture.test.Helper()
	if err := fixture.service.tasks.Create(name, "strm_generate", name); err != nil {
		fixture.test.Fatal(err)
	}
	fixture.counts = phaseCounts{}
	fixture.finish, fixture.run, fixture.entries = finish, name, 0
	fixture.freshID = 0
	before := phaseFiles(fixture.test, fixture.cfg.OutputPath)
	fixture.recovered = false
	fixture.mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(fixture.recovery))
	if fixture.recovery {
		fixture.mock.ExpectQuery("SELECT config_revision FROM").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
	}
	for directory := fixture.cfg.OutputPath; ; directory = filepath.Dir(directory) {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		if filepath.Dir(directory) == directory {
			break
		}
	}
	fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock\\(hashtextextended\\('share:default',34982\\)\\)").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	fixture.mock.ExpectQuery("SELECT strm_config_id,local_strm_path").WillReturnRows(sqlmock.NewRows([]string{"id", "path"}))
	fixture.mock.ExpectQuery("SELECT output_path FROM").WillReturnRows(sqlmock.NewRows([]string{"path"}))
	fixture.states["sentinel"] = dao.ExportState{Owner: "share:default", Key: "sentinel", Run: "previous"}
	rows := sqlmock.NewRows([]string{"key", "run"})
	keys := make([]string, 0, len(fixture.states))
	for key := range fixture.states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		state := fixture.states[key]
		rows.AddRow(key, state.Run)
	}
	fixture.mock.ExpectQuery("SELECT export_key,last_seen_run_id").WillReturnRows(rows)
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	if finish && len(fixture.store.sources) == 0 {
		fixture.expectFinish()
	}
	ctx := context.Background()
	if fixture.store.cancelAfter > 0 {
		if fixture.store.flagCancel {
			fixture.store.cancel = func() {
				if err := fixture.service.tasks.Cancel(name); err != nil {
					fixture.test.Fatal(err)
				}
			}
		} else {
			ctx, fixture.store.cancel = context.WithCancel(ctx)
			defer fixture.store.cancel()
		}
	}
	start := time.Now()
	err := fixture.service.export(ctx, fixture.cfg, query, name)
	elapsed := time.Since(start).Nanoseconds()
	if mockErr := fixture.mock.ExpectationsWereMet(); mockErr != nil {
		fixture.test.Fatal(mockErr)
	}
	after := phaseFiles(fixture.test, fixture.cfg.OutputPath)
	for outputPath, file := range after {
		old, exists := before[outputPath]
		if !exists {
			fixture.counts.Creates++
		} else if old.Content != file.Content || old.Modified != file.Modified || !os.SameFile(old.Info, file.Info) {
			fixture.counts.Rewrites++
		}
	}
	for outputPath := range before {
		if _, exists := after[outputPath]; !exists {
			fixture.counts.Deletes++
		}
	}
	task, taskErr := fixture.service.tasks.Get(name)
	if taskErr != nil {
		fixture.test.Fatal(taskErr)
	}
	metadata, _ := task["metadata"].(map[string]interface{})
	integer := func(values map[string]interface{}, key string) int {
		value, _ := values[key].(float64)
		return int(value)
	}
	result := phaseResult{Scenario: name, ElapsedNS: elapsed, Counts: fixture.counts, Processed: integer(task, "processed_files"), Success: integer(task, "success_files"), Failed: integer(task, "failed_files"), Added: integer(metadata, "added"), Updated: integer(metadata, "updated"), Skipped: integer(metadata, "skipped"), Conflicts: integer(metadata, "conflicts"), SkippedSources: integer(metadata, "skipped_sources"), Files: after, States: map[string]dao.ExportState{}, Stale: []string{}, Errors: []string{}}
	result.Total, result.Progress, result.ExportedFiles = integer(task, "total_files"), integer(task, "progress"), integer(metadata, "exported_files")
	result.Status, _ = task["status"].(string)
	if err != nil {
		result.Error = err.Error()
	}
	if values, ok := metadata["errors"].([]interface{}); ok {
		for _, value := range values {
			result.Errors = append(result.Errors, strings.ReplaceAll(value.(string), fixture.cfg.OutputPath, "<output>"))
		}
	}
	for key, state := range fixture.states {
		if state.Path != "" {
			state.Path, _ = filepath.Rel(fixture.cfg.OutputPath, state.Path)
		}
		state.Content = ""
		result.States[key] = state
	}
	for key := range fixture.stale {
		result.Stale = append(result.Stale, key)
	}
	sort.Strings(result.Stale)
	return result
}

func (fixture *phaseFixture) expectFinish() {
	fixture.finish = false
	for key, state := range fixture.states {
		if state.Run != fixture.run {
			fixture.mock.ExpectExec("UPDATE t_strm_export_state SET state='stale'").WithArgs("share:default", key, phaseArgument(func(value driver.Value) bool {
				if value != state.Run {
					return false
				}
				fixture.stale[key] = true
				return true
			})).WillReturnResult(sqlmock.NewResult(0, 1))
		}
	}
	if fixture.recovery {
		count := int64(1)
		if fixture.recoveryRace {
			count = 0
		}
		fixture.mock.ExpectExec("UPDATE t_share_export_consumer SET legacy_outputs_reconciled=true").WithArgs(phaseArgument(func(value driver.Value) bool {
			fixture.recovered = !fixture.recoveryRace
			return value == int64(1)
		})).WillReturnResult(sqlmock.NewResult(0, count))
	}
}

func phaseScenarios(test testing.TB) []phaseResult {
	test.Helper()
	fixture := newPhaseFixture(test, true)
	fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
	results := []phaseResult{}
	run := func(name string, query domain.ShareLibraryQuery, finish bool) phaseResult {
		result := fixture.round(name, query, finish)
		results = append(results, result)
		return result
	}
	result := run("initial", domain.ShareLibraryQuery{}, true)
	if result.Processed != 1 || result.Added != 1 || result.Counts.Creates != 1 || result.Error != "" {
		test.Fatalf("首次: %+v", result)
	}
	result = run("unchanged", domain.ShareLibraryQuery{}, true)
	if result.Skipped != 1 || result.Counts.Rewrites != 0 || result.Counts.Creates != 0 {
		test.Fatalf("未变: %+v", result)
	}
	fixture.store.sources = append(fixture.store.sources, phaseSource(2))
	result = run("added", domain.ShareLibraryQuery{}, true)
	if result.Processed != 2 || result.Added != 1 || result.Skipped != 1 {
		test.Fatalf("新增: %+v", result)
	}
	fixture.store.sources = fixture.store.sources[1:]
	result = run("share_cancelled", domain.ShareLibraryQuery{}, true)
	if result.Processed != 1 || result.Counts.Deletes != 0 || !fixture.stale["1:1:1"] {
		test.Fatalf("取消分享: %+v", result)
	}
	fixture.store.sources = append([]domain.ShareStrmSource{phaseSource(1)}, fixture.store.sources...)
	result = run("share_restored", domain.ShareLibraryQuery{}, true)
	if result.Skipped != 2 || fixture.stale["1:1:1"] {
		test.Fatalf("恢复分享: %+v", result)
	}
	fixture.store.sources[0].Episodes = []domain.ShareEpisode{{SeasonNumber: 2, EpisodeNumber: 3}}
	result = run("episode_remap", domain.ShareLibraryQuery{}, true)
	if result.Added != 1 || !fixture.stale["1:1:1"] || result.Counts.Deletes != 0 {
		test.Fatalf("改季集: %+v", result)
	}
	second := fixture.store.sources[0]
	second.ID, second.ShareID, second.ShareName, second.URL = 3, 3, "第二分享", "https://115.com/s/second"
	fixture.store.sources = append(fixture.store.sources, second)
	result = run("second_share", domain.ShareLibraryQuery{}, true)
	if result.Processed != 3 || result.Added != 0 || fixture.stale["1:2:3"] {
		test.Fatalf("后缀改变: %+v", result)
	}
	fixture.cfg.BaseURL = "https://changed.example.test"
	result = run("content_changed", domain.ShareLibraryQuery{}, true)
	if result.Updated != 2 || result.Counts.Rewrites != 2 {
		test.Fatalf("真实改写: %+v", result)
	}
	result = run("filtered", domain.ShareLibraryQuery{WorkKey: "tmdb:tv:2"}, false)
	if result.Processed != 1 || result.Error != "" {
		test.Fatalf("筛选: %+v", result)
	}
	result = run("file_ids_expanded", domain.ShareLibraryQuery{FileIDs: []int{3}}, false)
	if result.Processed != 2 || result.Skipped != 1 {
		test.Fatalf("文件筛选未扩展同作品: %+v", result)
	}
	fixture.store.strmMemory.fileErr = errors.New("清单登记失败")
	result = run("failed", domain.ShareLibraryQuery{}, false)
	if result.Failed != 2 || len(result.Errors) != 2 || result.Error == "" || result.Skipped != 2 {
		test.Fatalf("部分失败: %+v", result)
	}
	fixture.store.strmMemory.fileErr = nil
	fixture.foreign = true
	result = run("ownership_rejected", domain.ShareLibraryQuery{}, false)
	if result.Conflicts != 2 || result.Failed != 2 || result.Counts.Rewrites != 0 {
		test.Fatalf("归属拒绝: %+v", result)
	}
	fixture.foreign = false
	fixture.store.cancelAfter = 1
	result = run("interrupted", domain.ShareLibraryQuery{}, false)
	if result.Processed != 1 || result.Error != context.Canceled.Error() {
		test.Fatalf("中断进度: %+v", result)
	}
	fixture.service = NewShareStrmService(phaseReader{fixture.store}, nil, nil, nil, &OrganizeService{}, fixture.service.tasks, fixture.service.categories, nil, nil)
	fixture.service.SetExportDatabase(fixture.db)
	result = run("restart_rerun", domain.ShareLibraryQuery{}, true)
	if result.Processed != 3 || result.Skipped != 2 || result.Counts.Rewrites != 0 {
		test.Fatalf("重启是重跑: %+v", result)
	}
	fixture.store.cancelAfter, fixture.store.flagCancel = 1, true
	result = run("task_cancelled", domain.ShareLibraryQuery{}, false)
	if result.Processed != 1 || result.Error != context.Canceled.Error() {
		test.Fatalf("任务取消: %+v", result)
	}
	fixture.store.sources = nil
	result = run("empty_full", domain.ShareLibraryQuery{}, true)
	if result.Processed != 0 || result.Counts.Deletes != 0 || result.Error != "" {
		test.Fatalf("空全量: %+v", result)
	}

	fixture = newPhaseFixture(test, true)
	for id := 1; id <= 205; id++ {
		fixture.store.sources = append(fixture.store.sources, phaseSource(id))
	}
	result = run("bulk_seed", domain.ShareLibraryQuery{}, true)
	if result.Processed != 205 || result.Added != 205 || result.Counts.Creates != 205 {
		test.Fatalf("跨页基线: %+v", result)
	}
	result = run("bulk_unchanged", domain.ShareLibraryQuery{}, true)
	if result.Processed != 205 || result.Skipped != 205 || result.Counts.Rewrites != 0 {
		test.Fatalf("跨页未变: %+v", result)
	}
	fixture.store.cancelAfter = 105
	result = run("bulk_interrupted", domain.ShareLibraryQuery{}, false)
	if result.Processed != 105 || result.Skipped != 105 || result.Error != context.Canceled.Error() {
		test.Fatalf("跨批次取消: %+v", result)
	}
	fixture.store.readErr = errors.New("分页读取失败")
	result = run("page_error", domain.ShareLibraryQuery{}, false)
	if result.Processed != 100 || result.Skipped != 100 || result.Error != "分页读取失败" {
		test.Fatalf("分页失败: %+v", result)
	}
	fixture.store.readErr = nil
	result = run("bulk_restart", domain.ShareLibraryQuery{}, true)
	if result.Processed != 205 || result.Skipped != 205 {
		test.Fatalf("跨页重跑: %+v", result)
	}
	fixture.store.strmMemory.fileErr = errors.New("批量来源清单登记失败")
	result = run("bulk_partial_failure", domain.ShareLibraryQuery{}, false)
	if result.Processed != 205 || result.Failed != 205 || result.Skipped != 205 || len(result.Errors) != 100 || result.Error == "" {
		test.Fatalf("失败计数与错误上限: %+v", result)
	}

	fixture = newPhaseFixture(test, true)
	source := phaseSource(1)
	source.Episodes = append(source.Episodes, domain.ShareEpisode{SeasonNumber: 1, EpisodeNumber: 2})
	fixture.store.sources = []domain.ShareStrmSource{source}
	result = run("multiple_episodes", domain.ShareLibraryQuery{}, true)
	if result.Processed != 1 || result.Added != 2 || result.Counts.Creates != 2 {
		test.Fatalf("一源多路径: %+v", result)
	}
	fixture = newPhaseFixture(test, true)
	db, mock := phaseMockDatabase(test, &fixture.counts)
	fixture.service.store = &phaseSQLStore{ShareRecordDAO: dao.NewShareRecordDAO(db), fixture: fixture, mock: mock}
	for id := 1; id <= 105; id++ {
		fixture.store.sources = append(fixture.store.sources, phaseSource(id))
	}
	result = run("sql_seed", domain.ShareLibraryQuery{}, true)
	if result.Processed != 105 || result.Added != 105 || result.Counts.MemoryReads != 0 || result.Counts.MemoryWrites != 0 {
		test.Fatalf("真实 DAO 基线: %+v", result)
	}
	result = run("sql_unchanged", domain.ShareLibraryQuery{}, true)
	if result.Skipped != 105 || result.Counts.Rewrites != 0 {
		test.Fatalf("真实 DAO 未变: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		test.Fatal(err)
	}
	return results
}

func TestShareStrmPhase01Scenarios(t *testing.T) {
	results := phaseScenarios(t)
	if output := os.Getenv("ESTRM_PHASE01_RESULTS"); output != "" {
		raw, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestShareStrmPhase01FallbackAndLockedRevalidation(t *testing.T) {
	results := []phaseResult{}
	for _, scenario := range []string{"fallback", "fresh_episode", "fresh_version", "fresh_work", "fresh_missing", "fresh_error", "conflict_refresh"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newPhaseFixture(t, scenario != "fallback")
			source := phaseSource(1)
			fixture.store.sources = []domain.ShareStrmSource{source}
			if scenario == "fallback" {
				second := source
				second.ID, second.ShareID, second.ShareName, second.URL = 2, 2, "第二分享", "https://115.com/s/second"
				fixture.store.sources = append(fixture.store.sources, second)
			}
			fixture.store.fresh = func(fresh domain.ShareStrmSource) (domain.ShareStrmSource, error) {
				switch scenario {
				case "fresh_episode":
					fresh.Episodes = []domain.ShareEpisode{{SeasonNumber: 3, EpisodeNumber: 4}}
				case "fresh_version":
					fresh.FileVersion++
				case "fresh_work":
					fresh.WorkKey = "different"
				case "fresh_missing":
					return fresh, sql.ErrNoRows
				case "fresh_error":
					return fresh, errors.New("来源重读失败")
				case "conflict_refresh":
					second := fresh
					second.ID, second.ShareID, second.ShareName = 2, 2, "新并发分享"
					fixture.store.sources = append(fixture.store.sources, second)
				}
				return fresh, nil
			}
			result := fixture.round(scenario, domain.ShareLibraryQuery{WorkKey: source.WorkKey}, false)
			results = append(results, result)
			switch scenario {
			case "fallback":
				if result.Added != 1 {
					t.Fatalf("回退预扫缺失: %+v", result)
				}
			case "fresh_episode":
				if _, ok := result.States["1:3:4"]; !ok {
					t.Fatalf("未使用新季集: %+v", result)
				}
			case "fresh_version", "fresh_work", "fresh_missing":
				if result.SkippedSources != 1 || len(result.Files) != 0 {
					t.Fatalf("陈旧来源未跳过: %+v", result)
				}
			case "fresh_error":
				if result.Failed != 1 || len(result.Errors) != 1 {
					t.Fatalf("重读错误丢失: %+v", result)
				}
			case "conflict_refresh":
				if _, ok := result.States["1:1:1"]; !ok {
					t.Fatalf("未持锁重算冲突: %+v", result)
				}
			}
		})
	}
	if output := os.Getenv("ESTRM_PHASE01_RESULTS"); output != "" {
		raw, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output+".revalidation.json", append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkShareStrmPhase01(b *testing.B) {
	for iteration := 0; iteration < b.N; iteration++ {
		results := phaseScenarios(b)
		for _, result := range results {
			b.ReportMetric(float64(result.ElapsedNS), result.Scenario+"-ns/round")
		}
		if output := os.Getenv("ESTRM_PHASE01_BENCH_RESULTS"); output != "" {
			raw, err := json.Marshal(results)
			if err != nil {
				b.Fatal(err)
			}
			file, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				b.Fatal(err)
			}
			_, writeErr := file.Write(append(raw, '\n'))
			if err = errors.Join(writeErr, file.Close()); err != nil {
				b.Fatal(err)
			}
		}
	}
}
