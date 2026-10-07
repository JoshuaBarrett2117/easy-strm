package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOldSyncSkipsDeletedFileButFreshSyncCanRestore(t *testing.T) {
	c := NewShareOperationCoordinator()
	old := c.batchContext(context.Background(), "old-sync")
	deleted := domain.ShareMedia{ID: 11, ShareID: 1, FileName: "folder/a.mkv", RemoteFileID: "remote-a"}
	c.removedFile(deleted)
	files := []domain.ShareMedia{deleted, {ShareID: 1, FileName: "b.mkv", RemoteFileID: "remote-b"}, {ShareID: 1, FileName: "renamed-a.mkv", RemoteFileID: "remote-a"}}
	if got := c.filterSyncFiles(old, 1, files); len(got) != 1 || got[0].FileName != "b.mkv" {
		t.Fatal(got)
	}
	fresh := c.batchContext(context.Background(), "fresh-sync")
	if got := c.filterSyncFiles(fresh, 1, files); len(got) != 3 {
		t.Fatal(got)
	}
	if got := c.filterSyncFiles(old, 2, files); len(got) != 3 {
		t.Fatal("其他分享受影响", got)
	}
}

func TestFileDeletionDoesNotWaitForWholeSyncOrOtherFiles(t *testing.T) {
	c := NewShareOperationCoordinator()
	scan := acquireFixture(t, c, shareResource{key: shareKey(1)}, shareResource{key: "sync:1", exclusive: true})
	defer scan()
	other := acquireFixture(t, c, shareResource{key: shareKey(1)}, shareResource{key: fileKey(12), exclusive: true})
	defer other()
	active := acquireFixture(t, c, shareResource{key: shareKey(1)}, shareResource{key: fileKey(11), exclusive: true})
	ticket := c.reserveCleanup("delete-file", operationResources(domain.ShareOperation{Kind: "share_media_delete", ShareIDs: []int{1}, FileID: 11}))
	c.mu.Lock()
	available := c.available(ticket)
	c.mu.Unlock()
	if available {
		t.Fatal("活动文件结束前执行了删除")
	}
	active()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := c.wait(ctx, ticket, nil)
	if err != nil {
		t.Fatal("删除不应等待扫描或其他文件", err)
	}
	release()
	commit := acquireFixture(t, c, shareResource{key: fileKey(11), exclusive: true})
	commit()
}

func TestFileCleanupBlockersExcludeSharedLifecycleReaders(t *testing.T) {
	c := NewShareOperationCoordinator()
	ctxA := context.WithValue(context.Background(), shareOwnerContext{}, "identify-A")
	ctxB := context.WithValue(context.Background(), shareOwnerContext{}, "identify-B")
	a, _ := c.acquire(ctxA, nil, shareResource{key: shareKey(1)}, shareResource{key: fileKey(11), exclusive: true})
	defer a()
	b, _ := c.acquire(ctxB, nil, shareResource{key: shareKey(1)}, shareResource{key: fileKey(12), exclusive: true})
	defer b()
	ticket := c.reserveCleanup("delete-A", operationResources(domain.ShareOperation{Kind: "share_media_delete", ShareIDs: []int{1}, FileID: 11}))
	blockers := c.blockers(ticket)
	if len(blockers) != 1 || blockers[0] != "identify-A" {
		t.Fatal("共享占用被误报为冲突", blockers)
	}
}

