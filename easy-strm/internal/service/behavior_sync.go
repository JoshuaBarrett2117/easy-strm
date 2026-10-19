package service

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BehaviorStats 始终写入cron执行摘要，失败时也保留计数及前后游标。
type BehaviorStats struct {
	Fetched   int   `json:"fetched"`
	Matched   int   `json:"matched"`
	Generated int   `json:"generated"`
	Skipped   int   `json:"skipped"`
	Failed    int   `json:"failed"`
	Before    int64 `json:"checkpoint_before"`
	After     int64 `json:"checkpoint_after"`
}

// BehaviorRefresher 批量接收去重路径，不按每个事件刷新全库。
type BehaviorRefresher interface {
	RefreshBehaviorPaths(context.Context, []string) error
}

// BehaviorSyncService 编排行为增量、目录边界和身份索引，不执行全量校准。
type BehaviorSyncService struct {
	store     dao.BehaviorRepository
	remote    BehaviorRemote
	refresh   BehaviorRefresher
	serverURL func() string
}

// NewBehaviorSyncService 注入只读115客户端、持久仓储和已有Emby服务。
func NewBehaviorSyncService(store dao.BehaviorRepository, remote BehaviorRemote, refresh BehaviorRefresher, serverURL func() string) *BehaviorSyncService {
	return &BehaviorSyncService{store: store, remote: remote, refresh: refresh, serverURL: serverURL}
}

// BehaviorGenerates 仅分类P1事件，P2/P3由独立分派处理。
func BehaviorGenerates(event domain.BehaviorEvent) bool {
	switch event.Type {
	case 14, 17, 18:
		return true
	case 2, 23:
		return behaviorVideo(event.Name)
	default:
		return false
	}
}

func behaviorVideo(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp4", ".mkv", ".ts", ".avi", ".mov", ".wmv", ".flv", ".m2ts", ".mts", ".m2t", ".mpg", ".mpeg", ".m4v", ".webm", ".vob", ".iso", ".rm", ".rmvb", ".asf", ".3gp", ".divx":
		return true
	}
	return false
}

func behaviorLog(ctx context.Context, reason string, event domain.BehaviorEvent) {
	logger.WithContext(ctx, "behavior_sync").Log(logger.WARN, reason, logger.Fields{"event_id": int64(event.ID), "file_id": event.FileID, "type": event.Type}, nil)
}

// selectBehaviorBatch 扫描到检查点或末页，环形缓冲区只保留最老N条。
// 不在降序前N条处截断；因此积压大于N也不会丢失或饿死旧事件。
func selectBehaviorBatch(ctx context.Context, remote BehaviorRemote, cookie string, cursor int64, limit int, stats *BehaviorStats) ([]domain.BehaviorEvent, error) {
	// 115 的事件 ID 并非严格随页降序（实测 2000 条中有 25 处回升），
	// 因此不能按「ID 必须严格递减」判定数据损坏，也不能用「低于历史最低值就丢弃」
	// 的环形缓冲，否则会漏掉 ID 回升的真实事件。这里按 ID 去重收集，最后统一排序取最旧的一批。
	collected := make(map[int64]domain.BehaviorEvent, limit*2)
	for offset := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := remote.Events(ctx, cookie, offset)
		if err != nil {
			return nil, err
		}
		if !page.State {
			return nil, fmt.Errorf("115行为响应state=false")
		}
		stats.Fetched += len(page.List)
		hasNew := false
		for _, event := range page.List {
			id := int64(event.ID)
			if id <= 0 {
				return nil, fmt.Errorf("行为ID无效")
			}
			if id <= cursor {
				continue
			}
			hasNew = true
			if _, exists := collected[id]; exists {
				continue
			}
			collected[id] = event
		}
		if !page.NextPage {
			break
		}
		if len(page.List) == 0 {
			return nil, fmt.Errorf("行为分页为空但仍声明next_page，保留检查点")
		}
		if !hasNew {
			break
		}
		offset += len(page.List)
		if offset > limit*200 {
			break
		}
	}
	ids := make([]int64, 0, len(collected))
	for id := range collected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(first, second int) bool { return ids[first] < ids[second] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	selected := make([]domain.BehaviorEvent, 0, len(ids))
	for _, id := range ids {
		selected = append(selected, collected[id])
	}
	return selected, nil
}

type behaviorScope struct {
	domain.BehaviorRoot
	cid string
}
type behaviorRun struct {
	service *BehaviorSyncService
	store   dao.BehaviorSession
	account int
	cookie  string
	roots   []behaviorScope
	stats   *BehaviorStats
	changed map[string]bool
	work    int
}

