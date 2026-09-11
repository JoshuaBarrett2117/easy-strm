package dao

import (
	"context"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
)

func TestShareStrmDAOQueriesAndMapping(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	d := NewShareRecordDAO(db)
	ctx := context.Background()
	mock.ExpectQuery(`WITH ranked AS .*`).WithArgs("tv", 100).WillReturnRows(sqlmock.NewRows([]string{"id", "key", "url", "password", "file", "result", "remaining"}).AddRow(101, "tmdb:tv:1", "url", "code", "Show", `{"success":true,"media_type":"tv"}`, 1))
	sources, err := d.StrmSources(ctx, domain.ShareLibraryQuery{MediaType: "tv"}, 100)
	if err != nil || len(sources) != 1 || sources[0].ID != 101 {
		t.Fatalf("%+v %v", sources, err)
	}
	mock.ExpectExec(`INSERT INTO t_share_strm`).WithArgs("id", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	if err = d.SaveStrmEntry(ctx, domain.ShareStrmEntry{ID: "id", FileID: "fid"}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT payload FROM t_share_strm`).WithArgs("id").WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(`{"id":"id","file_id":"fid"}`))
	entry, err := d.GetStrmEntry(ctx, "id")
	if err != nil || entry.FileID != "fid" {
		t.Fatalf("%+v %v", entry, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs("key").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	release, err := d.LockStrmPlayback(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShareStrmLockSurvivesRequestCancellationUntilRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs("key").WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, cancel := context.WithCancel(context.Background())
	release, err := NewShareRecordDAO(db).LockStrmPlayback(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	// 取消请求不能自动把占用的事务连接还给池；115调用返回后再显式释放。
	time.Sleep(10 * time.Millisecond)
	if db.Stats().InUse != 1 {
		t.Fatal("请求取消提前释放了转存锁")
	}
	mock.ExpectRollback()
	release()
	if db.Stats().InUse != 0 {
		t.Fatal("转存结束后连接未释放")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
