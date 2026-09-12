package dao

import (
	"context"
	"easy-strm/internal/domain"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"testing"
)

// TestShareMediaPage 验证数据库分页参数、全局重复统计和空页返回。
func TestShareMediaPage(t *testing.T) {
	for _, duplicates := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery("WITH ranked AS").WithArgs(9, 10, 10).WillReturnRows(sqlmock.NewRows([]string{"total", "duplicates", "data"}).AddRow(25, 3, `[{"id":11,"share_id":9,"result":{"success":true,"media_type":"tv"}}]`))
		got, err := NewShareRecordDAO(db).ListMedia(context.Background(), 9, 2, 10, duplicates)
		if err != nil || got.Total != 25 || got.DuplicateCount != 3 || len(got.Data) != 1 || got.Data[0].ID != 11 {
			t.Fatalf("%+v %v", got, err)
		}
		mock.ExpectQuery("WITH ranked AS").WithArgs(9, 10, 90).WillReturnRows(sqlmock.NewRows([]string{"total", "duplicates", "data"}).AddRow(25, 3, `[]`))
		got, err = NewShareRecordDAO(db).ListMedia(context.Background(), 9, 10, 10, duplicates)
		if err != nil || got.Total != 25 || got.Data == nil || len(got.Data) != 0 {
			t.Fatalf("%+v %v", got, err)
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

// TestShareSummary 不加载媒体详情，返回完整数量统计。
func TestShareSummary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT s.id").WithArgs(20, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "url", "password", "note", "version", "created", "updated", "cancelled", "files", "identified", "failed", "pending", "unavailable", "media", "ignored"}).AddRow(1, "auto", "分享", "url", "", "", 1, "", "", false, 400, 380, 10, 5, 3, 200, 2))
	got, err := NewShareRecordDAO(db).List(context.Background(), domain.ShareRecordQuery{Summary: true})
	if err != nil || len(got.Data) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	r := got.Data[0]
	if r.FileCount != 400 || r.MediaCount != 200 || r.IdentifiedCount != 380 || r.UnavailableCount != 3 || len(r.Media) != 0 {
		t.Fatalf("%+v", r)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
