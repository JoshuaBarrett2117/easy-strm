package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
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