func TestDifferentFileCleanupsCanRunConcurrently(t *testing.T) {
	c := NewShareOperationCoordinator()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	a := c.reserveCleanup("delete-A", operationResources(domain.ShareOperation{Kind: "share_media_delete", ShareIDs: []int{1}, FileID: 11}))
	releaseA, err := c.wait(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA()
	b := c.reserveCleanup("delete-B", operationResources(domain.ShareOperation{Kind: "share_media_delete", ShareIDs: []int{1}, FileID: 12}))
	releaseB, err := c.wait(ctx, b, nil)
	if err != nil {
		t.Fatal("同分享不同文件被锁住", err)
	}
	releaseB()
}

func acquireFixture(t *testing.T, c *ShareOperationCoordinator, resources ...shareResource) func() {
	t.Helper()
	r, e := c.acquire(context.Background(), nil, resources...)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestShareResourcesIndependentAndDestructiveFIFO(t *testing.T) {
	c := NewShareOperationCoordinator()
	file1 := acquireFixture(t, c, shareResource{key: shareKey(1)}, shareResource{key: fileKey(11), exclusive: true})
	file2 := acquireFixture(t, c, shareResource{key: shareKey(1)}, shareResource{key: fileKey(12), exclusive: true})
	other := acquireFixture(t, c, shareResource{key: shareKey(2), exclusive: true})
	other()
	deleteTicket := c.reserveCleanup("delete", []shareResource{{key: shareKey(1), exclusive: true}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deleted := make(chan func(), 1)
	go func() {
		r, e := c.wait(ctx, deleteTicket, nil)
		if e == nil {
			deleted <- r
		}
	}()
	newWork := c.reserve("new-work", []shareResource{{key: shareKey(1)}, {key: fileKey(13), exclusive: true}})
	if r, e := c.try(shareResource{key: shareKey(1)}); e == nil {
		r()
		t.Fatal("清理等待时新工作抢占")
	}
	file1()
	select {
	case <-deleted:
		t.Fatal("另一活动文件尚未结束便删除")
	default:
	}
	file2()
	var release func()
	select {
	case release = <-deleted:
	case <-ctx.Done():
		t.Fatal("删除没有在目标结束后执行")
	}
	if r, e := c.try(shareResource{key: shareKey(2), exclusive: true}); e != nil {
		t.Fatal("无关目标被排队限制")
	} else {
		r()
	}
	release()
	r, e := c.wait(ctx, newWork, nil)
	if e != nil {
		t.Fatal(e)
	}
	r()
}

func TestShareResourcesAtomicMultiTargetAndCancellation(t *testing.T) {
	c := NewShareOperationCoordinator()
	active := acquireFixture(t, c, shareResource{key: shareKey(2)})
	ticket := c.reserve("batch-clear", []shareResource{{key: shareKey(1), exclusive: true}, {key: shareKey(2), exclusive: true}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.wait(ctx, ticket, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	r := acquireFixture(t, c, shareResource{key: shareKey(1), exclusive: true})
	r()
	active()
	if len(c.waiters) != 0 || len(c.holders) != 0 {
		t.Fatal("取消后残留资源")
	}
}

func TestShareResourcesOldBatchInvalidated(t *testing.T) {
	c := NewShareOperationCoordinator()
	old := c.batchContext(context.Background(), "old")
	c.invalidate([]int{1})
	if _, e := c.acquire(old, []int{1}, shareResource{key: shareKey(1)}); !errors.Is(e, ErrShareUnitSkipped) {
		t.Fatal("旧任务没有跳过清空目标", e)
	}
	r, e := c.acquire(old, []int{2}, shareResource{key: shareKey(2)})
	if e != nil {
		t.Fatal(e)
	}
	r()
	fresh := c.batchContext(context.Background(), "fresh")
	r, e = c.acquire(fresh, []int{1}, shareResource{key: shareKey(1)})
	if e != nil {
		t.Fatal(e)
	}
	r()
}

func TestShareResourcesBudgetAcrossTasks(t *testing.T) {
	c := NewShareOperationCoordinator()
	held := acquireFixture(t, c, shareResource{key: shareKey(1)}, shareResource{key: "budget:identify", limit: 1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiter := c.reserve("second-task", []shareResource{{key: shareKey(2)}, {key: "budget:identify", limit: 1}})
	done := make(chan func(), 1)
	go func() {
		r, e := c.wait(ctx, waiter, nil)
		if e == nil {
			done <- r
		}
	}()
	select {
	case <-done:
		t.Fatal("超出跨任务并发预算")
	default:
	}
	// 等待预算不能先占用其目标，否则该分享无法删除。
	cleanup := c.reserveCleanup("cleanup", []shareResource{{key: shareKey(2), exclusive: true}})
	r, e := c.wait(ctx, cleanup, nil)
	if e != nil {
		t.Fatal(e)
	}
	r()
	held()
	select {
	case r = <-done:
		r()
	case <-time.After(time.Second):
		t.Fatal("预算未释放")
	}
}

func TestShareRequestRegistrationDeduplicatesWithoutBlockingOthers(t *testing.T) {
	c := NewShareOperationCoordinator()
	started := make(chan struct{})
	unblock := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, e := c.registerRequest(context.Background(), "same", "first", func() error { close(started); <-unblock; return nil })
		if e != nil {
			t.Error(e)
		}
	}()
	<-started
	id, created, e := c.registerRequest(context.Background(), "other", "other-id", func() error { return nil })
	if e != nil || !created || id != "other-id" {
		t.Fatal(id, created, e)
	}
	close(unblock)
	wg.Wait()
	id, created, e = c.registerRequest(context.Background(), "same", "duplicate", func() error { t.Fatal("重复创建"); return nil })
	if e != nil || created || id != "first" {
		t.Fatal(id, created, e)
	}
}
