package dao

import (
	"context"
	"fmt"
	"testing"

	"easy-strm/internal/domain"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestUpdateSharePreservesMedia 编辑分享只更新主表，禁止删除重建媒体及识别结果。
func TestUpdateSharePreservesMedia(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE t_share_record SET").WithArgs("合集", "url", "pass", "note", 1, 2, "tv").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	mock.ExpectCommit()
	r := &domain.ShareRecord{ID: 1, Version: 2, Name: "合集", URL: "url", Password: "pass", Note: "note", MediaType: "tv"}
	if err := NewShareRecordDAO(db).Update(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestShareMediaIdentifyConflict 验证手动关联保存成功，以及后台更新后旧版本不能静默覆盖。
func TestShareMediaIdentifyConflict(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec("UPDATE t_share_media SET status").WithArgs("identified", sqlmock.AnyArg(), "", 7, 2).WillReturnResult(sqlmock.NewResult(0, affected))
		err = NewShareRecordDAO(db).Identify(context.Background(), domain.ShareMedia{ID: 7, Version: 2}, "identified", &domain.TmdbIdentifyResult{Success: true, Title: "七龙珠", MediaType: "tv", TmdbID: 12609}, "")
		if (err != nil) != (affected == 0) {
			t.Fatalf("affected=%d err=%v", affected, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

// TestShareRecordListPaginatesParents 验证大分享的媒体不会占用其他分享名额，且过滤和页码作用于主表。
func TestShareRecordListPaginatesParents(t *testing.T) {
	for _, keyword := range []string{"", "合集"} {
		t.Run("keyword="+keyword, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			count := mock.ExpectQuery(`SELECT count\(\*\) FROM t_share_record s`)
			if keyword != "" {
				count.WithArgs("%合集%")
			}
			count.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(23))
			// 限制必须在 JOIN 之前，SQL 模式会使旧的关联行分页实现失败。
			query := mock.ExpectQuery(`FROM \(SELECT .* FROM t_share_record s.*ORDER BY s.id DESC LIMIT \$\d OFFSET \$\d\) s LEFT JOIN t_share_media`)
			if keyword != "" {
				query.WithArgs("%合集%", 20, 20)
			} else {
				query.WithArgs(20, 20)
			}
			rows := sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"})
			for i := 1; i <= 404; i++ {
				rows.AddRow(3, "tv", "大合集", "https://115.com/s/a", "", "", 1, "now", "now", true, i, fmt.Sprintf("剧集%d", i), "tmdb", "identified", []byte(`{"success":true,"title":"剧集"}`), "", 1)
			}
			rows.AddRow(2, "movie", "另一分享", "https://115.com/s/b", "", "", 1, "now", "now", false, nil, nil, nil, nil, nil, nil, nil)
			query.WillReturnRows(rows)
			page, err := NewShareRecordDAO(db).List(context.Background(), domain.ShareRecordQuery{Page: 2, PageSize: 20, Keyword: keyword})
			if err != nil {
				t.Fatal(err)
			}
			if !page.Data[0].ShareCancelled || page.Data[1].ShareCancelled || page.Total != 23 || len(page.Data) != 2 || len(page.Data[0].Media) != 404 || page.Data[1].ID != 2 || len(page.Data[1].Media) != 0 {
				t.Fatalf("分页丢失分享或媒体: total=%d records=%d", page.Total, len(page.Data))
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
