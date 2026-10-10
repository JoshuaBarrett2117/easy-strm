package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"easy-strm/internal/domain"
)

type embyOverviewSnapshot struct {
	done    chan struct{}
	baseURL string
	apiKey  string
	expires time.Time
	data    []byte
	err     error
}

func (s *EmbyMonitorService) context() context.Context {
	if s.requestContext != nil {
		return s.requestContext
	}
	return context.Background()
}

// GetOverviewContext 合并同实例并发概览并缓存五秒；实例启停与配置变更每次主键校验，取消不缓存。
func (s *EmbyMonitorService) GetOverviewContext(ctx context.Context, serverID int) (*domain.EmbyMonitorOverview, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	server, err := s.servers.GetByIDContext(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if server == nil || !server.Enabled {
		return nil, fmt.Errorf("Emby 实例不存在或已停用")
	}
	s.overviewMu.Lock()
	if s.overviews == nil {
		s.overviews = make(map[int]*embyOverviewSnapshot)
	}
	snapshot := s.overviews[serverID]
	reused := snapshot != nil && snapshot.baseURL == server.BaseURL && snapshot.apiKey == server.APIKey && (snapshot.expires.IsZero() || s.now().Before(snapshot.expires))
	if !reused {
		snapshot = &embyOverviewSnapshot{done: make(chan struct{}), baseURL: server.BaseURL, apiKey: server.APIKey}
		s.overviews[serverID] = snapshot
	}
	s.overviewMu.Unlock()
	if reused {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-snapshot.done:
		}
	} else {
		worker := &EmbyMonitorService{servers: s.servers, monitor: s.monitor, http: s.http, redis: s.redis, now: s.now, requestContext: ctx}
		result, loadErr := worker.loadOverview(server)
		if loadErr == nil {
			snapshot.data, loadErr = json.Marshal(result)
		}
		snapshot.err = loadErr
		s.overviewMu.Lock()
		snapshot.expires = s.now().Add(5 * time.Second)
		if loadErr != nil && s.overviews[serverID] == snapshot {
			delete(s.overviews, serverID)
		}
		close(snapshot.done)
		s.overviewMu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snapshot.err != nil {
		return nil, snapshot.err
	}
	var result domain.EmbyMonitorOverview
	if err := json.Unmarshal(snapshot.data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
