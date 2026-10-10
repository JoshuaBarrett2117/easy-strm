package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

type boundedFileClient struct {
	fakeFileOperationCloud115Client
	active  atomic.Int64
	maximum atomic.Int64
	started chan struct{}
	release chan struct{}
}

func (client *boundedFileClient) MoveFile115(string, string, int, string) error {
	active := client.active.Add(1)
	defer client.active.Add(-1)
	for previous := client.maximum.Load(); active > previous; previous = client.maximum.Load() {
		if client.maximum.CompareAndSwap(previous, active) {
			break
		}
	}
	client.started <- struct{}{}
	<-client.release
	return nil
}

func TestBatchFileOperationBoundedWorkersReuseAndCancellation(t *testing.T) {
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	accountID := 7
	expectFileOperationMediaSourceByID(mock, 1, domain.SourceTypeCloud115, "0", &accountID)
	expectCloud115ByID(mock, 7, "测试Cookie")
	client := &boundedFileClient{started: make(chan struct{}, 20), release: make(chan struct{})}
	service := NewFileOperationService(NewMediaSourceService(dao.NewMediaSourceDAO(), nil), dao.NewCloud115DAO(), client)
	items := make([]domain.BatchOperationItem, 20)
	for index := range items {
		items[index] = domain.BatchOperationItem{FileID: fmt.Sprint(index + 1), TargetPath: "0"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan *domain.BatchOperationResult, 1)
	failed := make(chan error, 1)
	go func() {
		result, err := service.BatchOperationContext(ctx, "move", 1, 0, items)
		done <- result
		failed <- err
	}()
	for index := 0; index < 4; index++ {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			cancel()
			close(client.release)
			t.Fatal("有界工作协程未启动")
		}
	}
	cancel()
	close(client.release)
	select {
	case result := <-done:
		if err := <-failed; err != nil || result.Total != 20 || result.Success != 4 || result.Failed != 16 || len(result.Results) != 20 || client.maximum.Load() > 4 {
			t.Fatalf("并发或取消结果错误：%+v %v 最大并发=%d", result, err, client.maximum.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("取消后工作协程未结束")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBatchFileOperationCapBeforeLookupAndCancelledCopy(t *testing.T) {
	service := &FileOperationService{}
	if _, err := service.BatchOperationContext(context.Background(), "move", 1, 0, make([]domain.BatchOperationItem, 501)); err == nil {
		t.Fatal("超过硬上限的批次不得查询或执行")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.requestContext = ctx
	source, target := filepath.Join(t.TempDir(), "源文件"), filepath.Join(t.TempDir(), "目标文件")
	if err := os.WriteFile(source, []byte("内容"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.copyFile(source, target); !errors.Is(err, context.Canceled) {
		t.Fatalf("复制取消未保留：%v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("已取消复制不得创建目标文件")
	}
}

func TestBatchFileOperationMissing115AccountFailsWithoutPanic(t *testing.T) {
	accountID := 7
	worker := &FileOperationService{preparedAccounts: map[int]fileOperationAccount{accountID: {}}}
	if account, err := worker.operationAccount(accountID); account != nil || err == nil {
		t.Fatalf("已删除账号必须返回明确错误，不能进入SDK：账号=%v 错误=%v", account, err)
	}
	mock, cleanup := setupServiceMockDB(t)
	defer cleanup()
	expectFileOperationMediaSourceByID(mock, 1, domain.SourceTypeCloud115, "0", &accountID)
	mock.ExpectQuery("SELECT id, name, cookie, COALESCE").WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	service := NewFileOperationService(NewMediaSourceService(dao.NewMediaSourceDAO(), nil), dao.NewCloud115DAO(), &fakeFileOperationCloud115Client{})
	items := []domain.BatchOperationItem{{FileID: "1", NewName: "新名称"}, {FileID: "2", NewName: "新名称"}}
	result, err := service.BatchOperationContext(context.Background(), "rename", 1, 0, items)
	if err != nil || result.Total != 2 || result.Success != 0 || result.Failed != 2 {
		t.Fatalf("缺失账号应逐项失败并保持结果契约：%+v %v", result, err)
	}
	for _, item := range result.Results {
		if item.Success || !strings.Contains(item.Message, "115账号不存在") {
			t.Fatalf("缺失账号错误未保留：%+v", item)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBatchFileOperationOversizedRejectsBeforeResultAllocation(t *testing.T) {
	service := &FileOperationService{}
	items := make([]domain.BatchOperationItem, 50001)
	measurement := testing.Benchmark(func(benchmark *testing.B) {
		for iteration := 0; iteration < benchmark.N; iteration++ {
			if _, err := service.BatchOperationContext(context.Background(), "move", 1, 0, items); err == nil {
				benchmark.Fatal("超过上限的批次必须拒绝")
			}
		}
	})
	if measurement.AllocedBytesPerOp() > 4096 {
		t.Fatalf("拒绝批次不能先分配整批结果：每次分配%d字节", measurement.AllocedBytesPerOp())
	}
}
