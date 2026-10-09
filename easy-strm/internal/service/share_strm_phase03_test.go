package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

type phase03Checkpoints struct {
	*incrementalMemory
	schemaErr, inputErr, fullErr error
	fulls, requirements          int
	afterComplete                func()
}

func (store *phase03Checkpoints) CheckSchema(context.Context) error { return store.schemaErr }
func (store *phase03Checkpoints) ReadInput(context.Context) (domain.ShareExportInput, error) {
	return store.input, store.inputErr
}
func (store *phase03Checkpoints) CompleteWork(ctx context.Context, dirty domain.ShareExportDirty, revision int64, run string, states []domain.ShareExportSourceState, snapshot dao.ExportSnapshot) (int, error) {
	marked, err := store.incrementalMemory.CompleteWork(ctx, dirty, revision, run, states, snapshot)
	if err == nil && store.afterComplete != nil {
		store.afterComplete()
	}
	return marked, err
}

func (store *phase03Checkpoints) RequireBaseline(ctx context.Context) error {
	store.requirements++
	return store.incrementalMemory.RequireBaseline(ctx)
}
func (store *phase03Checkpoints) CompleteReconciliation(ctx context.Context, input domain.ShareExportInput, fingerprint string) error {
	if store.fullErr != nil {
		return store.fullErr
	}
	if input.ConfigRevision != store.input.ConfigRevision {
		return dao.ErrShareExportChanged
	}
	store.input.LegacyOutputsReconciled = true
	return store.Prepare(ctx, input, fingerprint)
}

func phase03Fixture(t *testing.T) (*phaseFixture, *phase03Checkpoints, *ShareStrmScheduledService) {
	fixture, memory, _ := newIncrementalFixture(t)
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { client.Close() })
	fixture.service.tasks = NewTaskService(dao.NewTaskRedisDAO(client))
	store := &phase03Checkpoints{incrementalMemory: memory}
	runner := NewShareStrmScheduledService(fixture.service, store)
	runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
		store.fulls++
		fingerprint, err := shareExportFingerprint(input)
		if err != nil {
			return err
		}
		return store.CompleteReconciliation(ctx, input, fingerprint)
	}
	return fixture, store, runner
}

func phase03Run(t *testing.T, fixture *phaseFixture, runner *ShareStrmScheduledService, mode, id string, ctx context.Context) error {
	t.Helper()
	if err := fixture.service.tasks.Create(id, "strm_generate", id); err != nil {
		t.Fatal(err)
	}
	for directory := fixture.cfg.OutputPath; ; directory = filepath.Dir(directory) {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		if filepath.Dir(directory) == directory {
			break
		}
	}
	fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock\\(hashtextextended\\('share:default',34982\\)\\)").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	err := runner.Run(ctx, mode, id)
	if expectationErr := fixture.mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
	return err
}

func TestShareStrmScheduledUntrustedPromotesExplicitReconciliation(t *testing.T) {
	for _, reason := range []string{"legacy", "required", "fingerprint", "revision"} {
		t.Run(reason, func(t *testing.T) {
			fixture, store, runner := phase03Fixture(t)
			switch reason {
			case "legacy":
				store.input.LegacyOutputsReconciled = false
			case "required":
				store.input.BaselineState = "required"
			case "fingerprint":
				store.input.Fingerprint = "untrusted"
			case "revision":
				store.input.ConfigRevision++
			}
			if err := phase03Run(t, fixture, runner, "incremental", "cron_fallback", context.Background()); err != nil {
				t.Fatal(err)
			}
			if store.fulls != 1 || store.fanouts != 1 || store.input.BaselineState != "ready" {
				t.Fatalf("full=%d seed=%d state=%s", store.fulls, store.fanouts, store.input.BaselineState)
			}
			task, _ := fixture.service.tasks.Get("cron_fallback")
			metadata := task["metadata"].(map[string]interface{})
			if metadata["requested_mode"] != "incremental" || metadata["effective_mode"] != "reconciliation" || metadata["fallback_reason"] == "" {
				t.Fatal(metadata)
			}
		})
	}
}

