package dao

import (
	"context"
	"easy-strm/internal/domain"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

// TestSyncUnchangedFileKeepsIdentifyVersion 重复扫描不递增识别版本，也不删除已有季集关联。
func TestSyncUnchangedFileKeepsIdentifyVersion(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	d := NewShareRecordDAO(db)
	for _, token := range []string{"scan1", "scan2", "changed"} {
		hash := "hash"
		if token == "changed" {
			hash = "new-hash"
		}
		attempt := &syncAttemptToken{prefix: token}
		expectSyncSession(mock, 2)
		expectSyncBatchLock(mock, 2)
		mock.ExpectQuery(regexp.QuoteMeta(shareSyncBatchSQL)).WithArgs(1, attempt,
			pq.Array([]string{"remote"}), pq.Array([]string{"电影.mkv"}), pq.Array([]int64{100}), pq.Array([]string{hash}), pq.Array([]string{"pick"}),
			pq.Array([]string{"电影.mkv"}), pq.Array([]string{hash}), pq.Array([]bool{false}), pq.Array([]int64{0}), domain.MetadataSourceAuto).
			WillReturnRows(sqlmock.NewRows([]string{"written", "orphans"}).AddRow(1, "{}"))
		mock.ExpectCommit()
		expectSyncBatchLock(mock, 2)
		mock.ExpectExec("UPDATE t_share_media_file SET available=FALSE").WithArgs(1, attempt).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		expectSyncRelease(mock)
		count, err := d.SyncFiles(context.Background(), 1, token, []domain.ShareMedia{{RemoteFileID: "remote", FileName: "电影.mkv", FileSize: 100, SHA1: hash, PickCode: "pick"}})
		if err != nil || count != 1 {
			t.Fatalf("count=%d err=%v", count, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	seen := shareSyncBatchSQL[strings.Index(shareSyncBatchSQL, "), seen_only AS ("):strings.Index(shareSyncBatchSQL, "), inserted AS (")]
	for _, forbidden := range []string{"version=", "result=", "media_id=", "status=", "metadata_source=", "DELETE"} {
		if strings.Contains(seen, forbidden) {
			t.Fatalf("重复扫描意外修改识别状态：%s", forbidden)
		}
	}
}
