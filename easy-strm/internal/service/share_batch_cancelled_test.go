package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"testing"
)

type cancelledBatchParser struct {
	calls    []string
	ordinary bool
}

func (p *cancelledBatchParser) ParseShareLink(_ context.Context, url, password string) (*domain.ParseShareResponse, error) {
	p.calls = append(p.calls, url)
	if url == "cancelled" {
		if p.ordinary {
			return nil, fmt.Errorf("访问密码错误")
		}
		return nil, ErrShareCancelled
	}
	return &domain.ParseShareResponse{}, nil
}

// TestCancelledShareDoesNotFailBatch 验证识别阶段只读已同步文件，不再调用分享解析器。
func TestCancelledShareDoesNotFailBatch(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	dao.InitTaskRedisDAO(client)
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	if err := tasks.Create("skip-test", "share_identify", "测试"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	rows := sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"}).AddRow(1, "auto", "取消分享", "cancelled", "", "", 1, "now", "now", true, 11, "旧媒体.mkv", "auto", "pending", nil, "", 1)
	mock.ExpectQuery("FROM \\(SELECT .* FROM t_share_record").WillReturnRows(rows)
	parser := &cancelledBatchParser{}
	s := &ShareRecordService{dao: dao.NewShareRecordDAO(db), tasks: tasks, parser: parser}
	s.runBatchIdentify(context.Background(), "skip-test", nil, false, nil)
	task, err := tasks.Get("skip-test")
	if err != nil || task["status"] != "completed" || len(parser.calls) != 0 {
		t.Fatalf("%+v calls=%v err=%v", task, parser.calls, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