func TestShareStrmScheduledBuildingRecoveryDoesNotRepeatFullOrSeed(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	first, second := phaseSource(1), phaseSource(2)
	second.WorkKey = "movie:2"
	fixture.store.sources = []domain.ShareStrmSource{first, second}
	store.beforeComplete = func() {
		if store.completions == 1 {
			store.commitErr = errors.New("completed-work transaction interrupted")
		}
	}
	if err := phase03Run(t, fixture, runner, "reconciliation", "cron_interrupted", context.Background()); err == nil {
		t.Fatal("expected interruption")
	}
	if store.fulls != 1 || store.fanouts != 1 || store.input.BaselineState != "building" {
		t.Fatalf("lost prepared phase: %+v", store)
	}
	if store.completions != 1 {
		t.Fatalf("expected one durable work: %d", store.completions)
	}
	previousReads := store.reads
	store.commitErr = nil
	store.beforeComplete = nil
	if err := phase03Run(t, fixture, runner, "reconciliation", "cron_resumed", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.fulls != 1 || store.fanouts != 1 || store.completions != 2 {
		t.Fatalf("full=%d seed=%d complete=%d", store.fulls, store.fanouts, store.completions)
	}
	if store.reads != previousReads+1 {
		t.Fatal("repeated an acknowledged work")
	}
}

func phase03ExpectFull(t *testing.T, fixture *phaseFixture, store *phase03Checkpoints) {
	t.Helper()
	fixture.recovery = true
	fixture.mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(true))
	fixture.mock.ExpectQuery("SELECT config_revision FROM").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(store.input.ConfigRevision + 1))
	fixture.mock.ExpectQuery("SELECT strm_config_id,local_strm_path").WillReturnRows(sqlmock.NewRows([]string{"id", "path"}))
	fixture.mock.ExpectQuery("SELECT output_path FROM").WillReturnRows(sqlmock.NewRows([]string{"path"}))
	rows := sqlmock.NewRows([]string{"key", "run"})
	for key, state := range fixture.states {
		rows.AddRow(key, state.Run)
	}
	fixture.mock.ExpectQuery("SELECT export_key,last_seen_run_id").WillReturnRows(rows)
}

