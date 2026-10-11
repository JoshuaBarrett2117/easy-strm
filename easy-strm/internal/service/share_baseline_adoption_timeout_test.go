package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"easy-strm/internal/domain"
)

func TestShareBaselineAdoptionTimeout(test *testing.T) {
	for _, scenario := range []struct {
		name, configured string
		override         *int
		wanted           int
		invalid          bool
	}{
		{name: "default", wanted: 20},
		{name: "configured", configured: "30", wanted: 30},
		{name: "configured-unlimited", configured: "0", wanted: 0},
		{name: "configured-max", configured: "43200", wanted: 43200},
		{name: "negative", configured: "-1", invalid: true},
		{name: "over-max", configured: "43201", invalid: true},
		{name: "overflow", configured: "9223372036854775808", invalid: true},
		{name: "fraction", configured: "20.5", invalid: true},
		{name: "malformed", configured: "minutes", invalid: true},
		{name: "override", configured: "30", override: baselineTimeoutPointer(7), wanted: 7},
		{name: "explicit-zero", configured: "30", override: baselineTimeoutPointer(0), wanted: 0},
		{name: "override-invalid-config", configured: "bad", override: baselineTimeoutPointer(4), wanted: 4},
		{name: "negative-override", override: baselineTimeoutPointer(-1), invalid: true},
		{name: "over-max-override", override: baselineTimeoutPointer(43201), invalid: true},
		{name: "max-override", override: baselineTimeoutPointer(43200), wanted: 43200},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			settings := &shareSettingsMemory{values: map[string]*domain.SystemConfig{}}
			if scenario.configured != "" {
				settings.values[shareBaselineTimeoutKey] = &domain.SystemConfig{ConfigVal: scenario.configured}
			}
			service := &ShareStrmService{settings: settings}
			request := domain.ShareBaselineAdoptionRequest{TimeoutMinutes: scenario.override}
			minutes, err := service.baselineAdoptionTimeout(context.Background(), request)
			if (err != nil) != scenario.invalid || (!scenario.invalid && minutes != scenario.wanted) {
				test.Fatalf("minutes=%d error=%v", minutes, err)
			}
			if scenario.invalid {
				return
			}
			parent, cancelParent := context.WithCancel(context.Background())
			defer cancelParent()
			ctx, cancel := newShareTaskContext(parent, minutes)
			defer cancel()
			deadline, timed := ctx.Deadline()
			if timed != (minutes != 0) || (timed && time.Until(deadline) < time.Duration(minutes)*time.Minute-time.Second) {
				test.Fatal("wrong deadline or duration overflow")
			}
			cancelParent()
			if !errors.Is(ctx.Err(), context.Canceled) {
				test.Fatal("timeout context lost cancellation")
			}
		})
	}
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&ShareStrmService{}).baselineAdoptionTimeout(parent, domain.ShareBaselineAdoptionRequest{}); !errors.Is(err, context.Canceled) {
		test.Fatal("cancelled submission was accepted")
	}
	parent, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	ctx, stop := newShareTaskContext(parent, 20)
	defer stop()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		test.Fatal("earlier parent deadline was lost")
	}
}

func baselineTimeoutPointer(minutes int) *int { return &minutes }

type baselineDeadlineStore struct {
	ShareBaselineAdoptionStore
	observed chan context.Context
}

func (store baselineDeadlineStore) CheckSchema(ctx context.Context) error {
	store.observed <- ctx
	return store.ShareBaselineAdoptionStore.CheckSchema(ctx)
}

func TestShareBaselineAdoptionTaskTimeoutSnapshot(test *testing.T) {
	for _, minutes := range []int{0, 7, 20} {
		fixture, store, _, request := baselineFlowFixture(test)
		if minutes != 20 {
			request.TimeoutMinutes = baselineTimeoutPointer(minutes)
		}
		observed := make(chan context.Context, 1)
		id, err := fixture.service.startBaselineAdoptionContext(context.Background(), request, baselineDeadlineStore{store, observed})
		if err != nil {
			test.Fatal(err)
		}
		select {
		case ctx := <-observed:
			deadline, timed := ctx.Deadline()
			if timed != (minutes != 0) || (timed && time.Until(deadline) < time.Duration(minutes)*time.Minute-time.Second) {
				test.Fatal("async task did not receive the resolved timeout")
			}
		case <-time.After(5 * time.Second):
			test.Fatal("async task did not start")
		}
		limit := time.Now().Add(5 * time.Second)
		for {
			task, readErr := fixture.service.tasks.Get(id)
			if readErr != nil {
				test.Fatal(readErr)
			}
			if task["status"] == "completed" {
				break
			}
			if time.Now().After(limit) {
				test.Fatalf("task did not complete: %+v", task)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

type baselineContextSettings struct {
	shareSettingsMemory
	read func(context.Context, ...string) (map[string]string, error)
}

func (settings baselineContextSettings) GetHotConfigs(ctx context.Context, keys ...string) (map[string]string, error) {
	return settings.read(ctx, keys...)
}

func TestShareBaselineAdoptionTimeoutSettingsContext(test *testing.T) {
	failure := errors.New("isolated settings failure")
	service := &ShareStrmService{settings: &baselineContextSettings{read: func(ctx context.Context, keys ...string) (map[string]string, error) {
		if len(keys) != 1 || keys[0] != shareBaselineTimeoutKey {
			test.Fatal("timeout lookup escaped its existing settings key")
		}
		return nil, failure
	}}}
	if _, err := service.baselineAdoptionTimeout(context.Background(), domain.ShareBaselineAdoptionRequest{}); !errors.Is(err, failure) {
		test.Fatal("settings failure was swallowed")
	}
	if minutes, err := service.baselineAdoptionTimeout(context.Background(), domain.ShareBaselineAdoptionRequest{TimeoutMinutes: baselineTimeoutPointer(7)}); err != nil || minutes != 7 {
		test.Fatal("per-call override unexpectedly read system settings")
	}
	parent, cancel := context.WithCancel(context.Background())
	service.settings = &baselineContextSettings{read: func(ctx context.Context, keys ...string) (map[string]string, error) {
		cancel()
		return nil, ctx.Err()
	}}
	if _, err := service.baselineAdoptionTimeout(parent, domain.ShareBaselineAdoptionRequest{}); !errors.Is(err, context.Canceled) {
		test.Fatal("context-aware existing DAO configuration lookup lost cancellation")
	}
}
