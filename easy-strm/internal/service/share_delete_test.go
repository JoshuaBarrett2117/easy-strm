package service

import (
	"context"
	"easy-strm/internal/dao"
	"errors"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDeleteShareRejectsBusyOrInvalid(t *testing.T) {
	s := &ShareRecordService{}
	if err := s.Delete(context.Background(), 0); err == nil {
		t.Fatal("应拒绝无效ID")
	}
	s.activeSyncID = "sync"
	if err := s.Delete(context.Background(), 1); err == nil {
		t.Fatal("同步期间不得删除")
	}
	s.activeSyncID = ""
	s.identifyMu.RLock()
	if err := s.Delete(context.Background(), 1); err == nil {
		t.Fatal("识别期间不得删除")
	}
	s.identifyMu.RUnlock()
	s.strmExportMu = &sync.Mutex{}
	s.strmExportMu.Lock()
	defer s.strmExportMu.Unlock()
	if err := s.Delete(context.Background(), 1); err == nil {
		t.Fatal("导出期间不得删除")
	}
}

func TestDeleteShareCleanupFailureKeepsRecords(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	p := filepath.Join(t.TempDir(), "directory.strm")
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT url FROM t_share_record").WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/abc"))
	mock.ExpectQuery("SELECT id FROM t_share_strm").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("entry-1"))
	mock.ExpectQuery("SELECT output_path").WillReturnRows(sqlmock.NewRows([]string{"path", "linked"}).AddRow(p, true))
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
	if err := s.Delete(context.Background(), 7); err == nil {
		t.Fatal("目录不得作为STRM删除")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteShareRetryAfterDatabaseFailure(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	p := filepath.Join(t.TempDir(), "historical.strm")
	if err := os.WriteFile(p, []byte("https://host/share-strm/entry-1"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectQuery("SELECT url FROM t_share_record").WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/abc"))
		mock.ExpectQuery("SELECT id FROM t_share_strm").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("entry-1"))
		mock.ExpectQuery("SELECT output_path").WillReturnRows(sqlmock.NewRows([]string{"path", "linked"}).AddRow(p, attempt > 0))
		mock.ExpectExec("INSERT INTO t_strm_export_history").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectBegin()
		if attempt == 0 {
			mock.ExpectExec("DELETE FROM t_strm_file").WillReturnError(errors.New("database unavailable"))
			mock.ExpectRollback()
		} else {
			for _, table := range []string{"t_strm_file", "t_strm_export_history", "t_strm_export_state", "t_share_strm", "t_share_record", "t_share_media"} {
				mock.ExpectExec("DELETE FROM " + table).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
		}
		err := s.Delete(context.Background(), 7)
		if (attempt == 0) != (err != nil) {
			t.Fatalf("attempt=%d err=%v", attempt, err)
		}
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("STRM未删除")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteShareWithoutExports(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT url FROM t_share_record").WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/abc"))
	mock.ExpectQuery("SELECT id FROM t_share_strm").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_share_record").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_share_media").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil).Delete(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteShareRemovesOnlyMatchingStrm(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	dir := t.TempDir()
	own, other := filepath.Join(dir, "own.strm"), filepath.Join(dir, "other.strm")
	os.WriteFile(own, []byte("http://localhost/share-strm/entry-1\n"), 0600)
	os.WriteFile(other, []byte("http://localhost/share-strm/entry-2\n"), 0600)
	mock.ExpectQuery("SELECT url FROM t_share_record").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"url"}).AddRow("https://115.com/s/abc"))
	mock.ExpectQuery("SELECT id FROM t_share_strm").WithArgs("abc").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("entry-1"))
	mock.ExpectQuery("SELECT output_path").WillReturnRows(sqlmock.NewRows([]string{"path", "linked"}).AddRow(own, true).AddRow(other, false))
	mock.ExpectExec("INSERT INTO t_strm_export_history").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	for _, table := range []string{"t_strm_file", "t_strm_export_history", "t_strm_export_state", "t_share_strm", "t_share_record", "t_share_media"} {
		mock.ExpectExec("DELETE FROM " + table).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, nil, nil)
	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatal("关联STRM未删除")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("其他分享STRM被删除")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
