package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBehaviorRemote 按 offset 脚本化返回行为分页，并支持路径解析与目录枚举。
type fakeBehaviorRemote struct {
	events   func(offset int) (domain.BehaviorPage, error)
	resolve  map[string]string
	listings map[string]BehaviorListing
}

func (f *fakeBehaviorRemote) Events(_ context.Context, _ string, offset int) (domain.BehaviorPage, error) {
	if f.events != nil {
		return f.events(offset)
	}
	return domain.BehaviorPage{State: true}, nil
}

func (f *fakeBehaviorRemote) Resolve(_ context.Context, _, cloudPath string) (string, error) {
	if cid, ok := f.resolve[cloudPath]; ok {
		return cid, nil
	}
	return "", fmt.Errorf("未配置路径 %q 的CID", cloudPath)
}

func (f *fakeBehaviorRemote) List(_ context.Context, _, cid string, _ int) (BehaviorListing, error) {
	if l, ok := f.listings[cid]; ok {
		return l, nil
	}
	return BehaviorListing{}, fmt.Errorf("未配置目录 %q 的枚举", cid)
}

// fakeBehaviorSession 提供账号锁内的检查点与身份索引，全部内存实现。
type fakeBehaviorSession struct {
	cursor  int64
	cookie  string
	cloudID int
	roots   []domain.BehaviorRoot
	index   map[string]*domain.BehaviorIndex
	pending []domain.BehaviorIndex
}

func (s *fakeBehaviorSession) Close() error { return nil }
func (s *fakeBehaviorSession) Checkpoint(_ context.Context) (int64, error) {
	return s.cursor, nil
}
func (s *fakeBehaviorSession) Advance(_ context.Context, cursor int64) error {
	s.cursor = cursor
	return nil
}
func (s *fakeBehaviorSession) Configuration(_ context.Context) (string, int, []domain.BehaviorRoot, error) {
	return s.cookie, s.cloudID, s.roots, nil
}
func (s *fakeBehaviorSession) Get(_ context.Context, id string) (*domain.BehaviorIndex, error) {
	return s.index[id], nil
}
func (s *fakeBehaviorSession) Tree(_ context.Context, _ string) ([]domain.BehaviorIndex, error) {
	return nil, nil
}
func (s *fakeBehaviorSession) Pending(_ context.Context) ([]domain.BehaviorIndex, error) {
	return s.pending, nil
}
func (s *fakeBehaviorSession) Put(_ context.Context, item domain.BehaviorIndex) error {
	cp := item
	s.index[item.FileID] = &cp
	return nil
}
func (s *fakeBehaviorSession) Delete(_ context.Context, id string) error {
	delete(s.index, id)
	return nil
}
func (s *fakeBehaviorSession) LockPaths(_ context.Context, _ []string) (func(), error) {
	return func() {}, nil
}

// fakeBehaviorRepository 返回同一内存会话，供 Run 端到端测试。
type fakeBehaviorRepository struct{ session *fakeBehaviorSession }

func (r *fakeBehaviorRepository) Lock(_ context.Context, _ int) (dao.BehaviorSession, error) {
	return r.session, nil
}

// fakeBehaviorRefresher 记录每次刷新收到的去重路径。
type fakeBehaviorRefresher struct{ batches [][]string }

func (f *fakeBehaviorRefresher) RefreshBehaviorPaths(_ context.Context, paths []string) error {
	f.batches = append(f.batches, append([]string(nil), paths...))
	return nil
}

