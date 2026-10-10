package service

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestPreparationStartHeartbeatRealBatchEnd(t *testing.T) {
	events := []string{}
	heartbeatSeen := make(chan struct{})
	err := runSharePreparation(context.Background(), "selection-sync", nil, time.Millisecond, time.Millisecond, func(event string, fields logger.Fields, err error) {
		events = append(events, event)
		if fields["phase"] != "selection-sync" || fields["eta"] != "unknown" || err != nil {
			t.Errorf("fields=%v err=%v", fields, err)
		}
		if event == "heartbeat" && fields["processed"] == 0 {
			if fields["rate"] != float64(0) || fields["total"] != -1 {
				t.Error("invented SQL progress")
			}
			select {
			case <-heartbeatSeen:
			default:
				close(heartbeatSeen)
			}
		}
	}, func(ctx context.Context, report sharePreparationReport) error {
		select {
		case <-heartbeatSeen:
		case <-time.After(time.Second):
			return errors.New("no heartbeat while SQL blocked")
		}
		report(100, -1)
		return nil
	})
	if err != nil || events[0] != "start" || events[len(events)-1] != "end" {
		t.Fatalf("events=%v err=%v", events, err)
	}
	length := len(events)
	time.Sleep(5 * time.Millisecond)
	if len(events) != length {
		t.Fatal("heartbeat goroutine not joined")
	}
}

func TestPreparationCancellationWhileBlockedAndNoSuccess(t *testing.T) {
	for _, taskCancel := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var cancelled atomic.Bool
		events := []string{}
		err := runSharePreparation(ctx, "selection-resolve", func() bool { return cancelled.Load() }, time.Millisecond, time.Millisecond, func(event string, _ logger.Fields, _ error) {
			events = append(events, event)
			if event == "heartbeat" {
				if taskCancel {
					cancelled.Store(true)
				} else {
					cancel()
				}
			}
		}, func(ctx context.Context, report sharePreparationReport) error {
			select {
			case <-ctx.Done():
				report(1, 1)
				return ctx.Err()
			case <-time.After(time.Second):
				return errors.New("cancel did not reach query")
			}
		})
		cancel()
		if !errors.Is(err, context.Canceled) || events[0] != "start" || events[len(events)-1] != "cancel" {
			t.Fatalf("events=%v err=%v", events, err)
		}
		for _, event := range events {
			if event == "end" || event == "batch" {
				t.Fatal("logged canceled success")
			}
		}
	}
}

func TestPreparationErrorAndAlreadyCancelledJoin(t *testing.T) {
	for _, preCancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if preCancelled {
			cancel()
		}
		events := []string{}
		err := runSharePreparation(ctx, "conflicts", nil, time.Hour, time.Hour, func(event string, _ logger.Fields, _ error) { events = append(events, event) }, func(context.Context, sharePreparationReport) error {
			if preCancelled {
				t.Fatal("called work after cancel")
			}
			return errors.New("query failed")
		})
		cancel()
		last := "error"
		if preCancelled {
			last = "cancel"
		}
		if err == nil || !reflect.DeepEqual(events, []string{"start", last}) {
			t.Fatalf("events=%v err=%v", events, err)
		}
	}
}

func TestPreparationPanicStillJoinsHeartbeat(t *testing.T) {
	var events atomic.Int64
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing panic")
			}
		}()
		runSharePreparation(context.Background(), "sync", nil, time.Millisecond, time.Millisecond, func(string, logger.Fields, error) { events.Add(1) }, func(context.Context, sharePreparationReport) error { panic("stage panic") })
	}()
	count := events.Load()
	time.Sleep(5 * time.Millisecond)
	if events.Load() != count {
		t.Fatal("panic leaked heartbeat")
	}
}

func TestPreparationFullInvalidReportPersistsToTask(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	ctx := logger.WithTaskID(context.Background(), "invalid-preparation")
	if err := tasks.CreateContext(ctx, "invalid-preparation", "strm_generate", "来源准备"); err != nil {
		t.Fatal(err)
	}
	invalid := &dao.ShareSelectionPreparationError{ManualKeys: []string{"a", "b", "c", "d", "e", "f", "g"}, AutoKeys: []string{"z"}}
	store := &selectedFilesystemStore{resolveErr: invalid}
	exporter := &ShareStrmService{tasks: tasks}
	if err := exporter.prepareSelectionWork(ctx, store, ""); !errors.Is(err, dao.ErrShareSelectionUnavailable) {
		t.Fatal(err)
	}
	task, err := tasks.GetContext(ctx, "invalid-preparation")
	if err != nil {
		t.Fatal(err)
	}
	metadata := task["metadata"].(map[string]interface{})
	report := metadata["selection_preparation_report"].(map[string]interface{})
	if len(report["manual_keys"].([]interface{})) != 7 || metadata["manual_invalid_count"] != float64(7) || len(report["auto_keys"].([]interface{})) != 1 {
		t.Fatalf("report lost beyond examples: %v", metadata)
	}
}

func TestPreparationCategoriesReceivesCancelableContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exporter := NewShareStrmService(&strmMemory{}, nil, nil, nil, nil, nil, func() ([]*domain.MediaCategory, error) {
		t.Fatal("production category reader must not use non-context fallback")
		return nil, nil
	}, nil, nil)
	exporter.SetCategoryContextReader(func(ctx context.Context) ([]*domain.MediaCategory, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err := exporter.export(ctx, domain.ShareStrmSettings{}, domain.ShareLibraryQuery{}, "cancel-categories"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
