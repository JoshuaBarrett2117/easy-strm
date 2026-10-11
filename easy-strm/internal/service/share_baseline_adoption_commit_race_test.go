package service

import (
	"context"
	"testing"
	"time"
)

func TestShareBaselineAdoptionCancellationAfterCommitCannotHideAdoption(test *testing.T) {
	fixture, store, _, request := baselineFlowFixture(test)
	committed, release := make(chan struct{}), make(chan struct{})
	store.afterCommit = func() { close(committed); <-release }
	id, err := fixture.service.startBaselineAdoptionContext(context.Background(), request, store)
	if err != nil {
		test.Fatal(err)
	}
	select {
	case <-committed:
	case <-time.After(5 * time.Second):
		test.Fatal("task did not commit")
	}
	cancelErr := fixture.service.tasks.Cancel(id)
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, readErr := fixture.service.tasks.Get(id)
		if readErr != nil {
			test.Fatal(readErr)
		}
		if task["status"] == "completed" {
			break
		}
		if time.Now().After(deadline) {
			test.Fatal("task did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if cancelErr == nil {
		test.Fatal("late cancellation must not hide committed manual adoption")
	}
	task, err := fixture.service.tasks.Get(id)
	if err != nil || task["status"] != "completed" {
		test.Fatalf("wrong committed status: %+v %v", task, err)
	}
	if err := fixture.service.tasks.Cancel(id); err == nil {
		test.Fatal("completed adoption must not become cancelled")
	}
}