// TestSelectBehaviorBatchCursorTermination 遇到 <= checkpoint 立即停止翻页，且结果不含旧事件。
func TestSelectBehaviorBatchCursorTermination(t *testing.T) {
	remote := &fakeBehaviorRemote{events: func(offset int) (domain.BehaviorPage, error) {
		if offset == 0 {
			return domain.BehaviorPage{State: true, NextPage: true, List: []domain.BehaviorEvent{
				{ID: 100, Type: 2, Name: "a.mp4"},
				{ID: 90, Type: 2, Name: "b.mp4"},
				{ID: 80, Type: 2, Name: "c.mp4"},
			}}, nil
		}
		return domain.BehaviorPage{State: true, NextPage: false, List: []domain.BehaviorEvent{
			{ID: 70, Type: 2, Name: "d.mp4"},
			{ID: 60, Type: 2, Name: "e.mp4"},
			{ID: 55, Type: 2, Name: "f.mp4"},
			{ID: 40, Type: 2, Name: "g.mp4"},
			{ID: 30, Type: 2, Name: "h.mp4"},
		}}, nil
	}}
	var stats BehaviorStats
	got, err := selectBehaviorBatch(context.Background(), remote, "cookie", 50, 10, &stats)
	if err != nil {
		t.Fatalf("selectBehaviorBatch 返回错误: %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("选中事件数 = %d，期望 6（55/60/70/80/90/100）", len(got))
	}
	for _, e := range got {
		if int64(e.ID) <= 50 {
			t.Errorf("结果包含 <= checkpoint 的事件 ID=%d", int64(e.ID))
		}
	}
	if stats.Fetched != 8 {
		t.Errorf("Fetched = %d，期望 8（两页累计拉取条数）", stats.Fetched)
	}
}

// TestSelectBehaviorBatchLastPage 最后一页 next_page=false 时正常结束，不依赖 checkpoint。
func TestSelectBehaviorBatchLastPage(t *testing.T) {
	remote := &fakeBehaviorRemote{events: func(offset int) (domain.BehaviorPage, error) {
		return domain.BehaviorPage{State: true, NextPage: false, List: []domain.BehaviorEvent{
			{ID: 30, Type: 2, Name: "a.mp4"},
			{ID: 20, Type: 2, Name: "b.mp4"},
			{ID: 10, Type: 2, Name: "c.mp4"},
		}}, nil
	}}
	var stats BehaviorStats
	got, err := selectBehaviorBatch(context.Background(), remote, "cookie", 0, 10, &stats)
	if err != nil {
		t.Fatalf("selectBehaviorBatch 返回错误: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("选中事件数 = %d，期望 3", len(got))
	}
	if int64(got[0].ID) != 10 || int64(got[1].ID) != 20 || int64(got[2].ID) != 30 {
		t.Fatalf("升序结果错误: %v", got)
	}
	if stats.Fetched != 3 {
		t.Errorf("Fetched = %d，期望 3", stats.Fetched)
	}
}

// TestBehaviorGeneratesFilter 删除/改名/移动事件不走生成分支，仅P1生成类型命中。
func TestBehaviorGeneratesFilter(t *testing.T) {
	cases := []struct {
		name string
		typ  int
		file string
		want bool
	}{
		{"删除事件不生成", 22, "x", false},
		{"目录改名不生成", 20, "x", false},
		{"文件改名不生成", 24, "x", false},
		{"移动事件不生成", 6, "x", false},
		{"上传视频生成", 2, "a.mp4", true},
		{"上传非视频不生成", 2, "a.txt", false},
		{"复制视频生成", 23, "a.mkv", true},
		{"复制非视频不生成", 23, "a.jpg", false},
		{"接收分享生成", 14, "", true},
		{"新建目录生成", 17, "", true},
		{"复制目录生成", 18, "", true},
		{"未知类型不生成", 99, "a.mp4", false},
	}
	for _, c := range cases {
		got := BehaviorGenerates(domain.BehaviorEvent{Type: c.typ, Name: c.file})
		if got != c.want {
			t.Errorf("%s: BehaviorGenerates(type=%d, name=%q) = %v, 期望 %v", c.name, c.typ, c.file, got, c.want)
		}
	}
}

// TestBehaviorVideoExtensions 覆盖需求声明的常见视频扩展名。
func TestBehaviorVideoExtensions(t *testing.T) {
	videos := []string{"a.mp4", "b.mkv", "c.ts", "d.avi", "e.mov", "f.wmv", "g.flv", "h.m2ts", "i.mts", "j.MP4"}
	for _, name := range videos {
		if !behaviorVideo(name) {
			t.Errorf("behaviorVideo(%q) 应为 true", name)
		}
	}
	nonVideos := []string{"a.txt", "b.jpg", "c.srt", "d.nfo", "e", "f.strm"}
	for _, name := range nonVideos {
		if behaviorVideo(name) {
			t.Errorf("behaviorVideo(%q) 应为 false", name)
		}
	}
}

// TestBehaviorWithinSubtree 校验目录子树归属判断，杜绝前缀误判与越界。
func TestBehaviorWithinSubtree(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "a", "b")
	within := filepath.Join(root, "c", "d.mp4")
	sibling := filepath.Join(string(filepath.Separator), "a", "bc", "d.mp4")
	parent := filepath.Join(string(filepath.Separator), "a")
	if !behaviorWithin(root, within) {
		t.Errorf("behaviorWithin(%q, %q) 应为 true", root, within)
	}
	if behaviorWithin(root, sibling) {
		t.Errorf("behaviorWithin(%q, %q) 应为 false（前缀误判）", root, sibling)
	}
	if behaviorWithin(root, parent) {
		t.Errorf("behaviorWithin(%q, %q) 应为 false（父目录越界）", root, parent)
	}
}

// TestBehaviorMovedContent 缺少SHA1时用旧内容原扩展名重写移动后的云盘路径。
func TestBehaviorMovedContent(t *testing.T) {
	old := "http://host/direct-link?path=%2Fmedia%2Fold%2FMovie.mkv&cloud115_id=1\n"
	got := behaviorMovedContent(old, "/media/new", "sub/Movie.strm")
	if !strings.Contains(got, "%2Fmedia%2Fnew%2Fsub%2FMovie.mkv") {
		t.Errorf("移动后内容未包含新路径: %q", got)
	}
	if !strings.Contains(got, "cloud115_id=1") {
		t.Errorf("移动后内容丢失账号参数: %q", got)
	}
	if strings.Contains(got, ".strm") {
		t.Errorf("移动后内容不应保留 .strm 后缀: %q", got)
	}
}

// TestBehaviorWriteIdempotent 同一文件重复生成只写一次，第二次计入跳过。
func TestBehaviorWriteIdempotent(t *testing.T) {
	rootPath := t.TempDir()
	store := &fakeBehaviorSession{index: map[string]*domain.BehaviorIndex{}}
	run := &behaviorRun{
		service: &BehaviorSyncService{serverURL: func() string { return "http://localhost:8082" }},
		store:   store,
		account: 1,
		stats:   &BehaviorStats{},
		changed: map[string]bool{},
		roots:   []behaviorScope{{BehaviorRoot: domain.BehaviorRoot{ID: 1, CloudPath: "/media", LocalPath: rootPath}, cid: "100"}},
	}
	node := BehaviorNode{ID: "2001", ParentID: "100", Name: "Movie.mkv", Size: 10, SHA1: "abc"}
	ctx := context.Background()
	if err := run.write(ctx, node, run.roots[0], "Movie.mkv"); err != nil {
		t.Fatalf("首次生成失败: %v", err)
	}
	if run.stats.Generated != 1 {
		t.Fatalf("首次生成数 = %d，期望 1", run.stats.Generated)
	}
	if _, err := os.Stat(filepath.Join(rootPath, "Movie.strm")); err != nil {
		t.Fatalf("STRM 文件未生成: %v", err)
	}
	if err := run.write(ctx, node, run.roots[0], "Movie.mkv"); err != nil {
		t.Fatalf("重复生成失败: %v", err)
	}
	if run.stats.Generated != 1 {
		t.Errorf("重复执行后生成数 = %d，期望仍为 1（幂等）", run.stats.Generated)
	}
	if run.stats.Skipped != 1 {
		t.Errorf("重复执行跳过数 = %d，期望 1", run.stats.Skipped)
	}
}

// TestBehaviorSyncRunAdvancesCheckpointAndIsIdempotent 端到端：首次前进检查点并生成，
// 再次运行无新事件、无新增文件、检查点不变。
func TestBehaviorSyncRunAdvancesCheckpointAndIsIdempotent(t *testing.T) {
	rootPath := t.TempDir()
	session := &fakeBehaviorSession{
		cookie:  "testcookie",
		cloudID: 1,
		roots:   []domain.BehaviorRoot{{ID: 1, CloudPath: "/media", LocalPath: rootPath}},
		index:   map[string]*domain.BehaviorIndex{},
	}
	repo := &fakeBehaviorRepository{session: session}
	remote := &fakeBehaviorRemote{
		resolve: map[string]string{"/media": "100"},
		listings: map[string]BehaviorListing{
			"100": {Ancestors: []BehaviorNode{{ID: "0", ParentID: "0"}, {ID: "100", ParentID: "0", Name: "media"}}},
		},
	}
	remote.events = func(offset int) (domain.BehaviorPage, error) {
		return domain.BehaviorPage{State: true, NextPage: false, List: []domain.BehaviorEvent{
			{ID: 100, Type: 2, FileID: "2001", ParentID: "100", Name: "Movie.mkv", Size: 10, SHA1: "abc"},
		}}, nil
	}
	refresher := &fakeBehaviorRefresher{}
	svc := NewBehaviorSyncService(repo, remote, refresher, func() string { return "http://localhost:8082" })

	stats1, err := svc.Run(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("首次运行失败: %v", err)
	}
	if stats1.Generated != 1 {
		t.Fatalf("首次运行生成数 = %d，期望 1", stats1.Generated)
	}
	if stats1.After != 100 || session.cursor != 100 {
		t.Fatalf("检查点未前进: after=%d cursor=%d", stats1.After, session.cursor)
	}
	if _, err := os.Stat(filepath.Join(rootPath, "Movie.strm")); err != nil {
		t.Fatalf("STRM 未生成: %v", err)
	}
	if len(refresher.batches) != 1 {
		t.Fatalf("应触发一次 Emby 刷新，实际 %d 次", len(refresher.batches))
	}

	// 第二次运行：无新事件，检查点不回退，无新增文件。
	remote.events = func(offset int) (domain.BehaviorPage, error) {
		return domain.BehaviorPage{State: true, NextPage: false}, nil
	}
	stats2, err := svc.Run(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("重复运行失败: %v", err)
	}
	if stats2.Generated != 0 {
		t.Fatalf("重复运行生成数 = %d，期望 0", stats2.Generated)
	}
	if stats2.After != 100 {
		t.Fatalf("重复运行检查点 = %d，期望保持 100", stats2.After)
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		t.Fatalf("读取输出目录失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("重复运行产生额外文件，目录条目数 = %d，期望 1", len(entries))
	}
}

// TestSelectBehaviorBatchToleratesNonMonotonicIDs 覆盖线上真实情况：
// 115 的事件 ID 并非严格降序，不能据此判定数据损坏。
func TestSelectBehaviorBatchToleratesNonMonotonicIDs(t *testing.T) {
	remote := &fakeBehaviorRemote{events: func(offset int) (domain.BehaviorPage, error) {
		return domain.BehaviorPage{State: true, NextPage: false, List: []domain.BehaviorEvent{
			{ID: 100, Type: 2, Name: "a.mp4"},
			{ID: 95, Type: 2, Name: "b.mp4"},
			{ID: 97, Type: 2, Name: "c.mp4"},
		}}, nil
	}}
	var stats BehaviorStats
	got, err := selectBehaviorBatch(context.Background(), remote, "cookie", 0, 10, &stats)
	if err != nil {
		t.Fatalf("ID 非严格降序不应报错: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("选中事件数 = %d，期望 3", len(got))
	}
}

// TestSelectBehaviorBatchStopsWhenPageHasNoNewEvents 覆盖整页都 <= checkpoint 时停止翻页。
func TestSelectBehaviorBatchStopsWhenPageHasNoNewEvents(t *testing.T) {
	fetched := 0
	remote := &fakeBehaviorRemote{events: func(offset int) (domain.BehaviorPage, error) {
		fetched++
		return domain.BehaviorPage{State: true, NextPage: true, List: []domain.BehaviorEvent{
			{ID: 40, Type: 2, Name: "old1.mp4"},
			{ID: 30, Type: 2, Name: "old2.mp4"},
		}}, nil
	}}
	var stats BehaviorStats
	got, err := selectBehaviorBatch(context.Background(), remote, "cookie", 50, 10, &stats)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("不应选中事件，实际 %d", len(got))
	}
	if fetched != 1 {
		t.Fatalf("应只拉取一页即停止，实际 %d 页", fetched)
	}
}
