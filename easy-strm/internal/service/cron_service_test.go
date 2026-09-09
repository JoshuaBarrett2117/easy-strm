package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"sync"
	"testing"
	"time"
)

func TestParseCronSupportsFiveAndSixFields(t *testing.T) {
	for _, expr := range []string{"0 3 * * *", "0 0 3 * * *"} {
		if _, err := ParseCron(expr, "Asia/Shanghai"); err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
	}
	if _, err := ParseCron("0 0 * *", "Asia/Shanghai"); err == nil {
		t.Fatal("expected invalid cron")
	}
}

func cronTestService(t *testing.T) (*CronService, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitDAO(database)
	dao.InitTaskRedisDAO(client)
	s := NewCronService(dao.NewCronTaskDAO())
	s.SetTaskService(NewTaskService(dao.NewTaskRedisDAO(client)))
	t.Cleanup(func() { s.Stop(); client.Close(); database.Close() })
	mock.MatchExpectationsInOrder(false)
	return s, mock
}

func expectCronRead(mock sqlmock.Sqlmock, id, config int, handler, status string, builtin bool) {
	now := time.Now()
	mock.ExpectQuery(`SELECT id, task_name, task_type, COALESCE`).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "cloud", "config", "cron", "status", "last", "next", "result", "message", "created", "updated"}).AddRow(id, "测试任务", handler, 1, config, "0 0 3 * * *", status, nil, nil, "", "", now, now))
	raw, _ := json.Marshal(map[string]interface{}{"cloud115_id": 1, "strm_config_id": config})
	mock.ExpectQuery(`SELECT COALESCE\(task_key`).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"key", "handler", "params", "timezone", "builtin"}).AddRow("test", handler, string(raw), "Local", builtin))
}

func waitCronIdle(t *testing.T, s *CronService) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		idle := len(s.running) == 0
		s.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("处理器未在时限内结束")
}

