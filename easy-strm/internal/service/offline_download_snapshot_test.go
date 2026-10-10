package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestOfflinePollingSnapshotNoSQLCopyAndExactTotal(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	key := offlineSnapshotKey{page: 1, size: 20}
	service := &OfflineDownloadService{recordDAO: dao.NewOfflineDownloadTaskDAO(database), snapshots: map[offlineSnapshotKey]*offlineSnapshotEntry{
		key: {sampled: time.Now(), result: &domain.OfflineDownloadTaskListResponse{Total: 127, SnapshotAt: "2026-10-11T00:00:00Z", Data: []domain.OfflineDownloadTask{{ID: 9, Name: "原名"}}}},
	}, snapshotSlots: make(chan struct{}, 2)}
	first, err := service.ListSnapshot(context.Background(), 0, "", 1, 20)
	if err != nil || first.Total != 127 || len(first.Data) != 1 || first.SnapshotAt == "" {
		t.Fatalf("总数不能替换为页长：%+v %v", first, err)
	}
	first.Data[0].Name = "外部修改"
	second, err := service.ListSnapshot(context.Background(), 0, "", 1, 20)
	if err != nil || second.Data[0].Name != "原名" {
		t.Fatal("快照响应不可共享可变切片")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.ListSnapshot(ctx, 0, "", 1, 20); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误未透传：%v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOfflinePollingColdSnapshotBackgroundSingleflight(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("WITH filtered AS NOT MATERIALIZED").WithArgs(0, "", 20, 0).WillDelayFor(30 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"total", "data", "snapshot_at"}).AddRow(40, `[]`, "2026-10-11T00:00:00Z"))
	service := &OfflineDownloadService{recordDAO: dao.NewOfflineDownloadTaskDAO(database)}
	var group sync.WaitGroup
	for index := 0; index < 10; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := service.ListSnapshot(context.Background(), 0, "", 1, 20)
			if err == nil && result.Total != 40 {
				t.Errorf("冷快照不得伪造总数：%+v", result)
			}
		}()
	}
	group.Wait()
	deadline := time.Now().Add(time.Second)
	for {
		service.syncMu.Lock()
		entry := service.snapshots[offlineSnapshotKey{page: 1, size: 20}]
		ready := entry != nil && !entry.refreshing && entry.result != nil
		service.syncMu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("后台快照未完成")
		}
		time.Sleep(time.Millisecond)
	}
	result, err := service.ListSnapshot(context.Background(), 0, "", 1, 20)
	if err != nil || result.Total != 40 || len(result.Data) != 0 || result.SnapshotAt == "" {
		t.Fatalf("空页仍需保持精确总数：%+v %v", result, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
