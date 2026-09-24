package service

import (
	"context"
	"easy-strm/internal/dao"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestShareBatchUsesConfiguredWorkersAndProgress(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.MatchExpectationsInOrder(false)
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	dao.InitTaskRedisDAO(client)
	t.Cleanup(func() { dao.InitTaskRedisDAO(nil); client.Close() })
	tasks := NewTaskService(dao.NewTaskRedisDAO(client))
	if err := tasks.Create("pool", "share_identify", "fixture"); err != nil {
		t.Fatal(err)
	}
	rows := func() *sqlmock.Rows {
		r := sqlmock.NewRows([]string{"id", "type", "fid", "name", "source", "status", "result", "version"})
		for i := 1; i <= 4; i++ {
			r.AddRow(7, "tv", i, fmt.Sprintf("Show%d S01E01.mkv", i), "tmdb", "pending", nil, 1)
		}
		return r
	}
	m.ExpectQuery(`SELECT s.id,s.media_type,f.id`).WillReturnRows(rows())
	m.ExpectQuery(`CASE WHEN f.status='identified'`).WillReturnRows(rows())
	for i := 0; i < 4; i++ {
		m.ExpectBegin()
		m.ExpectExec("UPDATE t_share_media_file SET status").WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectQuery("SELECT media_id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(nil))
		m.ExpectExec("DELETE FROM t_share_media_file_episode").WillReturnResult(sqlmock.NewResult(0, 0))
		m.ExpectCommit()
	}
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		fmt.Fprint(w, `{"results":[]}`)
	}))
	defer server.Close()
	tmdb := NewTmdbService("fixture", nil)
	tmdb.baseURL = server.URL
	tmdb.httpClient = server.Client()
	s := NewShareRecordService(dao.NewShareRecordDAO(db), tmdb, tasks, nil)
	done := make(chan struct{})
	go func() { defer close(done); s.runBatchIdentify(context.Background(), "pool", nil, false, []int{7}) }()
	for i := 0; i < 4; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			close(release)
			<-done
			t.Fatal("actual batch failed to start four workers")
		}
	}
	close(release)
	<-done
	task, err := tasks.Get("pool")
	if err != nil || fmt.Sprint(task["failed_files"]) != "4" || fmt.Sprint(task["processed_files"]) != "4" {
		t.Fatalf("task=%v err=%v", task, err)
	}
	if calls.Load() != 4 {
		t.Fatal(calls.Load())
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
