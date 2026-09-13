package dao

import (
	"regexp"
	"testing"

	"easy-strm/internal/domain"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestCronTaskDAOSaveDefinitionUsesDistinctParametersForDifferentColumnTypes(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		mock, cleanup := setupMockDB(t)
		defer cleanup()

		task := &domain.CronTask{
			TaskKey:  "task:test",
			TaskName: "分享库strm增量导出",
			Handler:  "share_strm_incremental_export",
			Params:   map[string]interface{}{},
			Timezone: "Local",
			CronExpr: "0 0 3 * * *",
			Status:   "enabled",
		}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO t_cron_task(task_key,task_name,task_type,handler,params,timezone,builtin,cloud115_id,strm_config_id,cron_expr,status) VALUES($1,$2,$3,$4,$5,$6,false,$7,$8,$9,$10) RETURNING id`)).
			WithArgs(task.TaskKey, task.TaskName, task.Handler, task.Handler, []byte(`{}`), task.Timezone, nil, nil, task.CronExpr, task.Status).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
		mock.ExpectCommit()

		if err := NewCronTaskDAO().SaveDefinition(task); err != nil {
			t.Fatalf("保存分享库增量导出任务失败: %v", err)
		}
		if task.ID != 11 {
			t.Fatalf("任务ID = %d，期望 11", task.ID)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("SQL 预期未满足: %v", err)
		}
	})

	t.Run("update", func(t *testing.T) {
		mock, cleanup := setupMockDB(t)
		defer cleanup()

		task := &domain.CronTask{
			ID:       11,
			TaskName: "分享库strm增量导出",
			Handler:  "share_strm_incremental_export",
			Params:   map[string]interface{}{},
			Timezone: "Local",
			CronExpr: "0 0 3 * * *",
			Status:   "enabled",
		}
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_strm_config SET cron='' WHERE id IN (SELECT strm_config_id FROM t_cron_task WHERE id=$1 AND handler='full_generate' AND (handler<>$2 OR strm_config_id IS DISTINCT FROM $3::integer))`)).
			WithArgs(task.ID, task.Handler, nil).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_cron_task SET task_name=$1,task_type=$2,handler=$3,params=$4,timezone=$5,cloud115_id=$6,strm_config_id=$7,cron_expr=$8,status=$9 WHERE id=$10`)).
			WithArgs(task.TaskName, task.Handler, task.Handler, []byte(`{}`), task.Timezone, nil, nil, task.CronExpr, task.Status, task.ID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		if err := NewCronTaskDAO().SaveDefinition(task); err != nil {
			t.Fatalf("更新分享库增量导出任务失败: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("SQL 预期未满足: %v", err)
		}
	})
}
