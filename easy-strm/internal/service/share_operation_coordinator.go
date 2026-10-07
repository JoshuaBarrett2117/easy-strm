package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ErrShareUnitSkipped 表示旧批次的目标已被清空或删除，应跳过而不是重新创建。
var ErrShareUnitSkipped = errors.New("目标已被清空或删除，跳过旧任务单元")

type shareResource struct {
	key       string
	exclusive bool
	limit     int
}
type shareTicket struct {
	owner     string
	resources []shareResource
	cleanup   bool
}
type shareHolder struct {
	readers map[string]int
	writer  string
}
type shareRequest struct {
	id    string
	ready chan struct{}
	err   error
}
type shareEpochContext struct{}
type shareOwnerContext struct{}
type shareSubmissionContext struct{}

// ShareOperationCoordinator 按实际资源原子分配占用；等待队列只阻塞相交资源。
type ShareOperationCoordinator struct {
	mu           sync.Mutex
	changed      chan struct{}
	holders      map[string]*shareHolder
	waiters      []*shareTicket
	epochs       map[int]uint64
	requests     map[string]*shareRequest
	revision     uint64
	removedFiles map[int]map[string]uint64
}

// NewShareOperationCoordinator 创建分享业务的资源协调器，不持有数据库或网络依赖。
func NewShareOperationCoordinator() *ShareOperationCoordinator {
	return &ShareOperationCoordinator{changed: make(chan struct{}), holders: map[string]*shareHolder{}, epochs: map[int]uint64{}, requests: map[string]*shareRequest{}, removedFiles: map[int]map[string]uint64{}}
}

func shareKey(id int) string { return fmt.Sprintf("share:%d", id) }
func fileKey(id int) string  { return fmt.Sprintf("file:%d", id) }
func sortedShareIDs(ids []int) []int {
	out := append([]int(nil), ids...)
	sort.Ints(out)
	unique := out[:0]
	for _, id := range out {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	return unique
}
func ownerOf(ctx context.Context) string     { v, _ := ctx.Value(shareOwnerContext{}).(string); return v }
func (c *ShareOperationCoordinator) signal() { close(c.changed); c.changed = make(chan struct{}) }
func normalizeResources(resources []shareResource) []shareResource {
	byKey := map[string]shareResource{}
	for _, r := range resources {
		old := byKey[r.key]
		r.exclusive = r.exclusive || old.exclusive
		if r.limit == 0 {
			r.limit = old.limit
		}
		byKey[r.key] = r
	}
	out := make([]shareResource, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}
func resourcesConflict(a, b []shareResource) bool {
	for _, x := range a {
		for _, y := range b {
			if x.key == y.key && (x.exclusive || y.exclusive) {
				return true
			}
		}
	}
	return false
}
func (c *ShareOperationCoordinator) reserve(owner string, resources []shareResource) *shareTicket {
	return c.reserveKind(owner, resources, false)
}
func (c *ShareOperationCoordinator) reserveCleanup(owner string, resources []shareResource) *shareTicket {
	return c.reserveKind(owner, resources, true)
}
func (c *ShareOperationCoordinator) reserveKind(owner string, resources []shareResource, cleanup bool) *shareTicket {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &shareTicket{owner: owner, resources: normalizeResources(resources), cleanup: cleanup}
	c.waiters = append(c.waiters, t)
	c.signal()
	return t
}
func (c *ShareOperationCoordinator) remove(t *shareTicket) {
	for i, v := range c.waiters {
		if v == t {
			c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
			return
		}
	}
}
func (c *ShareOperationCoordinator) available(t *shareTicket) bool {
	if !t.cleanup {
		for _, other := range c.waiters {
			if other != t && other.cleanup && resourcesConflict(other.resources, t.resources) {
				return false
			}
		}
	}
	for _, earlier := range c.waiters {
		if earlier == t {
			break
		}
		if (!t.cleanup || earlier.cleanup) && resourcesConflict(earlier.resources, t.resources) {
			return false
		}
	}
	for _, r := range t.resources {
		h := c.holders[r.key]
		if h == nil {
			continue
		}
		if h.writer != "" {
			return false
		}
		count := 0
		for _, n := range h.readers {
			count += n
		}
		if r.exclusive && count > 0 || r.limit > 0 && count >= r.limit {
			return false
		}
	}
	return true
}
func (c *ShareOperationCoordinator) take(t *shareTicket) func() {
	c.remove(t)
	for _, r := range t.resources {
		h := c.holders[r.key]
		if h == nil {
			h = &shareHolder{readers: map[string]int{}}
			c.holders[r.key] = h
		}
		if r.exclusive {
			h.writer = t.owner
		} else {
			h.readers[t.owner]++
		}
	}
	c.signal()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			for _, r := range t.resources {
				h := c.holders[r.key]
				if r.exclusive {
					h.writer = ""
				} else {
					h.readers[t.owner]--
					if h.readers[t.owner] == 0 {
						delete(h.readers, t.owner)
					}
				}
				if h.writer == "" && len(h.readers) == 0 {
					delete(c.holders, r.key)
				}
			}
			c.signal()
		})
	}
}
func (c *ShareOperationCoordinator) stale(ctx context.Context, shares []int) bool {
	snapshot, ok := ctx.Value(shareEpochContext{}).(map[int]uint64)
	if !ok {
		return false
	}
	for _, id := range shares {
		if snapshot[id] != c.epochs[id] {
			return true
		}
	}
	return false
}
func (c *ShareOperationCoordinator) wait(ctx context.Context, t *shareTicket, shares []int) (func(), error) {
	return c.waitReported(ctx, t, shares, nil)
}
func (c *ShareOperationCoordinator) waitReported(ctx context.Context, t *shareTicket, shares []int, report func()) (func(), error) {
	for {
		c.mu.Lock()
		if err := ctx.Err(); err != nil {
			c.remove(t)
			c.signal()
			c.mu.Unlock()
			return nil, err
		}
		if c.stale(ctx, shares) {
			c.remove(t)
			c.signal()
			c.mu.Unlock()
			return nil, ErrShareUnitSkipped
		}
		if c.available(t) {
			release := c.take(t)
			c.mu.Unlock()
			return release, nil
		}
		changed := c.changed
		c.mu.Unlock()
		if report != nil {
			report()
		}
		select {
		case <-ctx.Done():
		case <-changed:
		}
	}
}
func (c *ShareOperationCoordinator) acquire(ctx context.Context, shares []int, resources ...shareResource) (func(), error) {
	owner := ownerOf(ctx)
	if owner == "" {
		owner = fmt.Sprintf("request:%p", &resources)
	}
	return c.wait(ctx, c.reserve(owner, resources), shares)
}
func (c *ShareOperationCoordinator) try(resources ...shareResource) (func(), error) {
	t := c.reserveCleanup(fmt.Sprintf("request:%p", &resources), resources)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.available(t) {
		c.remove(t)
		c.signal()
		return nil, fmt.Errorf("目标资源正在处理或等待清理")
	}
	return c.take(t), nil
}
func (c *ShareOperationCoordinator) batchContext(ctx context.Context, owner string) context.Context {
	c.mu.Lock()
	snapshot := make(map[int]uint64, len(c.epochs))
	for id, v := range c.epochs {
		snapshot[id] = v
	}
	revision := c.revision
	c.mu.Unlock()
	return context.WithValue(context.WithValue(context.WithValue(ctx, shareEpochContext{}, snapshot), shareOwnerContext{}, owner), shareSubmissionContext{}, revision)
}

