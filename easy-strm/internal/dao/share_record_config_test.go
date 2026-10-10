package dao

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShareRecordConfigPointReadAndMissing(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT id,media_type,name,url,password,note,version,created_at::text,updated_at::text,share_cancelled FROM t_share_record WHERE id=\\$1").WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created", "updated", "cancelled"}).AddRow(9, "tv", "分享", "链接", "访问码", "备注", 4, "创建", "更新", true))
	record, err := NewShareRecordDAO(database).GetRecord(context.Background(), 9)
	if err != nil || record.Version != 4 || !record.ShareCancelled || record.Password != "访问码" || len(record.Media) != 0 {
		t.Fatalf("配置或版本丢失：%+v %v", record, err)
	}
	mock.ExpectQuery("FROM t_share_record WHERE id").WithArgs(10).WillReturnError(sql.ErrNoRows)
	if _, err = NewShareRecordDAO(database).GetRecord(context.Background(), 10); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("不存在错误丢失：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