// Run 在账号锁内按时间顺序处理整个选定批次；任何失败均不推进检查点。
// 入参为 STRM 配置 ID，事件过滤仅作用于该配置的目录子树，检查点独立于账号下其他配置。
func (service *BehaviorSyncService) Run(ctx context.Context, strmConfigID, limit int) (stats BehaviorStats, runErr error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	defer func() {
		if runErr != nil {
			stats.Failed++
		}
	}()
	if strmConfigID <= 0 || limit <= 0 || limit > 20000 {
		return stats, fmt.Errorf("配置ID或max_events无效（配置ID>0且max_events在1至20000）")
	}
	store, err := service.store.Lock(ctx, strmConfigID)
	if err != nil {
		return stats, err
	}
	defer func() { runErr = errors.Join(runErr, store.Close()) }()
	stats.Before, err = store.Checkpoint(ctx)
	stats.After = stats.Before
	if err != nil {
		return stats, err
	}
	cookie, account, configured, err := store.Configuration(ctx)
	if err != nil {
		return stats, err
	}
	if len(configured) == 0 {
		return stats, nil
	}
	if strings.TrimSpace(cookie) == "" {
		return stats, fmt.Errorf("配置账号未保存Cookie，不能使用proapi token")
	}
	batch, err := selectBehaviorBatch(ctx, service.remote, cookie, stats.Before, limit, &stats)
	if err != nil {
		return stats, err
	}
	run := &behaviorRun{service: service, store: store, account: account, cookie: cookie, stats: &stats, changed: map[string]bool{}}
	resolutionFailed := false
	for _, root := range configured {
		if err = ctx.Err(); err != nil {
			return stats, err
		}
		root.LocalPath, err = NormalizeStrmOutputPath(root.LocalPath)
		if err != nil || root.LocalPath == string(filepath.Separator) {
			return stats, fmt.Errorf("配置%d输出路径不安全", root.ID)
		}
		if !strings.HasPrefix(root.CloudPath, "/") || path.Clean(root.CloudPath) != root.CloudPath || strings.Contains(root.CloudPath, "\\") {
			return stats, fmt.Errorf("配置%d网盘路径无效", root.ID)
		}
		cid, resolveErr := service.remote.Resolve(ctx, cookie, root.CloudPath)
		if resolveErr != nil || !validBehaviorID(cid) || (cid == "0" && root.CloudPath != "/") {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			behaviorLog(ctx, fmt.Sprintf("配置%d路径解析失败，显式跳过本批次，禁止回退其他目录", root.ID), domain.BehaviorEvent{})
			resolutionFailed = true
			continue
		}
		run.roots = append(run.roots, behaviorScope{BehaviorRoot: root, cid: cid})
	}
	if resolutionFailed {
		stats.Skipped += len(batch)
	} else {
		paths := make([]string, 0, len(run.roots))
		for _, root := range run.roots {
			paths = append(paths, root.LocalPath)
		}
		release, lockErr := store.LockPaths(ctx, paths)
		if lockErr != nil {
			return stats, lockErr
		}
		defer release()
		if err = run.recoverPending(ctx); err != nil {
			return stats, err
		}
		for _, event := range batch {
			if err = ctx.Err(); err != nil {
				return stats, err
			}
			if err = run.handle(ctx, event); err != nil {
				return stats, fmt.Errorf("事件%d: %w", event.ID, err)
			}
		}
		if service.refresh != nil && len(batch) > 0 {
			for _, root := range run.roots {
				run.changed[root.LocalPath] = true
			}
			paths = paths[:0]
			for changed := range run.changed {
				paths = append(paths, changed)
			}
			sort.Strings(paths)
			if err = service.refresh.RefreshBehaviorPaths(ctx, paths); err != nil {
				return stats, err
			}
		}
	}
	cursor := stats.Before
	if len(batch) > 0 {
		cursor = int64(batch[len(batch)-1].ID)
	}
	if err = store.Advance(ctx, cursor); err != nil {
		return stats, err
	}
	stats.After = cursor
	return stats, nil
}

func (run *behaviorRun) budget(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	run.work++
	if run.work > 20000 {
		return fmt.Errorf("目录工作超过20000项，保留检查点")
	}
	return nil
}

func (run *behaviorRun) handle(ctx context.Context, event domain.BehaviorEvent) error {
	switch event.Type {
	case 22:
		run.stats.Matched++
		return run.remove(ctx, event.FileID, event)
	case 20, 24, 6:
		run.stats.Matched++
		return run.relocate(ctx, event)
	}
	if !BehaviorGenerates(event) {
		run.stats.Skipped++
		return nil
	}
	run.stats.Matched++
	node := BehaviorNode{ID: event.FileID, ParentID: event.ParentID, Name: event.Name, Size: event.Size, SHA1: event.SHA1, IsDir: event.Type == 17 || event.Type == 18}
	if event.Type == 14 {
		listing, err := run.service.remote.List(ctx, run.cookie, event.ParentID, 0)
		if err != nil {
			return err
		}
		found := false
		for offset := 0; ; {
			for _, child := range listing.Nodes {
				if child.ID == event.FileID {
					node = child
					found = true
					break
				}
			}
			if found || offset+len(listing.Nodes) >= listing.Count {
				break
			}
			if len(listing.Nodes) == 0 {
				return fmt.Errorf("分享父目录分页未前进")
			}
			offset += len(listing.Nodes)
			if err = run.budget(ctx); err != nil {
				return err
			}
			listing, err = run.service.remote.List(ctx, run.cookie, event.ParentID, offset)
			if err != nil {
				return err
			}
		}
		if !found {
			run.skip(ctx, event, "分享顶层项目已不存在")
			return nil
		}
	}
	return run.generate(ctx, node, event, map[string]bool{}, 0)
}

