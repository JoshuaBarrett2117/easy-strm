package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareStrmCronHandlersRegisteredAndMapped(t *testing.T) {
	scheduler := NewCronService(nil)
	RegisterShareStrmCronHandlers(scheduler, &ShareStrmService{})
	if len(scheduler.Handlers()) != 2 {
		t.Fatal("both handlers required")
	}
	for _, handler := range scheduler.Handlers() {
		if handler.Execute == nil || cronTaskKind(handler.Key) != "strm_generate" || cronShareMode(handler.Key) == "" {
			t.Fatal(handler)
		}
		if err := scheduler.ValidateDefinition(&domain.CronTask{TaskName: handler.Name, Handler: handler.Key, Params: map[string]interface{}{}, CronExpr: "0 */6 * * *", Status: "enabled"}); err != nil {
			t.Fatal(err)
		}
		if _, err := handler.Execute(context.Background(), nil, "cron_uninitialized"); err == nil {
			t.Fatal("not wired to exporter")
		}
	}
}

func TestShareStrmCronMutualExclusionAcrossIDs(t *testing.T) {
	for _, firstHandler := range []string{"share_strm_incremental_export", "share_strm_full_reconciliation"} {
		t.Run(firstHandler, func(t *testing.T) {
			scheduler, mock := cronTestService(t)
			entered, finish := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(finish) })
			secondHandler := "share_strm_full_reconciliation"
			if firstHandler == secondHandler {
				secondHandler = "share_strm_incremental_export"
			}
			scheduler.Register(CronHandler{Key: firstHandler, Execute: func(context.Context, *domain.CronTask, string) (string, error) {
				close(entered)
				<-finish
				return "完成", nil
			}})
			scheduler.Register(CronHandler{Key: secondHandler, Execute: func(context.Context, *domain.CronTask, string) (string, error) {
				t.Error("mutex allowed other handler")
				return "", errors.New("must skip")
			}})
			expectCronRead(mock, 11, 0, firstHandler, "enabled", false)
			trigger := "scheduled"
			if firstHandler == "share_strm_full_reconciliation" {
				trigger = "manual"
			}
			mock.ExpectExec(`INSERT INTO t_cron_task_run`).WithArgs(11, sqlmock.AnyArg(), trigger, "pending").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(`UPDATE t_cron_task_run SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "running", "", 11).WillReturnResult(sqlmock.NewResult(0, 1))
			active, err := scheduler.Run(11, trigger)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("handler not entered")
			}
			expectCronRead(mock, 22, 0, secondHandler, "enabled", false)
			mock.ExpectExec(`INSERT INTO t_cron_task_run`).WithArgs(22, sqlmock.AnyArg(), "manual", "skipped").WillReturnResult(sqlmock.NewResult(2, 1))
			mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs("skipped", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			skipped, err := scheduler.Run(22, "manual")
			if err != nil {
				t.Fatal(err)
			}
			task, err := scheduler.tasks.Get(skipped)
			if err != nil {
				t.Fatal(err)
			}
			metadata := task["metadata"].(map[string]interface{})
			if task["task_type"] != "strm_generate" || task["status"] != "completed" || metadata["outcome"] != "skipped" || metadata["effective_mode"] != "skipped" || metadata["blocked_by_task_id"] != active || metadata["skip_reason"] == "" {
				t.Fatal(task)
			}
			mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs("success", "完成", active).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "success", "完成", 11).WillReturnResult(sqlmock.NewResult(0, 1))
			once.Do(func() { close(finish) })
			waitCronIdle(t, scheduler)
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareStrmCronRestartWaitsNextOccurrence(t *testing.T) {
	scheduler, mock := cronTestService(t)
	RegisterShareStrmCronHandlers(scheduler, &ShareStrmService{})
	entered := make(chan struct{}, 2)
	for _, handler := range scheduler.Handlers() {
		handler.Execute = func(context.Context, *domain.CronTask, string) (string, error) {
			entered <- struct{}{}
			return "must not replay", nil
		}
		scheduler.Register(handler)
	}
	mock.ExpectExec(`WITH interrupted AS`).WillReturnResult(sqlmock.NewResult(0, 2))
	now := time.Now()
	last := now.Add(-8 * 24 * time.Hour)
	rows := sqlmock.NewRows([]string{"id", "name", "type", "cloud", "config", "cron", "status", "last", "next", "result", "message", "created", "updated"})
	for index, key := range []string{"share_strm_incremental_export", "share_strm_full_reconciliation"} {
		identifier := index + 1
		rows.AddRow(identifier, key, key, 0, 0, "0 0 0 1 1 *", "enabled", last, last, "running", "interrupted", now, now)
		for hydration := 0; hydration < 2; hydration++ {
			mock.ExpectQuery(`SELECT COALESCE\(task_key`).WithArgs(identifier).WillReturnRows(sqlmock.NewRows([]string{"key", "handler", "params", "timezone", "builtin"}).AddRow(key, key, `{}`, "UTC", false))
		}
		mock.ExpectExec(`UPDATE t_cron_task SET next_run_time`).WithArgs(sqlmock.AnyArg(), identifier).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery(`SELECT id, task_name, task_type, COALESCE`).WillReturnRows(rows)
	if err := scheduler.LoadTasksFromDB(); err != nil {
		t.Fatal(err)
	}
	for _, entry := range scheduler.engine.Entries() {
		if !entry.Next.After(now) {
			t.Fatalf("replayed interrupted schedule: %+v", entry)
		}
	}
	scheduler.Stop()
	select {
	case <-entered:
		t.Fatal("startup invoked exporter")
	default:
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareStrmCronCancellationTerminalState(t *testing.T) {
	scheduler, mock := cronTestService(t)
	scheduler.Register(CronHandler{Key: "share_strm_incremental_export", Execute: func(context.Context, *domain.CronTask, string) (string, error) { return "", context.Canceled }})
	expectCronRead(mock, 1, 0, "share_strm_incremental_export", "enabled", false)
	mock.ExpectExec(`INSERT INTO t_cron_task_run`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE t_cron_task_run SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "running", "", 1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs("cancelled", context.Canceled.Error(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "cancelled", context.Canceled.Error(), 1).WillReturnResult(sqlmock.NewResult(0, 1))
	id, err := scheduler.Run(1, "scheduled")
	if err != nil {
		t.Fatal(err)
	}
	waitCronIdle(t, scheduler)
	task, err := scheduler.tasks.Get(id)
	if err != nil || task["status"] != "cancelled" {
		t.Fatalf("%v %v", task, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
