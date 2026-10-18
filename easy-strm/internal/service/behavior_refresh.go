package service

import (
	"context"
	"easy-strm/internal/pkg/logger"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// BehaviorEmbyGateway 为行为同步限定 Emby 只读能力，便于测试替换真实服务。
type BehaviorEmbyGateway interface {
	IsEnabled() bool
	ListLibraries() ([]EmbyVirtualFolderInfo, error)
	RefreshLibrary(string) (*EmbyLibraryRefreshResult, error)
}

// EmbyBehaviorRefresher 把一批本地变更路径映射到 Emby 媒体库并合并刷新，
// 全程去重与限流，避免每个事件触发一次刷新造成风暴。
type EmbyBehaviorRefresher struct {
	gateway     BehaviorEmbyGateway
	minInterval time.Duration
	mu          sync.Mutex
	lastRefresh time.Time
}

// NewEmbyBehaviorRefresher 构造限流刷新器，间隔为0时使用默认值。
func NewEmbyBehaviorRefresher(gateway BehaviorEmbyGateway) *EmbyBehaviorRefresher {
	return &EmbyBehaviorRefresher{gateway: gateway, minInterval: 60 * time.Second}
}

// RefreshBehaviorPaths 将去重后的路径映射到受影响媒体库，逐库触发一次刷新。
// Emby 刷新是尽力而为的联动：任何失败只记日志不返回错误，避免已完成的 STRM 写入被阻塞。
func (r *EmbyBehaviorRefresher) RefreshBehaviorPaths(ctx context.Context, paths []string) error {
	if r == nil || r.gateway == nil || !r.gateway.IsEnabled() {
		return nil
	}
	normalized := map[string]bool{}
	for _, raw := range paths {
		if p, err := NormalizeStrmOutputPath(raw); err == nil {
			normalized[p] = true
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	libraries, err := r.gateway.ListLibraries()
	if err != nil {
		logger.WithContext(ctx, "behavior_refresh").Log(logger.WARN, "读取 Emby 媒体库失败，跳过刷新", nil, err)
		return nil
	}
	targets := map[string]bool{}
	for _, lib := range libraries {
		if lib.ItemID == "" {
			continue
		}
		locations := append(append([]string{}, lib.Locations...), lib.Path)
		matched := false
		for _, loc := range locations {
			loc = strings.TrimSpace(loc)
			if loc == "" {
				continue
			}
			root, err := NormalizeStrmOutputPath(loc)
			if err != nil {
				continue
			}
			for p := range normalized {
				if behaviorWithin(root, p) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			targets[lib.ItemID] = true
		}
	}
	if len(targets) == 0 {
		logger.WithContext(ctx, "behavior_refresh").Log(logger.WARN, "变更路径未映射到任何 Emby 媒体库，跳过刷新", logger.Fields{"paths": len(normalized)}, nil)
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.minInterval > 0 && time.Since(r.lastRefresh) < r.minInterval {
		return nil
	}
	r.lastRefresh = time.Now()
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var errs []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if _, err := r.gateway.RefreshLibrary(id); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		logger.WithContext(ctx, "behavior_refresh").Log(logger.WARN, "部分 Emby 媒体库刷新失败", logger.Fields{"libraries": len(ids), "failed": len(errs)}, errors.Join(errs...))
	}
	return nil
}
