package dao

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCronTaskDAOGetNotificationTasks(t *testing.T) {
	for _, branch := range []string{"all", "query error", "scan error", "rows error", "canceled"} {
		t.Run(branch, func(t *testing.T) {
			mock, cleanup := setupMockDB(t)
			defer cleanup()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if branch == "canceled" {
				cancel()
			} else {
				query := mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, COALESCE(task_key,''), task_name, COALESCE(handler,''), COALESCE(status,'') FROM t_cron_task ORDER BY id`))
				rows := sqlmock.NewRows([]string{"id", "key", "name", "handler", "status"}).AddRow(1, "key:a", "甲", "shared", "enabled").AddRow(2, "key:b", "乙", "shared", "disabled").AddRow(3, "", "空状态", "legacy", "")
				switch branch {
				case "query error":
					query.WillReturnError(errors.New("unavailable"))
				case "scan error":
					query.WillReturnRows(sqlmock.NewRows([]string{"id", "key", "name", "handler", "status"}).AddRow("invalid", "", "", "", ""))
				case "rows error":
					query.WillReturnRows(rows.RowError(1, errors.New("stream failed")))
				default:
					query.WillReturnRows(rows).RowsWillBeClosed()
				}
			}
			tasks, err := NewCronTaskDAO().GetNotificationTasks(ctx)
			if branch == "all" {
				if err != nil || len(tasks) != 3 || tasks[1].TaskKey != "key:b" || tasks[1].Handler != "shared" || tasks[1].Status != "disabled" || tasks[2].Status != "" {
					t.Fatalf("tasks=%+v err=%v", tasks, err)
				}
			} else if err == nil {
				t.Fatal("不能忽略查询、扫描或迭代错误")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNotificationTaskScopeBatchUpsertContextCancellation(t *testing.T) {
	for _, branch := range []string{"already canceled", "cancel write", "write timeout"} {
		t.Run(branch, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			previous := DB
			DB = database
			defer func() { DB = previous; InvalidateSystemConfigCache(database) }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if branch == "already canceled" {
				cancel()
			} else {
				if branch == "cancel write" {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
					defer deadlineCancel()
				}
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO t_system_config").WillDelayFor(3 * time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectRollback()
			}
			started := time.Now()
			err = NewSystemConfigDAO().BatchUpsertContext(ctx, map[string]string{"notify_task_keys": "[]", "notify_task_scope_configured": "true"})
			if err == nil || time.Since(started) > 2800*time.Millisecond {
				t.Fatalf("取消或两秒预算必须中断写入: %v", err)
			}
			deadline := time.Now().Add(time.Second)
			for {
				if err = mock.ExpectationsWereMet(); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func TestNotificationTaskScopeBatchUpsertAtomic(t *testing.T) {
	for _, branch := range []string{"commit", "rollback"} {
		t.Run(branch, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			previous := DB
			DB = database
			defer func() { DB = previous; InvalidateSystemConfigCache(database) }()
			mock.ExpectBegin()
			if branch == "rollback" {
				mock.ExpectExec("INSERT INTO t_system_config").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO t_system_config").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(errors.New("write failed"))
				mock.ExpectRollback()
			} else {
				mock.MatchExpectationsInOrder(false)
				mock.ExpectExec("INSERT INTO t_system_config").WithArgs("notify_task_keys", "[]").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO t_system_config").WithArgs("notify_task_scope_configured", "true").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = NewSystemConfigDAO().BatchUpsert(map[string]string{"notify_task_keys": "[]", "notify_task_scope_configured": "true"})
			if (err == nil) != (branch == "commit") {
				t.Fatalf("err=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