// TestCronRunSerializesConfig 验证手动与定时共享同一配置互斥，跳过也可追踪。
func TestCronRunSerializesConfig(t *testing.T) {
	s, mock := cronTestService(t)
	entered, finish := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	s.Register(CronHandler{Key: "full_generate", Execute: func(context.Context, *domain.CronTask, string) (string, error) {
		close(entered)
		<-finish
		return "成功", nil
	}})
	s.Register(CronHandler{Key: "incremental_sync", Execute: func(context.Context, *domain.CronTask, string) (string, error) {
		return "不应调用", errors.New("不应调用")
	}})
	expectCronRead(mock, 1, 8, "full_generate", "enabled", false)
	mock.ExpectExec(`INSERT INTO t_cron_task_run`).WithArgs(1, sqlmock.AnyArg(), "scheduled", "pending").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE t_cron_task_run SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "running", "", 1).WillReturnResult(sqlmock.NewResult(0, 1))
	id, err := s.Run(1, "scheduled")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("处理器没有启动")
	}
	expectCronRead(mock, 2, 8, "incremental_sync", "enabled", false)
	mock.ExpectExec(`INSERT INTO t_cron_task_run`).WithArgs(2, sqlmock.AnyArg(), "manual", "skipped").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs("skipped", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	skipped, err := s.Run(2, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if skipped == id {
		t.Fatal("执行ID应独立")
	}
	task, err := s.tasks.Get(skipped)
	if err != nil || task == nil {
		t.Fatalf("跳过记录不可追踪: %v", err)
	}
	mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs("success", "成功", id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "success", "成功", 1).WillReturnResult(sqlmock.NewResult(0, 1))
	once.Do(func() { close(finish) })
	waitCronIdle(t, s)
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCronHandlerFailureAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancel"}[cancelled], func(t *testing.T) {
			s, mock := cronTestService(t)
			entered := make(chan struct{})
			s.Register(CronHandler{Key: "log_cleanup", Execute: func(ctx context.Context, _ *domain.CronTask, _ string) (string, error) {
				close(entered)
				if cancelled {
					<-ctx.Done()
					return "", ctx.Err()
				}
				return "", errors.New("清理失败")
			}})
			expectCronRead(mock, 3, 0, "log_cleanup", "enabled", true)
			mock.ExpectExec(`INSERT INTO t_cron_task_run`).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectExec(`UPDATE t_cron_task_run SET status='running'`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, "running", "", 3).WillReturnResult(sqlmock.NewResult(0, 1))
			status, message := "failed", "清理失败"
			if cancelled {
				status = "cancelled"
				message = "任务已取消"
			}
			mock.ExpectExec(`UPDATE t_cron_task_run SET status=\$1`).WithArgs(status, message, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE t_cron_task SET last_run_time`).WithArgs(sqlmock.AnyArg(), nil, status, message, 3).WillReturnResult(sqlmock.NewResult(0, 1))
			id, err := s.Run(3, "manual")
			if err != nil {
				t.Fatal(err)
			}
			<-entered
			if cancelled {
				if err = s.tasks.Cancel(id); err != nil {
					t.Fatal(err)
				}
			}
			waitCronIdle(t, s)
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCronInvalidEditLeavesSchedule(t *testing.T) {
	s, mock := cronTestService(t)
	s.Register(CronHandler{Key: "full_generate"})
	expectCronRead(mock, 1, 8, "full_generate", "enabled", false)
	_, err := s.Save(&domain.CronTask{ID: 1, Handler: "full_generate", TaskName: "测试", CronExpr: "invalid"})
	if err == nil {
		t.Fatal("错误表达式必须在写库前失败")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	expectCronRead(mock, 3, 0, "log_cleanup", "enabled", true)
	if err = s.Delete(3); err == nil {
		t.Fatal("内置任务不允许删除")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCronScheduleTimezone(t *testing.T) {
	schedule, err := ParseCron("0 0 3 * * *", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 9, 8, 18, 0, 0, 0, time.UTC)
	if got := schedule.Next(from); !got.Equal(time.Date(2026, 9, 8, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("错误的下次时间: %v", got)
	}
	if _, err = ParseCron("0 0 3 * * *", "invalid/timezone"); err == nil {
		t.Fatal("时区应验证")
	}
}

func TestCronHotReloadKeepsSingleRegistration(t *testing.T) {
	s, mock := cronTestService(t)
	s.Register(CronHandler{Key: "log_cleanup"})
	task := &domain.CronTask{ID: 9, Handler: "log_cleanup", CronExpr: "0 0 3 * * *", Timezone: "Asia/Shanghai", Status: "enabled"}
	mock.ExpectExec(`UPDATE t_cron_task SET next_run_time`).WithArgs(sqlmock.AnyArg(), 9).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.replaceLocked(task); err != nil {
		t.Fatal(err)
	}
	first := s.entries[9]
	task.CronExpr = "0 0 4 * * *"
	mock.ExpectExec(`UPDATE t_cron_task SET next_run_time`).WithArgs(sqlmock.AnyArg(), 9).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.replaceLocked(task); err != nil {
		t.Fatal(err)
	}
	if len(s.engine.Entries()) != 1 || s.entries[9] == first {
		t.Fatal("更新必须替换原调度")
	}
	current := s.entries[9]
	task.CronExpr = "invalid"
	if err := s.replaceLocked(task); err == nil || s.entries[9] != current {
		t.Fatal("解析失败不应移除原调度")
	}
	task.CronExpr = "0 0 4 * * *"
	task.Status = "disabled"
	mock.ExpectExec(`UPDATE t_cron_task SET next_run_time`).WithArgs(nil, 9).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.replaceLocked(task); err != nil {
		t.Fatal(err)
	}
	if len(s.engine.Entries()) != 0 || s.GetNextRunTime(9) != nil {
		t.Fatal("停用后不应有后续调度")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectStrmGenerationUsesSameMutex(t *testing.T) {
	s, _ := cronTestService(t)
	release, err := s.AcquireStrmExecution(9, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AcquireStrmExecution(9, "scheduled"); err == nil {
		t.Fatal("定时与手动不能同时写同一配置")
	}
	inner, err := s.AcquireStrmExecution(9, "manual")
	if err != nil {
		t.Fatal(err)
	}
	inner()
	if _, err = s.AcquireStrmExecution(9, "other"); err == nil {
		t.Fatal("内层不应提前释放外层的互斥")
	}
	release()
	next, err := s.AcquireStrmExecution(9, "scheduled")
	if err != nil {
		t.Fatal(err)
	}
	next()
}
