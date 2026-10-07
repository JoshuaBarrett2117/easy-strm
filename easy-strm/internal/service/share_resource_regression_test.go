package service

import (
	"context"
	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

// TestClearUnrelatedShareDuringIdentification 识别不应阻塞无关分享的清空。
func TestClearUnrelatedShareDuringIdentification(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
	release, err := s.Coordinator().acquire(context.Background(), nil, shareResource{key: shareKey(1)}, shareResource{key: fileKey(11), exclusive: true})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM t_share_record").WithArgs(2).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	mock.ExpectExec("DELETE FROM t_share_media_file").WithArgs(2).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if _, err := s.ClearMedia(context.Background(), 2); err != nil {
		t.Fatal("无关分享被全局识别锁阻塞：", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
