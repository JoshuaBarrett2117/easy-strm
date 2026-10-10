package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestDashboardResourceMonitorNeverReadsFilesOrCapacity(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectDashboardAccounts(mock)
	mock.ExpectQuery("FROM t_media_source ORDER BY").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "path", "watch_path", "account", "priority", "enabled", "target", "media_type", "metadata_source", "conflict", "mode", "auto", "watch", "interval", "library", "created", "updated"}))
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	mini.Push("easy_strm:task:list", "运行中")
	mini.Set("easy_strm:task:运行中", `{"task_id":"运行中","status":"running"}`)
	provider := &fakeDashboardStorageProvider{}
	service := NewDashboardService(dao.NewCloud115DAO(), dao.NewMediaSourceDAO(), nil, dao.NewTaskRedisDAO(client), provider, nil)
	result, err := service.GetDashboardResourceMonitorContext(context.Background())
	if err != nil || result.RunningTasks != 1 || result.ActiveAccounts != 2 || len(provider.calls) != 0 {
		t.Fatalf("监控不应加载未返回的文件或容量统计：%+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.GetDashboardResourceMonitorContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未透传：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardRequestReusesAccountAndTaskReads(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectDashboardAccounts(mock)
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	mini.Push("easy_strm:task:list", "任务")
	mini.Set("easy_strm:task:任务", `{"task_id":"任务","status":"running"}`)
	service := NewDashboardService(dao.NewCloud115DAO(), nil, nil, dao.NewTaskRedisDAO(client), nil, nil).forDashboardRequest(context.Background())
	stats := &DashboardStats{}
	if err := service.fillAccountStats(stats); err != nil {
		t.Fatal(err)
	}
	if err := service.fillStorageStats(stats); err != nil {
		t.Fatal(err)
	}
	if err := service.fillTaskStats(stats); err != nil {
		t.Fatal(err)
	}
	mini.Del("easy_strm:task:任务")
	tasks, err := service.dashboardTasks()
	if err != nil || len(tasks) != 1 || stats.Tasks.Running != 1 || len(stats.Storage.Accounts) != 2 {
		t.Fatalf("同请求内视图不一致：%+v %v", stats, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardFileStatsCacheCopyExpiryAndContext(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	mock.ExpectQuery("SELECT count\\(\\*\\),max\\(update_time\\) FROM t_strm_file").WillReturnRows(sqlmock.NewRows([]string{"total", "last"}).AddRow(150003, time.Now()))
	service := NewDashboardService(nil, nil, dao.NewStrmFileDAO(), nil, nil, nil)
	first, err := service.dashboardFileStats()
	if err != nil || first.Total != 150003 || first.LastGenerationTime == nil {
		t.Fatalf("文件统计错误：%+v %v", first, err)
	}
	*first.LastGenerationTime = "外部修改"
	second, err := service.dashboardFileStats()
	if err != nil || *second.LastGenerationTime == "外部修改" {
		t.Fatal("统计缓存返回了共享指针")
	}
	service.fileStatsCache.Lock()
	service.fileStatsCache.expires = time.Now().Add(-time.Second)
	service.fileStatsCache.Unlock()
	mock.ExpectQuery("SELECT count\\(\\*\\),max").WillReturnRows(sqlmock.NewRows([]string{"total", "last"}).AddRow(150004, nil))
	third, err := service.dashboardFileStats()
	if err != nil || third.Total != 150004 || third.LastGenerationTime != nil {
		t.Fatalf("过期统计未刷新：%+v %v", third, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.forDashboardRequest(ctx).dashboardFileStats(); !errors.Is(err, context.Canceled) {
		t.Fatalf("统计取消未透传：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
