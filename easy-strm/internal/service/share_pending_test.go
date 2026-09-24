package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"testing"
)

// TestPendingIdentifySelection 继续识别不能覆盖已识别记录或重新尝试失败项。
func TestPendingIdentifySelection(t *testing.T) {
	for _, status := range []string{"pending", "", "failed", "identified", "masked"} {
		m := domain.ShareMedia{Status: status}
		want := status == "pending" || status == ""
		if got := shouldIdentifyShareMedia(m, "auto", false, true); got != want {
			t.Fatalf("status=%q got=%v", status, got)
		}
	}
	m := domain.ShareMedia{Status: "identified", Result: &domain.TmdbIdentifyResult{MediaType: "tv", Success: true}}
	if shouldIdentifyShareMedia(m, "movie", false, true) {
		t.Fatal("继续识别不能覆盖媒体类型不同的已识别项")
	}
	if !shouldIdentifyShareMedia(m, "movie", false, false) {
		t.Fatal("普通识别保留媒体类型修正行为")
	}
}

// TestRecordPendingScope 续跑单个分享不得处理其他分享的待识别内容，也不重新扫描。
func TestRecordPendingScope(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	dao.InitTaskRedisDAO(client)
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	if err := tasks.Create("record-pending", "share_identify", "测试"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT s.id,s.media_type,f.id.*AND NOT s.share_cancelled.*s.id=ANY\(\$1\).*f.status IN \('pending',''\)`).WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "fid", "name", "source", "status", "result", "version"}))
	parser := &cancelledBatchParser{}
	s := &ShareRecordService{dao: dao.NewShareRecordDAO(db), tasks: tasks, parser: parser}
	s.runBatchIdentify(context.Background(), "record-pending", nil, false, []int{1}, true)
	task, err := tasks.Get("record-pending")
	if err != nil || task["status"] != "completed" || len(parser.calls) != 0 {
		t.Fatal(task, err, parser.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestFailedIdentifySelection 失败重试不能覆盖待识别、已识别或脱敏记录。
func TestFailedIdentifySelection(t *testing.T) {
	for _, status := range []string{"pending", "", "failed", "identified", "masked"} {
		got := shouldIdentifyShareMedia(domain.ShareMedia{Status: status}, "auto", true, false, true)
		if got != (status == "failed") {
			t.Fatalf("status=%q got=%v", status, got)
		}
	}
}