func TestShareStrmScheduledActualFullDiscoversOrphanAndDiskDrift(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
	runner.full = func(ctx context.Context, input domain.ShareExportInput, id string) error {
		err := runner.reconcile(ctx, input, id)
		fixture.recovery = false
		return err
	}
	fixture.states["unresolvable-legacy-orphan"] = dao.ExportState{Owner: "share:default", Key: "unresolvable-legacy-orphan", Run: "previous"}
	phase03ExpectFull(t, fixture, store)
	fixture.mock.ExpectExec("UPDATE t_strm_export_state SET state='stale'").WithArgs("share:default", "unresolvable-legacy-orphan", "previous").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := phase03Run(t, fixture, runner, "reconciliation", "cron_actual_full", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.fanouts != 1 || store.completions != 1 {
		t.Fatal("full did not seed and consume work")
	}
	task, err := fixture.service.tasks.Get("cron_actual_full")
	if err != nil {
		t.Fatal(err)
	}
	if metadata := task["metadata"].(map[string]interface{}); metadata["reconciliation_unseen_outputs"] != float64(1) || metadata["phase"] != "consuming" {
		t.Fatal(metadata)
	}
	var path string
	for _, state := range fixture.states {
		if state.Path != "" {
			path = state.Path
		}
	}
	if path == "" {
		t.Fatal("missing full output")
	}
	wanted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("disk drift\n"), 0644); err != nil {
		t.Fatal(err)
	}
	delete(fixture.states, "unresolvable-legacy-orphan")
	phase03ExpectFull(t, fixture, store)
	if err = phase03Run(t, fixture, runner, "reconciliation", "cron_drift", context.Background()); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != string(wanted) {
		t.Fatalf("drift not repaired: %q %v", actual, err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	phase03ExpectFull(t, fixture, store)
	if err = phase03Run(t, fixture, runner, "reconciliation", "cron_missing_disk", context.Background()); err != nil {
		t.Fatal(err)
	}
	actual, err = os.ReadFile(path)
	if err != nil || string(actual) != string(wanted) {
		t.Fatalf("missing file not rebuilt: %q %v", actual, err)
	}
}

func TestShareStrmScheduledFullFailureNeverPreparesOrAcknowledges(t *testing.T) {
	for _, reason := range []string{"cancel", "foreign", "unregistered", "completion"} {
		t.Run(reason, func(t *testing.T) {
			fixture, store, runner := phase03Fixture(t)
			fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
			runner.full = runner.reconcile
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch reason {
			case "cancel":
				fixture.store.cancelAfter = 1
				fixture.store.cancel = cancel
			case "foreign":
				fixture.foreign = true
			case "unregistered":
				fixture.unregistered = true
				source := fixture.store.sources[0]
				source.Result.SeasonNumber, source.Result.EpisodeNumber = 1, 1
				relative, err := fixture.service.strmRelativePath(source, domain.ShareFileInfo{Path: source.FileName}, nil)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(fixture.cfg.OutputPath, relative)
				if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte("foreign content\n"), 0644); err != nil {
					t.Fatal(err)
				}
			case "completion":
				store.fullErr = errors.New("atomic preparation failed")
			}
			phase03ExpectFull(t, fixture, store)
			err := phase03Run(t, fixture, runner, "reconciliation", "cron_full_failure", ctx)
			if err == nil {
				t.Fatal("failure swallowed")
			}
			if reason == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if store.fanouts != 0 || store.completions != 0 || store.input.BaselineState != "required" {
				t.Fatalf("false prepared phase: %+v", store)
			}
		})
	}
}

func TestShareStrmScheduledCancellationAfterCompletedWorkResumesPending(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	fixture.store.sources = []domain.ShareStrmSource{phaseSource(1), phaseSource(2)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.afterComplete = cancel
	err := phase03Run(t, fixture, runner, "reconciliation", "cron_cancel_boundary", ctx)
	if !errors.Is(err, context.Canceled) || store.completions != 1 || store.input.BaselineState != "building" {
		t.Fatalf("%v complete=%d", err, store.completions)
	}
	task, taskErr := fixture.service.tasks.Get("cron_cancel_boundary")
	if taskErr != nil || task["processed_files"] != float64(1) {
		t.Fatalf("lost completed-boundary progress: %v %v", task, taskErr)
	}
	store.afterComplete = nil
	if err = phase03Run(t, fixture, runner, "incremental", "cron_resume_boundary", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.fulls != 1 || store.fanouts != 1 || store.completions != 2 {
		t.Fatal("repeated full or lost pending")
	}
}

func TestShareStrmScheduledTrustedZeroDirtyDoesNotReadFullOrSeed(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	if err := phase03Run(t, fixture, runner, "incremental", "cron_zero", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.fulls != 0 || store.fanouts != 0 || store.reads != 0 || fixture.counts.SourcePages != 0 {
		t.Fatal("stable incremental scanned full library")
	}
	task, err := fixture.service.tasks.Get("cron_zero")
	if err != nil {
		t.Fatal(err)
	}
	metadata := task["metadata"].(map[string]interface{})
	if metadata["processed_works"] != float64(0) || metadata["effective_mode"] != "incremental" {
		t.Fatal(metadata)
	}
}

func TestShareStrmScheduledUnpreparedRetryDoesNotRepeatedlyInvalidate(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	store.fullErr = errors.New("full preparation interrupted")
	if err := phase03Run(t, fixture, runner, "reconciliation", "cron_prepare_fail", context.Background()); err == nil {
		t.Fatal("expected failed preparation")
	}
	revision := store.input.ConfigRevision
	if store.requirements != 1 || store.input.BaselineState != "required" || store.fanouts != 0 {
		t.Fatal("false prepared baseline")
	}
	store.fullErr = nil
	if err := phase03Run(t, fixture, runner, "incremental", "cron_prepare_retry", context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.input.ConfigRevision != revision || store.requirements != 1 || store.fanouts != 1 || store.fulls != 2 {
		t.Fatal("retry invalidated again or did not finish preparation")
	}
}

func TestShareStrmScheduledFullCannotIgnoreMissingSchema(t *testing.T) {
	fixture, store, runner := phase03Fixture(t)
	runner.full = runner.reconcile
	fixture.mock.ExpectQuery("SELECT strm_config_id,local_strm_path").WillReturnRows(sqlmock.NewRows([]string{"id", "path"}))
	fixture.mock.ExpectQuery("SELECT output_path FROM").WillReturnRows(sqlmock.NewRows([]string{"path"}))
	fixture.mock.ExpectQuery("SELECT export_key,last_seen_run_id").WillReturnRows(sqlmock.NewRows([]string{"key", "run"}))
	fixture.mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"present"}).AddRow(false))
	err := phase03Run(t, fixture, runner, "reconciliation", "cron_schema_lost", context.Background())
	if !errors.Is(err, dao.ErrShareExportSchemaMissing) || store.fanouts != 0 || store.completions != 0 {
		t.Fatalf("scheduled full bypassed schema: %v", err)
	}
}

func TestShareStrmIncrementalRebuildResumesBuildingWithoutReseeding(t *testing.T) {
	fixture, store, worker := newIncrementalFixture(t)
	fixture.store.sources = []domain.ShareStrmSource{phaseSource(1)}
	store.input.BaselineState = "required"
	if err := store.Prepare(context.Background(), store.input, store.input.Fingerprint); err != nil {
		t.Fatal(err)
	}
	revision := store.input.ConfigRevision
	for directory := fixture.cfg.OutputPath; ; directory = filepath.Dir(directory) {
		fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock_shared").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
		if filepath.Dir(directory) == directory {
			break
		}
	}
	fixture.mock.ExpectQuery("SELECT pg_try_advisory_lock\\(hashtextextended\\('share:default',34982\\)\\)").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))
	fixture.mock.ExpectExec("SELECT pg_advisory_unlock_all").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := worker.Rebuild(context.Background(), "rebuild_resumed"); err != nil {
		t.Fatal(err)
	}
	if store.input.ConfigRevision != revision || store.fanouts != 1 || store.completions != 1 {
		t.Fatal("Rebuild invalidated and reseeded prepared revision")
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareStrmScheduledPreflightFailsClosed(t *testing.T) {
	for _, reason := range []string{"schema", "input", "protocol", "settings", "state", "cancel"} {
		t.Run(reason, func(t *testing.T) {
			fixture, store, runner := phase03Fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch reason {
			case "schema":
				store.schemaErr = dao.ErrShareExportSchemaMissing
			case "input":
				store.inputErr = errors.New("invalid input")
			case "protocol":
				store.input.ProtocolVersion = 99
			case "settings":
				store.input.Settings.BaseURL = "invalid"
			case "state":
				store.input.BaselineState = "unknown"
			case "cancel":
				cancel()
			}
			if err := runner.Run(ctx, "incremental", "cron_fail"); err == nil {
				t.Fatal("expected fail closed")
			}
			if store.fulls != 0 || store.fanouts != 0 || store.requirements != 0 {
				t.Fatal("preflight side effects")
			}
			if err := fixture.mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
