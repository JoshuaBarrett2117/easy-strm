package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
)

type baselineAdoptionMemory struct {
	*phase03Checkpoints
	mu           sync.Mutex
	adoptions    int
	failure      error
	beforeAdopt  func()
	afterCommit  func()
	checkFailure error
	seedStates   []domain.ShareExportSourceState
}

func (store *baselineAdoptionMemory) CheckSchema(context.Context) error { return store.checkFailure }
func (store *baselineAdoptionMemory) ReadInput(context.Context) (domain.ShareExportInput, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.input, nil
}
func (store *baselineAdoptionMemory) AdoptBaseline(ctx context.Context, request domain.ShareBaselineAdoptionRequest, fingerprint, id string, beforeCommit ...func() error) (domain.ShareBaselineAdoptionResult, error) {
	if store.beforeAdopt != nil {
		store.beforeAdopt()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	result := domain.ShareBaselineAdoptionResult{TaskID: id, ManualConfirmation: "人工确认采纳"}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if store.failure != nil {
		return result, store.failure
	}
	if store.input.ConfigRevision != request.Expected["config_revision"] || store.input.Fingerprint != fingerprint {
		return result, dao.ErrShareExportChanged
	}
	for _, guard := range beforeCommit {
		if err := guard(); err != nil {
			return result, err
		}
	}
	if store.input.BaselineState == "ready" {
		result.AlreadyReady = true
	} else {
		store.adoptions++
		for _, state := range store.seedStates {
			store.states[state.SourceFileID] = state
		}
		for key, dirty := range store.dirty {
			store.acked[key] = dirty.Revision
			result.AcknowledgedWorks++
		}
		store.input.BaselineState = "ready"
		store.input.LegacyOutputsReconciled = true
	}
	result.BaselineState, result.CompletedRevision = "ready", store.input.ConfigRevision
	if store.afterCommit != nil {
		store.afterCommit()
	}
	return result, nil
}

func baselineFlowFixture(test *testing.T) (*phaseFixture, *baselineAdoptionMemory, *ShareStrmScheduledService, domain.ShareBaselineAdoptionRequest) {
	fixture, checkpoints, runner := phase03Fixture(test)
	checkpoints.input.BaselineState = "building"
	checkpoints.input.LegacyOutputsReconciled = false
	store := &baselineAdoptionMemory{phase03Checkpoints: checkpoints}
	request := domain.ShareBaselineAdoptionRequest{ManualConfirmation: "人工确认采纳", Reason: "mock DBA 独立核验", DBAAttestation: "mock-review-42", ExpectedOutputPath: fixture.cfg.OutputPath, ActorUserID: 42, Expected: map[string]int64{"config_revision": 1, "prepared_revision": 1, "completed_revision": 0, "selection_count": 1, "output_count": 1, "pending_count": 1, "dirty_revision_sum": 2, "selection_revision_sum": 1}}
	store.dirty["1"] = domain.ShareExportDirty{WorkKey: "1", Revision: 2}
	store.seedStates = []domain.ShareExportSourceState{{SourceFileID: 1, ShareID: 1, MediaID: 1, WorkKey: "1", State: "active", ExportKeys: []string{"1:1:1"}}}
	fixture.states["1:1:1"] = dao.ExportState{Owner: "share:default", Key: "1:1:1", Run: "adopt"}
	return fixture, store, runner, request
}

func TestShareBaselineAdoptionActualIncrementalEntryAndFutureDeletion(test *testing.T) {
	fixture, store, runner, request := baselineFlowFixture(test)
	if err := phase03Run(test, fixture, runner, "incremental", "before_adopt", context.Background()); !errors.Is(err, dao.ErrShareExportBaselineRequired) {
		test.Fatalf("untrusted baseline allowed: %v", err)
	}
	result, err := runShareBaselineAdoption(context.Background(), store, request, "adopt")
	if err != nil || result.AcknowledgedWorks != 1 {
		test.Fatalf("adoption: %+v %v", result, err)
	}
	if err := phase03Run(test, fixture, runner, "incremental", "after_adopt", context.Background()); err != nil {
		test.Fatalf("actual scheduled incremental rejected adopted baseline: %v", err)
	}
	if store.fulls != 0 || store.fanouts != 0 {
		test.Fatal("incremental must not fall back or seed")
	}
	if store.states[1].State != "active" || len(store.states[1].ExportKeys) != 1 {
		test.Fatal("adoption must seed source checkpoint associations")
	}
	worker := NewShareStrmIncrementalService(fixture.service, store.incrementalMemory)
	if err := runIncrementalFixture(test, fixture, worker, "direct_after_adopt"); err != nil {
		test.Fatalf("actual worker entry rejected adoption: %v", err)
	}
	store.bump("1")
	result, err = runShareBaselineAdoption(context.Background(), store, request, "repeat")
	if err != nil || !result.AlreadyReady || result.AcknowledgedWorks != 0 || store.acked["1"] == store.dirty["1"].Revision {
		test.Fatalf("repeat consumed new incremental work: %+v %v", result, err)
	}
	if err := phase03Run(test, fixture, runner, "incremental", "future_delete", context.Background()); err != nil {
		test.Fatal(err)
	}
	if !fixture.stale["1:1:1"] || store.states[1].State != "stale" || store.completions != 1 {
		test.Fatal("future deletion lost adopted source checkpoint keys")
	}
	if store.fulls != 0 || store.fanouts != 0 {
		test.Fatal("ordinary incremental semantics changed")
	}
}

func TestShareBaselineAdoptionServiceRejectsChangedInputAndSchema(test *testing.T) {
	for _, scenario := range []string{"schema", "path", "revision", "transaction", "cancel"} {
		test.Run(scenario, func(test *testing.T) {
			_, store, _, request := baselineFlowFixture(test)
			ctx := context.Background()
			switch scenario {
			case "schema":
				store.checkFailure = dao.ErrShareExportSchemaMissing
			case "path":
				request.ExpectedOutputPath = "/isolated/other"
			case "revision":
				store.input.ConfigRevision++
			case "transaction":
				store.failure = errors.New("transaction failed")
			case "cancel":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, err := runShareBaselineAdoption(ctx, store, request, "reject"); err == nil {
				test.Fatal("must reject")
			}
			if store.adoptions != 0 || store.input.BaselineState != "building" || store.acked["1"] != 0 {
				test.Fatal("failed adoption changed baseline")
			}
		})
	}
}

func TestShareBaselineAdoptionTaskAuditAndConcurrentRepeat(test *testing.T) {
	fixture, store, _, request := baselineFlowFixture(test)
	ids := make([]string, 2)
	for index := range ids {
		id, err := fixture.service.startBaselineAdoptionContext(context.Background(), request, store)
		if err != nil {
			test.Fatal(err)
		}
		ids[index] = id
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, id := range ids {
		for {
			task, err := fixture.service.tasks.Get(id)
			if err != nil {
				test.Fatal(err)
			}
			if task["status"] == "completed" {
				metadata := task["metadata"].(map[string]interface{})
				if metadata["manual_confirmation"] != "人工确认采纳" || metadata["confirmed_by"] != float64(42) || metadata["baseline_adoption"] == nil || metadata["adoption_request"] == nil {
					test.Fatalf("missing audit: %+v", metadata)
				}
				break
			}
			if time.Now().After(deadline) || task["status"] == "failed" {
				test.Fatalf("task did not complete: %+v", task)
			}
			time.Sleep(time.Millisecond)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.adoptions != 1 {
		test.Fatalf("concurrent repeats adopted %d times", store.adoptions)
	}
}