func (c *ShareOperationCoordinator) removedFile(file domain.ShareMedia) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revision++
	if c.removedFiles[file.ShareID] == nil {
		c.removedFiles[file.ShareID] = map[string]uint64{}
	}
	c.removedFiles[file.ShareID]["path:"+file.FileName] = c.revision
	if file.RemoteFileID != "" {
		c.removedFiles[file.ShareID]["remote:"+file.RemoteFileID] = c.revision
	}
	c.signal()
}
func (c *ShareOperationCoordinator) filterSyncFiles(ctx context.Context, id int, files []domain.ShareMedia) []domain.ShareMedia {
	revision, ok := ctx.Value(shareSubmissionContext{}).(uint64)
	if !ok {
		return files
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := c.removedFiles[id]
	out := make([]domain.ShareMedia, 0, len(files))
	for _, file := range files {
		if removed["path:"+file.FileName] > revision || file.RemoteFileID != "" && removed["remote:"+file.RemoteFileID] > revision {
			continue
		}
		out = append(out, file)
	}
	return out
}
func (c *ShareOperationCoordinator) invalidate(ids []int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		c.epochs[id]++
		delete(c.removedFiles, id)
	}
	c.signal()
}
func (c *ShareOperationCoordinator) blockers(t *shareTicket) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	owners := map[string]bool{}
	for _, r := range t.resources {
		if h := c.holders[r.key]; h != nil {
			if h.writer != "" {
				owners[h.writer] = true
			}
			if r.exclusive || r.limit > 0 {
				for owner := range h.readers {
					owners[owner] = true
				}
			}
		}
	}
	for _, earlier := range c.waiters {
		if earlier == t {
			break
		}
		if (!t.cleanup || earlier.cleanup) && resourcesConflict(earlier.resources, t.resources) {
			owners[earlier.owner] = true
		}
	}
	out := []string{}
	for owner := range owners {
		if !strings.HasPrefix(owner, "request:") {
			out = append(out, owner)
		}
	}
	sort.Strings(out)
	return out
}
func (c *ShareOperationCoordinator) pendingShare(id int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.waiters {
		for _, r := range t.resources {
			if r.key == shareKey(id) && r.exclusive {
				return true
			}
		}
	}
	h := c.holders[shareKey(id)]
	return h != nil && h.writer != ""
}

// registerRequest 只锁登记表；创建任务在锁外执行，相同请求等待创建结果后复用ID。
func (c *ShareOperationCoordinator) registerRequest(ctx context.Context, key, id string, create func() error) (string, bool, error) {
	c.mu.Lock()
	if old := c.requests[key]; old != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-old.ready:
			return old.id, false, old.err
		}
	}
	r := &shareRequest{id: id, ready: make(chan struct{})}
	c.requests[key] = r
	c.mu.Unlock()
	err := create()
	c.mu.Lock()
	r.err = err
	if err != nil {
		delete(c.requests, key)
	}
	close(r.ready)
	c.mu.Unlock()
	return id, err == nil, err
}
func (c *ShareOperationCoordinator) finishRequest(key, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r := c.requests[key]; r != nil && r.id == id {
		delete(c.requests, key)
	}
}
