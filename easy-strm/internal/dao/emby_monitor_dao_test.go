package dao

import (
	"regexp"
	"testing"
	"time"

	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestEmbyMonitorTouchPlaybackCreatesAndUpdates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dao := NewEmbyMonitorDAO(db)
	now := time.Date(2026, 8, 30, 10, 30, 0, 0, time.UTC)
	sample := domain.EmbyPlaybackSample{ServerID: 1, SessionID: "s1", UserID: "u1", ItemID: "i1", ItemName: "影片"}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, item_id, started_at, last_seen_at, is_paused, ended_at FROM t_emby_playback_event
		WHERE server_id=$1 AND session_id=$2 AND ended_at IS NULL FOR UPDATE`)).WithArgs(1, "s1").WillReturnError(sqlmock.ErrCancelled)
	mock.ExpectRollback()
	if _, err = dao.TouchPlayback(sample, now, 45*time.Second); err == nil {
		t.Fatal("查询错误应向上返回")
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, item_id, started_at, last_seen_at, is_paused, ended_at").WithArgs(1, "s1").WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "started_at", "last_seen_at", "is_paused", "ended_at"}).AddRow(2, "i1", now.Add(-20*time.Minute), now.Add(-2*time.Minute), false, nil))
	mock.ExpectExec("UPDATE t_emby_playback_event SET user_id").WithArgs(2, "u1", "", "", "影片", "", "", "", "", "", "", int64(45), now, false).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	seconds, err := dao.TouchPlayback(sample, now, 45*time.Second)
	if err != nil || seconds != 45 {
		t.Fatalf("采样增量限制错误 seconds=%d err=%v", seconds, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
