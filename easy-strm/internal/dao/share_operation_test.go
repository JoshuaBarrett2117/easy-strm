package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func operationRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"sequence", "task_id", "operation", "request_key", "share_ids", "file_id", "status", "phase", "result", "error", "created_at"})
}

func TestShareOperationEnqueueAndRecoveryPreserveIdentity(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	d := NewShareOperationDAO(db)
	now := time.Now()
	m.ExpectQuery("INSERT INTO t_share_operation_queue").WithArgs("new", "share_clear", "same", pq.Array([]int{1, 2}), 0).WillReturnRows(operationRows().AddRow(4, "existing", "share_clear", "same", "{1,2}", 0, "pending", "等待目标资源", `{}`, "", now))
	v, err := d.Enqueue(context.Background(), domain.ShareOperation{TaskID: "new", Kind: "share_clear", Key: "same", ShareIDs: []int{1, 2}})
	if err != nil || v.TaskID != "existing" || len(v.ShareIDs) != 2 {
		t.Fatal(v, err)
	}
	m.ExpectQuery("WHERE status IN .* OR NOT published ORDER BY sequence").WillReturnRows(operationRows().AddRow(5, "committed", "share_clear", "another", "{}", 0, "completed", "清理完成", `{"deleted":8}`, "", now))
	rows, err := d.Recoverable(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Result["deleted"] != float64(8) {
		t.Fatal(rows, err)
	}
	m.ExpectQuery("WHERE task_id=\\$1").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	if _, err = d.Get(context.Background(), "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareOperationEmptyAllClearUsesNonNullTargets(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("INSERT INTO t_share_operation_queue").WithArgs("empty", "share_clear", "empty", "{}", 0).WillReturnRows(operationRows().AddRow(1, "empty", "share_clear", "empty", "{}", 0, "pending", "等待目标资源", `{}`, "", time.Now()))
	if _, err := NewShareOperationDAO(db).Enqueue(context.Background(), domain.ShareOperation{TaskID: "empty", Kind: "share_clear", Key: "empty"}); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareOperationConditionalStartCancelAndFinish(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	d := NewShareOperationDAO(db)
	m.ExpectExec("status IN \\('pending','running'\\)").WithArgs("cancelled").WillReturnResult(sqlmock.NewResult(0, 0))
	if started, err := d.Start(context.Background(), "cancelled"); started || err != nil {
		t.Fatal(started, err)
	}
	m.ExpectExec("AND status='pending'").WithArgs("running").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := d.Cancel(context.Background(), "running"); err == nil {
		t.Fatal("执行阶段不能取消")
	}
	m.ExpectExec("AND status='pending'").WithArgs("waiting").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := d.Cancel(context.Background(), "waiting"); err != nil {
		t.Fatal(err)
	}
	m.ExpectExec("AND status IN \\('pending','running'\\)").WithArgs("committed", "failed", "late error", "null").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := d.Finish(context.Background(), "committed", "failed", "late error", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareClearJournalCommitsWithDeletionOrRollsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			db, m, _ := sqlmock.New()
			defer db.Close()
			m.ExpectBegin()
			m.ExpectExec("DELETE FROM t_share_media_file WHERE share_id = ANY").WithArgs(pq.Array([]int{1, 2})).WillReturnResult(sqlmock.NewResult(0, 3))
			m.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
			journal := m.ExpectExec("UPDATE t_share_operation_queue SET status='completed'").WithArgs("original-task", int64(3))
			if fail {
				journal.WillReturnError(errors.New("journal unavailable"))
				m.ExpectRollback()
			} else {
				journal.WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			}
			count, err := NewShareRecordDAO(db).ClearSelectedMedia(WithShareOperationTask(context.Background(), "original-task"), []int{1, 2})
			if fail != (err != nil) || !fail && count != 3 {
				t.Fatal(count, err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareSyncRejectsChangedShareVersionBeforeWriting(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectBegin()
	m.ExpectQuery("SELECT id FROM t_share_record WHERE id=\\$1 AND version=\\$2 FOR UPDATE").WithArgs(7, 2).WillReturnError(sql.ErrNoRows)
	m.ExpectRollback()
	if _, err := NewShareRecordDAO(db).SyncFiles(context.Background(), 7, "old-scan", []domain.ShareMedia{{FileName: "old.mkv"}}, 2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareFileCandidateReadsCurrentVersionAndEpisodes(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT f.share_id,f.version,f.file_name").WithArgs(11).WillReturnRows(sqlmock.NewRows([]string{"share_id", "version", "file_name", "source", "status", "available", "type", "result", "episodes"}).AddRow(7, 3, "Show.mkv", "tmdb", "identified", true, "tv", `{"success":true,"tmdb_id":123,"media_type":"tv"}`, `[{"season_number":1,"episode_number":2}]`))
	v, err := NewShareRecordDAO(db).FileCandidate(context.Background(), 11)
	if err != nil || v.Version != 3 || v.Result.TmdbID != 123 || len(v.Episodes) != 1 || v.Episodes[0].EpisodeNumber != 2 {
		t.Fatal(v, err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
