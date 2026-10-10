package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCacheOverviewReuseCopyClearInvalidationAndContext(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := &CacheAdminService{db: database}
	expectCounts := func(value int) {
		for _, table := range []string{"t_tmdb_cache", "t_identify_cache", "t_media_file_cache"} {
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM " + table).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(value))
		}
	}
	expectCounts(40)
	first, err := service.GetOverviewContext(context.Background())
	if err != nil || first.GeneratedAt == "" || first.Groups[0].Count != 40 {
		t.Fatalf("缓存概览错误：%+v %v", first, err)
	}
	first.Groups[0].Count = 999
	second, err := service.GetOverviewContext(context.Background())
	if err != nil || second.Groups[0].Count != 40 {
		t.Fatal("缓存概览共享了可变响应")
	}
	mock.ExpectExec("DELETE FROM t_media_file_cache").WillReturnResult(sqlmock.NewResult(0, 40))
	if _, err = service.ClearGroup("media_file_cache"); err != nil {
		t.Fatal(err)
	}
	expectCounts(0)
	third, err := service.GetOverviewContext(context.Background())
	if err != nil || third.Groups[0].Count != 0 {
		t.Fatalf("清理后缓存未失效：%+v %v", third, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.GetOverviewContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误未透传：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
