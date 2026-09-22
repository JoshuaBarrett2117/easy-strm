package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"testing"
	"time"
)

type blockedSyncParser struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockedSyncParser) ParseShareLink(ctx context.Context, _, _ string) (*domain.ParseShareResponse, error) {
	close(p.started)
	select {
	case <-p.release:
	case <-ctx.Done():
	}
	return nil, context.Canceled
}

// TestShareSyncParallelAndDuplicate 同步允许与识别并行，重复请求复用任务，清空仍受保护。
func TestShareSyncParallelAndDuplicate(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	dao.InitTaskRedisDAO(client)
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM \(SELECT`).WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"}).AddRow(1, "auto", "分享", "https://115.com/s/abc", "", "", 1, "now", "now", false, nil, nil, nil, nil, nil, nil, nil))
	p := &blockedSyncParser{started: make(chan struct{}), release: make(chan struct{})}
	s := NewShareRecordService(dao.NewShareRecordDAO(db), nil, tasks, p)
	s.identifyMu.Lock()
	id, err := s.StartRecordSync(context.Background(), 1)
	s.identifyMu.Unlock()
	if err != nil {
		t.Fatal("识别运行时应允许同步:", err)
	}
	defer func() {
		close(p.release)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			task, _ := tasks.Get(id)
			if task["status"] == "failed" {
				return
			}
			time.Sleep(time.Millisecond * 10)
		}
		t.Error("同步未结束")
	}()
	select {
	case <-p.started:
	case <-time.After(3 * time.Second):
		t.Fatal("同步未启动")
	}
	duplicate, err := s.StartBatchSync(context.Background(), []int{1, 1})
	if err != nil || duplicate != id {
		t.Fatalf("重复同步未复用任务: %s %v", duplicate, err)
	}
	if _, err := s.StartRecordSync(context.Background(), 2); err == nil {
		t.Fatal("不同同步应继续串行")
	}
	if _, err := s.ClearMedia(context.Background(), 1); err == nil {
		t.Fatal("同步时不允许清空")
	}
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM \(SELECT`).WillReturnRows(sqlmock.NewRows([]string{"id", "media_type", "name", "url", "password", "note", "version", "created_at", "updated_at", "share_cancelled", "mid", "file_name", "source", "status", "result", "error", "mversion"}))
	identifyID, err := s.StartRecordIdentify(context.Background(), 1)
	if err != nil {
		t.Fatal("同步运行时应允许启动识别:", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := tasks.Get(identifyID)
		if err != nil {
			t.Fatal(err)
		}
		if task["status"] == "completed" {
			break
		}
		if task["status"] == "failed" || time.Now().After(deadline) {
			t.Fatalf("识别未完成: %v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
