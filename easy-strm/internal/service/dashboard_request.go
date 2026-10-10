package service

import (
	"context"
	"sync"
	"time"

	"easy-strm/internal/domain"
)

type dashboardRequestState struct {
	ctx          context.Context
	accounts     []*domain.Cloud115
	accountError error
	accountsRead bool
	tasks        []map[string]interface{}
	taskError    error
	tasksRead    bool
}

type dashboardFileStatsCache struct {
	sync.Mutex
	done    chan struct{}
	expires time.Time
	value   StrmFileStats
}

func (s *DashboardService) forDashboardRequest(ctx context.Context) *DashboardService {
	return &DashboardService{cloud115DAO: s.cloud115DAO, mediaSourceDAO: s.mediaSourceDAO, strmFileDAO: s.strmFileDAO, taskRedisDAO: s.taskRedisDAO, storageProvider: s.storageProvider, storageCache: s.storageCache, fileStatsCache: s.fileStatsCache, requestState: &dashboardRequestState{ctx: ctx}}
}

func (s *DashboardService) dashboardContext() context.Context {
	if s.requestState != nil {
		return s.requestState.ctx
	}
	return context.Background()
}

func (s *DashboardService) dashboardAccounts() ([]*domain.Cloud115, error) {
	if s.requestState == nil {
		return s.cloud115DAO.GetAllContext(s.dashboardContext(), "id", "ASC")
	}
	state := s.requestState
	if !state.accountsRead {
		state.accounts, state.accountError = s.cloud115DAO.GetAllContext(state.ctx, "id", "ASC")
		state.accountsRead = true
	}
	return state.accounts, state.accountError
}

func (s *DashboardService) dashboardTasks() ([]map[string]interface{}, error) {
	if s.requestState == nil {
		return s.taskRedisDAO.WithContext(s.dashboardContext()).GetUnified()
	}
	state := s.requestState
	if !state.tasksRead {
		state.tasks, state.taskError = s.taskRedisDAO.WithContext(state.ctx).GetUnified()
		state.tasksRead = true
	}
	return state.tasks, state.taskError
}

func (s *DashboardService) dashboardFileStats() (StrmFileStats, error) {
	cache := s.fileStatsCache
	if cache == nil {
		return s.readDashboardFileStats()
	}
	for {
		if err := s.dashboardContext().Err(); err != nil {
			return StrmFileStats{}, err
		}
		cache.Lock()
		if time.Now().Before(cache.expires) {
			value := copyDashboardFileStats(cache.value)
			cache.Unlock()
			return value, nil
		}
		if cache.done != nil {
			done := cache.done
			cache.Unlock()
			select {
			case <-s.dashboardContext().Done():
				return StrmFileStats{}, s.dashboardContext().Err()
			case <-done:
				continue
			}
		}
		cache.done = make(chan struct{})
		cache.Unlock()
		value, err := s.readDashboardFileStats()
		cache.Lock()
		if err == nil {
			cache.value = copyDashboardFileStats(value)
			cache.expires = time.Now().Add(5 * time.Second)
		}
		close(cache.done)
		cache.done = nil
		cache.Unlock()
		return value, err
	}
}

func (s *DashboardService) readDashboardFileStats() (StrmFileStats, error) {
	total, lastTime, err := s.strmFileDAO.StatsContext(s.dashboardContext())
	value := StrmFileStats{Total: total}
	if lastTime != nil {
		formatted := lastTime.Format(time.RFC3339)
		value.LastGenerationTime = &formatted
	}
	return value, err
}

func copyDashboardFileStats(value StrmFileStats) StrmFileStats {
	if value.LastGenerationTime != nil {
		formatted := *value.LastGenerationTime
		value.LastGenerationTime = &formatted
	}
	return value
}