func (run *behaviorRun) skip(ctx context.Context, event domain.BehaviorEvent, reason string) {
	run.stats.Skipped++
	behaviorLog(ctx, reason, event)
}

func safeBehaviorName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00:") && !strings.ContainsAny(name, "\r\n")
}

func (run *behaviorRun) locate(ctx context.Context, node BehaviorNode) (*behaviorScope, []BehaviorNode, string, error) {
	if !validBehaviorID(node.ID) || !validBehaviorID(node.ParentID) || !safeBehaviorName(node.Name) {
		return nil, nil, "", fmt.Errorf("文件身份或名称不安全")
	}
	listing, err := run.service.remote.List(ctx, run.cookie, node.ParentID, 0)
	if err != nil {
		return nil, nil, "", err
	}
	chain := listing.Ancestors
	if len(chain) == 0 || len(chain) > 256 || chain[0].ID != "0" || chain[len(chain)-1].ID != node.ParentID {
		return nil, nil, "", fmt.Errorf("115祖先链不完整")
	}
	seen := map[string]bool{}
	for position, ancestor := range chain {
		if !validBehaviorID(ancestor.ID) || seen[ancestor.ID] {
			return nil, nil, "", fmt.Errorf("115祖先链存在环或无效ID")
		}
		seen[ancestor.ID] = true
		if position > 0 && (ancestor.ParentID != chain[position-1].ID || !safeBehaviorName(ancestor.Name)) {
			return nil, nil, "", fmt.Errorf("115祖先链父子关系无效")
		}
	}
	for position := len(chain) - 1; position >= 0; position-- {
		for _, root := range run.roots {
			if chain[position].ID != root.cid {
				continue
			}
			relative := []string{}
			for _, ancestor := range chain[position+1:] {
				relative = append(relative, ancestor.Name)
			}
			relative = append(relative, node.Name)
			return &root, chain[position:], filepath.Join(relative...), nil
		}
	}
	return nil, nil, "", nil
}

func (run *behaviorRun) generate(ctx context.Context, node BehaviorNode, event domain.BehaviorEvent, seen map[string]bool, depth int) error {
	if err := run.budget(ctx); err != nil {
		return err
	}
	if depth > 128 || seen[node.ID] {
		return fmt.Errorf("递归目录过深或存在环")
	}
	seen[node.ID] = true
	root, ancestors, relative, err := run.locate(ctx, node)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		run.skip(ctx, event, "目录边界解析失败，显式跳过，禁止回退")
		return nil
	}
	if root == nil {
		run.skip(ctx, event, "事件不在配置目录子树内")
		return nil
	}
	for position, ancestor := range ancestors {
		components := []string{root.LocalPath}
		for _, child := range ancestors[1 : position+1] {
			components = append(components, child.Name)
		}
		item := domain.BehaviorIndex{CloudID: run.account, FileID: ancestor.ID, ParentID: ancestor.ParentID, Path: filepath.Join(components...), IsDir: true}
		if err = run.rememberDirectory(ctx, item); err != nil {
			return err
		}
	}
	if !node.IsDir {
		if !behaviorVideo(node.Name) {
			run.stats.Skipped++
			return nil
		}
		return run.write(ctx, node, *root, relative)
	}
	if err = run.rememberDirectory(ctx, domain.BehaviorIndex{CloudID: run.account, FileID: node.ID, ParentID: node.ParentID, Path: filepath.Join(root.LocalPath, relative), IsDir: true}); err != nil {
		return err
	}
	for offset := 0; ; {
		if err = run.budget(ctx); err != nil {
			return err
		}
		listing, listErr := run.service.remote.List(ctx, run.cookie, node.ID, offset)
		if listErr != nil {
			return listErr
		}
		for _, child := range listing.Nodes {
			if child.ParentID != node.ID {
				return fmt.Errorf("递归枚举父目录不匹配")
			}
			if err = run.generate(ctx, child, event, seen, depth+1); err != nil {
				return err
			}
		}
		offset += len(listing.Nodes)
		if offset >= listing.Count {
			break
		}
		if len(listing.Nodes) == 0 {
			return fmt.Errorf("子目录分页未前进")
		}
	}
	return nil
}

func (run *behaviorRun) rememberDirectory(ctx context.Context, item domain.BehaviorIndex) error {
	old, err := run.store.Get(ctx, item.FileID)
	if err != nil {
		return err
	}
	if old != nil {
		if !old.IsDir {
			return fmt.Errorf("目录身份与已有文件冲突")
		}
		return nil
	}
	return run.store.Put(ctx, item)
}
