package dao

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"easy-strm/internal/domain"
)

// newOfflineDownloadTaskMockDB 创建sqlmock内存库与DAO实例
func newOfflineDownloadTaskMockDB(t *testing.T) (*OfflineDownloadTaskDAO, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	return NewOfflineDownloadTaskDAO(db), mock, func() { _ = db.Close() }
}

func TestOfflineDownloadTaskDAOBatchInsert(t *testing.T) {
	dao, mock, cleanup := newOfflineDownloadTaskMockDB(t)
	defer cleanup()

	tasks := []domain.OfflineDownloadTask{
		{TaskId: "offline-1", Cloud115ID: 1, Url: "ed2k://|file|a.mkv|1|HASH|/", InfoHash: "hash1", Status: domain.OfflineStatusPending, SaveDirID: "dir-1"},
		{TaskId: "offline-1", Cloud115ID: 1, Url: "bad-url", Status: domain.OfflineStatusFailed, ErrorMessage: "链接格式无效"},
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO t_offline_download_task
		(task_id, cloud115_id, url, info_hash, name, size, status, percent, error_message, save_dir_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10),($11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`)).
		WithArgs(
			"offline-1", 1, "ed2k://|file|a.mkv|1|HASH|/", "hash1", "", int64(0), domain.OfflineStatusPending, 0.0, "", "dir-1",
			"offline-1", 1, "bad-url", "", "", int64(0), domain.OfflineStatusFailed, 0.0, "链接格式无效", "",
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := dao.BatchInsert(context.Background(), tasks); err != nil {
		t.Fatalf("expected batch insert to succeed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadTaskDAOBatchInsertEmpty(t *testing.T) {
	dao, _, cleanup := newOfflineDownloadTaskMockDB(t)
	defer cleanup()

	if err := dao.BatchInsert(context.Background(), nil); err != nil {
		t.Fatalf("empty batch should be no-op: %v", err)
	}
}

func TestOfflineDownloadTaskDAOList(t *testing.T) {
	dao, mock, cleanup := newOfflineDownloadTaskMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM t_offline_download_task WHERE 1=1 AND cloud115_id = $1`)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}).
		AddRow(10, "offline-1", 1, "ed2k://|file|a.mkv|1|HASH|/", "hash1", "a.mkv", int64(1024), domain.OfflineStatusDownloading, 55.5, "", "dir-1", "2026-08-05 10:00:00", "2026-08-05 10:05:00")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE 1=1 AND cloud115_id = $1 ORDER BY id DESC LIMIT $2 OFFSET $3`)).
		WithArgs(1, 20, 0).
		WillReturnRows(rows)

	tasks, total, err := dao.List(context.Background(), 1, "", 1, 20)
	if err != nil {
		t.Fatalf("expected list to succeed: %v", err)
	}
	if total != 1 || len(tasks) != 1 {
		t.Fatalf("unexpected result: total=%d len=%d", total, len(tasks))
	}
	if tasks[0].InfoHash != "hash1" || tasks[0].Status != domain.OfflineStatusDownloading || tasks[0].Percent != 55.5 {
		t.Fatalf("unexpected record: %+v", tasks[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadTaskDAOGetByIDNotFound(t *testing.T) {
	dao, mock, cleanup := newOfflineDownloadTaskMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE id = $1`)).
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)

	task, err := dao.GetByID(context.Background(), 99)
	if err != nil {
		t.Fatalf("expected no error for missing record: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil record, got %+v", task)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadTaskDAOGetActiveByAccount(t *testing.T) {
	dao, mock, cleanup := newOfflineDownloadTaskMockDB(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"id", "task_id", "cloud115_id", "url", "info_hash", "name", "size", "status", "percent", "error_message", "save_dir_id", "create_time", "update_time"}).
		AddRow(1, "offline-1", 1, "u1", "hash1", "", int64(0), domain.OfflineStatusPending, 0.0, "", "dir-1", "", "")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, task_id, cloud115_id, url, info_hash, name, size,
	status, percent, error_message, save_dir_id,
	COALESCE(create_time::text, ''), COALESCE(update_time::text, '') FROM t_offline_download_task WHERE cloud115_id = $1 AND status IN ($2, $3) ORDER BY id ASC`)).
		WithArgs(1, domain.OfflineStatusPending, domain.OfflineStatusDownloading).
		WillReturnRows(rows)

	tasks, err := dao.GetActiveByAccount(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected query to succeed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].InfoHash != "hash1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestOfflineDownloadTaskDAOUpdateAndDelete(t *testing.T) {
	dao, mock, cleanup := newOfflineDownloadTaskMockDB(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_offline_download_task
		SET name = $1, size = $2, status = $3, percent = $4, error_message = $5, update_time = CURRENT_TIMESTAMP
		WHERE cloud115_id = $6 AND info_hash = $7`)).
		WithArgs("a.mkv", int64(1024), domain.OfflineStatusCompleted, 100.0, "", 1, "hash1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE t_offline_download_task
		SET status = $1, error_message = $2, update_time = CURRENT_TIMESTAMP
		WHERE id = $3`)).
		WithArgs(domain.OfflineStatusRemoved, "任务已不在115离线列表中", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM t_offline_download_task WHERE id = $1`)).
		WithArgs(int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	if err := dao.UpdateByHash(ctx, 1, "hash1", "a.mkv", 1024, domain.OfflineStatusCompleted, 100.0, ""); err != nil {
		t.Fatalf("expected update by hash to succeed: %v", err)
	}
	if err := dao.UpdateStatus(ctx, 2, domain.OfflineStatusRemoved, "任务已不在115离线列表中"); err != nil {
		t.Fatalf("expected update status to succeed: %v", err)
	}
	if err := dao.Delete(ctx, 3); err != nil {
		t.Fatalf("expected delete to succeed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
