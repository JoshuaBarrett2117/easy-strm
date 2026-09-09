package dao

import (
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestCronTaskDAOCreateHandlesNullableRunFields(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewCronTaskDAO()
	now := time.Now()
	createRows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	getRows := sqlmock.NewRows([]string{
		"id", "task_name", "task_type", "cloud115_id", "strm_config_id", "cron_expr", "status",
		"last_run_time", "next_run_time", "last_run_status", "last_run_message", "create_time", "update_time",
	}).AddRow(
		1, "STRM全量生成-test", "full_generate", 2, 3, "0 2 * * *", "enabled",
		nil, nil, "", "", now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_cron_task (task_name, task_type, cloud115_id, strm_config_id, cron_expr, status)
		VALUES ($1, $2, $3, $4, $5, 'enabled')
		RETURNING id`)).
		WithArgs("STRM全量生成-test", "full_generate", 2, 3, "0 2 * * *").
		WillReturnRows(createRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_name, task_type, COALESCE(cloud115_id,0), COALESCE(strm_config_id,0), cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task WHERE id = $1`)).
		WithArgs(1).
		WillReturnRows(getRows)

	task, err := dao.Create("STRM全量生成-test", "full_generate", 2, 3, "0 2 * * *")
	if err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}
	if task.LastRunStatus != "" || task.LastRunMessage != "" {
		t.Fatalf("expected nullable run fields to normalize to empty string, got status=%q message=%q", task.LastRunStatus, task.LastRunMessage)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCronTaskDAOUpdateHandlesNullableRunFields(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()

	dao := NewCronTaskDAO()
	now := time.Now()
	updateRows := sqlmock.NewRows([]string{"id"}).AddRow(9)
	getRows := sqlmock.NewRows([]string{
		"id", "task_name", "task_type", "cloud115_id", "strm_config_id", "cron_expr", "status",
		"last_run_time", "next_run_time", "last_run_status", "last_run_message", "create_time", "update_time",
	}).AddRow(
		9, "STRM全量生成-test", "full_generate", 2, 3, "15 3 * * *", "disabled",
		nil, nil, "", "", now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE t_cron_task SET task_name=$1, task_type=$2, cron_expr=$3, status=$4 WHERE id=$5
		RETURNING id`)).
		WithArgs("STRM全量生成-test", "full_generate", "15 3 * * *", "disabled", 9).
		WillReturnRows(updateRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_name, task_type, COALESCE(cloud115_id,0), COALESCE(strm_config_id,0), cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task WHERE id = $1`)).
		WithArgs(9).
		WillReturnRows(getRows)

	task, err := dao.Update(9, "STRM全量生成-test", "full_generate", "15 3 * * *", "disabled")
	if err != nil {
		t.Fatalf("expected update to succeed: %v", err)
	}
	if task.Status != "disabled" {
		t.Fatalf("expected updated status to be disabled, got %q", task.Status)
	}
	if task.LastRunStatus != "" || task.LastRunMessage != "" {
		t.Fatalf("expected nullable run fields to normalize to empty string, got status=%q message=%q", task.LastRunStatus, task.LastRunMessage)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
