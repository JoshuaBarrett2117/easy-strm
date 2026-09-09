package dao

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

// TestShareCandidateIdempotency 同一路径复用原ID，首次插入与查询失败均保留事务边界。
func TestShareCandidateIdempotency(t *testing.T) {
	for _, exists := range []bool{true, false} {
		db, mock, _ := sqlmock.New()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id FROM t_share_record").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
		q := mock.ExpectQuery("SELECT id,version FROM t_share_media").WithArgs(9, "剧集")
		if exists {
			q.WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(7, 2))
		} else {
			q.WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("INSERT INTO t_share_media").WithArgs(9, "剧集", "auto").WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(7, 1))
		}
		mock.ExpectCommit()
		m := &domain.ShareMedia{ShareID: 9, FileName: "剧集", MetadataSource: "auto"}
		if err := NewShareRecordDAO(db).AddMediaCandidate(context.Background(), m); err != nil || m.ID != 7 {
			t.Fatal(m, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
