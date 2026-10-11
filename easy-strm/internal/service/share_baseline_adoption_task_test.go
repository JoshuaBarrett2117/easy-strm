package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShareBaselineAdoptionTaskFailureAndCancellation(test *testing.T) {
	for _, scenario := range []string{"failure", "cancel"} {
		test.Run(scenario, func(test *testing.T) {
			fixture, store, _, request := baselineFlowFixture(test)
			entered, release := make(chan struct{}), make(chan struct{})
			if scenario == "failure" {
				store.failure = errors.New("isolated transaction failure")
			} else {
				store.beforeAdopt = func() { close(entered); <-release }
			}
			id, err := fixture.service.startBaselineAdoptionContext(context.Background(), request, store)
			if err != nil {
				test.Fatal(err)
			}
			if scenario == "cancel" {
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					test.Fatal("task did not start")
				}
				if err := fixture.service.tasks.Cancel(id); err != nil {
					close(release)
					test.Fatal(err)
				}
				close(release)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				task, err := fixture.service.tasks.Get(id)
				if err != nil {
					test.Fatal(err)
				}
				wanted := "failed"
				if scenario == "cancel" {
					wanted = "cancelled"
				}
				_, running := fixture.service.tasks.cancelGuards.Load(id)
				if task["status"] == wanted && !running {
					break
				}
				if time.Now().After(deadline) {
					test.Fatalf("wrong terminal task: %+v", task)
				}
				time.Sleep(time.Millisecond)
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if store.adoptions != 0 || store.input.BaselineState != "building" || store.acked["1"] != 0 {
				test.Fatal("failed or cancelled task adopted baseline")
			}
		})
	}
}
