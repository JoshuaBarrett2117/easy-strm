package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
)

func expectOverviewServer(mock sqlmock.Sqlmock, endpoint string, enabled bool) {
	mock.ExpectQuery("FROM t_emby_server WHERE id").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "url", "key", "enabled", "default", "created", "updated", "port"}).AddRow(1, "测试", endpoint, "测试密钥", enabled, true, time.Now(), time.Now(), 0))
}

func TestEmbyOverviewPluginFirstAndSnapshotIsolation(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte("[]"))
	}))
	defer server.Close()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewEmbyMonitorService(dao.NewEmbyServerDAO(database), dao.NewEmbyMonitorDAO(database), server.Client(), nil)
	expectOverviewServer(mock, server.URL, true)
	mock.ExpectExec("INSERT INTO t_emby_monitor_state").WillReturnResult(sqlmock.NewResult(0, 1))
	first, err := service.GetOverview(1)
	if err != nil || first.Meta.Source != embyMonitorSourcePlugin {
		t.Fatalf("插件成功不应读取本地聚合：%+v %v", first, err)
	}
	first.Summary.TotalSeconds = 999
	before := requests.Load()
	expectOverviewServer(mock, server.URL, true)
	second, err := service.GetOverview(1)
	if err != nil || second.Summary.TotalSeconds != 0 || requests.Load() != before {
		t.Fatalf("缓存必须隔离可变响应且避免重复网络请求：%+v %v", second, err)
	}
	expectOverviewServer(mock, server.URL, false)
	if _, err = service.GetOverview(1); err == nil {
		t.Fatal("已停用实例不得返回缓存")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyOverviewCancelledDoesNotQuery(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewEmbyMonitorService(dao.NewEmbyServerDAO(database), dao.NewEmbyMonitorDAO(database), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.GetOverviewContext(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未透传：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyOverviewSingleflightWaiterCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-request.Context().Done():
				return
			}
		}
		writer.Write([]byte("[]"))
	}))
	defer server.Close()
	defer close(release)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.MatchExpectationsInOrder(false)
	expectOverviewServer(mock, server.URL, true)
	expectOverviewServer(mock, server.URL, true)
	mock.ExpectExec("INSERT INTO t_emby_monitor_state").WillReturnResult(sqlmock.NewResult(0, 1))
	service := NewEmbyMonitorService(dao.NewEmbyServerDAO(database), dao.NewEmbyMonitorDAO(database), server.Client(), nil)
	leaderCtx, stopLeader := context.WithCancel(context.Background())
	defer stopLeader()
	done := make(chan error, 1)
	go func() { _, err := service.GetOverviewContext(leaderCtx, 1); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("概览未开始")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err = service.GetOverviewContext(ctx, 1); !errors.Is(err, context.DeadlineExceeded) || requests.Load() != 1 {
		t.Fatalf("等待者取消不应增加请求或取消首请求：%v 请求=%d", err, requests.Load())
	}
	release <- struct{}{}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("首请求未完成")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmbyOverviewLocalFallbackKeepsTotals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/emby/Sessions" {
			writer.Write([]byte("[]"))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	expectOverviewServer(mock, server.URL, true)
	mock.ExpectExec("INSERT INTO t_emby_monitor_state").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT MIN").WillReturnRows(sqlmock.NewRows([]string{"start"}).AddRow(time.Now()))
	mock.ExpectQuery("COALESCE\\(SUM").WillReturnRows(sqlmock.NewRows([]string{"today", "week", "month", "total", "today_users", "week_users", "month_users"}).AddRow(10, 20, 30, 400, 1, 2, 3))
	mock.ExpectQuery("date_trunc").WillReturnRows(sqlmock.NewRows([]string{"hour", "users", "seconds"}).AddRow(time.Now(), 2, 10))
	service := NewEmbyMonitorService(dao.NewEmbyServerDAO(database), dao.NewEmbyMonitorDAO(database), server.Client(), nil)
	result, err := service.GetOverview(1)
	if err != nil || result.Meta.Source != embyMonitorSourceLocal || result.Summary.TotalSeconds != 400 || result.Summary.PeakUsers != 2 || result.Meta.DegradedReason == "" {
		t.Fatalf("本地回退契约改变：%+v %v", result, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
