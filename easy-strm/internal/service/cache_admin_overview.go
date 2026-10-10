package service

import (
	"context"
	"encoding/json"
	"time"
)

type cacheOverviewSnapshot struct {
	done        chan struct{}
	data        []byte
	err         error
	expires     time.Time
	invalidated bool
}

func (s *CacheAdminService) overviewContext() context.Context {
	if s.requestContext != nil {
		return s.requestContext
	}
	return context.Background()
}

func (s *CacheAdminService) invalidateOverview() {
	s.overviewMu.Lock()
	defer s.overviewMu.Unlock()
	if s.overview != nil {
		s.overview.invalidated = true
		s.overview = nil
	}
}

// GetOverviewContext 合并并发概览并缓存五秒，数据库查询保留取消；不更改Redis维护扫描协议。
func (s *CacheAdminService) GetOverviewContext(ctx context.Context) (*CacheOverview, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.overviewMu.Lock()
		snapshot := s.overview
		reused := snapshot != nil && (snapshot.expires.IsZero() || time.Now().Before(snapshot.expires))
		if !reused {
			snapshot = &cacheOverviewSnapshot{done: make(chan struct{})}
			s.overview = snapshot
		}
		s.overviewMu.Unlock()
		if reused {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-snapshot.done:
			}
		} else {
			worker := &CacheAdminService{db: s.db, redisClient: s.redisClient, requestContext: ctx}
			result, err := worker.loadOverview()
			if err == nil {
				snapshot.data, err = json.Marshal(result)
			}
			snapshot.err = err
			s.overviewMu.Lock()
			snapshot.expires = time.Now().Add(5 * time.Second)
			if err != nil && s.overview == snapshot {
				s.overview = nil
			}
			close(snapshot.done)
			s.overviewMu.Unlock()
		}
		s.overviewMu.Lock()
		invalidated := snapshot.invalidated
		s.overviewMu.Unlock()
		if invalidated {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if snapshot.err != nil {
			return nil, snapshot.err
		}
		var result CacheOverview
		if err := json.Unmarshal(snapshot.data, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
}
