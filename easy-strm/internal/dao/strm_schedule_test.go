package dao

import (
	"easy-strm/internal/domain"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
)

func TestStrmConfigRollsBackWhenScheduleFails(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO t_strm_config`).WillReturnRows(sqlmock.NewRows([]string{"id", "tree", "created", "updated"}).AddRow(9, "", time.Now(), time.Now()))
	mock.ExpectExec(`INSERT INTO t_cron_task`).WillReturnError(errors.New("调度写入失败"))
	mock.ExpectRollback()
	_, err := NewStrmConfigDAO().SaveWithSchedule(&domain.StrmConfig{Cloud115Id: 1, Cron: "0 0 3 * * *"})
	if err == nil {
		t.Fatal("不能把配置与任务分开提交")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStrmDeleteReturnsAllSchedules(t *testing.T) {
	mock, cleanup := setupMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM t_strm_config WHERE id=\$1 FOR UPDATE`).WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectQuery(`SELECT id FROM t_cron_task WHERE strm_config_id=\$1`).WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
	mock.ExpectExec(`DELETE FROM t_strm_config WHERE id=\$1`).WithArgs(9).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	ids, err := NewStrmConfigDAO().DeleteWithSchedules(9)
	if err != nil || len(ids) != 2 {
		t.Fatalf("应卸载全量和增量调度: %v %v", ids, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
