package service

import (
	"context"
	"easy-strm/internal/domain"
	"testing"
	"time"
)

type shareSettingsMemory struct{ values map[string]*domain.SystemConfig }

func (s *shareSettingsMemory) GetByKey(key string) (*domain.SystemConfig, error) {
	return s.values[key], nil
}
func (s *shareSettingsMemory) Upsert(key, value string) error {
	if s.values == nil { s.values = map[string]*domain.SystemConfig{} }
	s.values[key] = &domain.SystemConfig{ConfigVal: value}
	return nil
}

func TestShareTaskUnlimitedAndConfiguredDeadline(t *testing.T) {
	ctx, cancel := newShareTaskContext(context.Background(), 0)
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("无限制不能有总截止时间")
	}
	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatal("无限制仍应支持取消")
	}
	before := time.Now()
	ctx, cancel = newShareTaskContext(context.Background(), 2)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || deadline.Before(before.Add(119*time.Second)) || deadline.After(before.Add(121*time.Second)) {
		t.Fatal(deadline)
	}
	parent, parentCancel := context.WithCancel(context.Background())
	child, childCancel := newShareTaskContext(parent, 0)
	defer childCancel()
	parentCancel()
	if child.Err() != context.Canceled {
		t.Fatal("必须继承父上下文取消")
	}
	expired, expiredCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expiredCancel()
	child, childCancel = newShareTaskContext(expired, 0)
	defer childCancel()
	if child.Err() != context.DeadlineExceeded {
		t.Fatal("不能移除调用方已有期限")
	}
}
func TestShareTaskSettingsRoundTrip(t *testing.T) {
	store := &shareSettingsMemory{}
	s := &ShareRecordService{taskSettingsStore: store}
	value, err := s.GetTaskSettings()
	if err != nil || value.TimeoutMinutes != 0 || value.WorkerCount != defaultShareWorkerCount {
		t.Fatal(value, err)
	}
	for _, minutes := range []int{60, 0, 43200} {
		if err = s.SaveTaskSettings(ShareTaskSettings{TimeoutMinutes: minutes}); err != nil {
			t.Fatal(err)
		}
		value, err = s.GetTaskSettings()
		if err != nil || value.TimeoutMinutes != minutes {
			t.Fatal(value, err)
		}
	}
	for _, minutes := range []int{-1, 43201} {
		if s.SaveTaskSettings(ShareTaskSettings{TimeoutMinutes: minutes}) == nil {
			t.Fatal(minutes)
		}
	}
	store.values[shareTaskTimeoutKey].ConfigVal = "invalid"
	if _, err = s.GetTaskSettings(); err == nil {
		t.Fatal("损坏配置不得静默忽略")
	}
}

func TestShareTaskSettingsWorkerCountValidation(t *testing.T) {
	store := &shareSettingsMemory{}
	s := &ShareRecordService{taskSettingsStore: store}
	if err := s.SaveTaskSettings(ShareTaskSettings{TimeoutMinutes: 1, WorkerCount: 4}); err != nil {
		t.Fatalf("default worker count should be accepted: %v", err)
	}
	if got, err := s.GetTaskSettings(); err != nil || got.WorkerCount != 4 {
		t.Fatalf("worker count round trip: got=%+v err=%v", got, err)
	}
	for _, n := range []int{0, 33} {
		if err := s.SaveTaskSettings(ShareTaskSettings{WorkerCount: n}); err == nil {
			t.Fatalf("worker count %d should be rejected", n)
		}
	}
}
