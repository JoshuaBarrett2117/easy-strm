package dao

import (
	"context"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

// TestSyncUnchangedFileKeepsIdentifyVersion 重复扫描不递增识别版本，也不删除已有季集关联。
func TestSyncUnchangedFileKeepsIdentifyVersion(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	d := NewShareRecordDAO(db)
	for _, token := range []string{"scan1", "scan2", "changed"} {
		changed := token == "changed"
		hash := "hash"
		if changed {
			hash = "new-hash"
		}
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id FROM t_share_record").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectQuery("SELECT id,sha1").WithArgs(1, "remote", "电影.mkv").WillReturnRows(sqlmock.NewRows([]string{"id", "sha1"}).AddRow(7, "hash"))
		mock.ExpectExec(`UPDATE t_share_media_file SET file_name=.*version=version\+CASE WHEN \$6 OR file_name IS DISTINCT FROM \$1 THEN 1 ELSE 0 END`).WithArgs("电影.mkv", int64(100), hash, "pick", token, changed, 7).WillReturnResult(sqlmock.NewResult(0, 1))
		if changed {
			mock.ExpectExec("DELETE FROM t_share_media_file_episode").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(1, token).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM t_share_media m").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		count, err := d.SyncFiles(context.Background(), 1, token, []domain.ShareMedia{{RemoteFileID: "remote", FileName: "电影.mkv", FileSize: 100, SHA1: hash, PickCode: "pick"}})
		if err != nil || count != 1 {
			t.Fatalf("count=%d err=%v", count, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
