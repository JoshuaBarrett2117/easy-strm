package service

import (
	"context"
	"fmt"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

type offlineSnapshotKey struct {
	accountID int
	status    string
	page      int
	size      int
}

type offlineSnapshotEntry struct {
	result     *domain.OfflineDownloadTaskListResponse
	sampled    time.Time
	refreshing bool
}

type offlineSyncAttempt struct {
	done chan struct{}
	ran  bool
	err  error
}

// ListSnapshot 仅读进程快照并触发有界后台刷新；未准备好时返回明确错误，绝不伪造 total。
func (s *OfflineDownloadService) ListSnapshot(ctx context.Context, accountID int, status string, page, size int) (*domain.OfflineDownloadTaskListResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := offlineSnapshotKey{accountID: max(accountID, 0), status: status, page: max(page, 1), size: min(max(size, 1), 100)}
	if size <= 0 {
		key.size = 20
	}
	s.syncMu.Lock()
	if s.snapshots == nil {
		s.snapshots = make(map[offlineSnapshotKey]*offlineSnapshotEntry)
		s.snapshotSlots = make(chan struct{}, 2)
	}
	entry := s.snapshots[key]
	if entry == nil {
		if len(s.snapshots) >= 128 {
			var oldestKey offlineSnapshotKey
			var oldest *offlineSnapshotEntry
			for candidateKey, candidate := range s.snapshots {
				if !candidate.refreshing && (oldest == nil || candidate.sampled.Before(oldest.sampled)) {
					oldestKey, oldest = candidateKey, candidate
				}
			}
			if oldest == nil {
				s.syncMu.Unlock()
				return nil, fmt.Errorf("云下载快照刷新繁忙，请稍后重试")
			}
			delete(s.snapshots, oldestKey)
		}
		entry = &offlineSnapshotEntry{}
		s.snapshots[key] = entry
	}
	if !entry.refreshing && time.Since(entry.sampled) >= 10*time.Second {
		select {
		case s.snapshotSlots <- struct{}{}:
			entry.refreshing = true
			go s.refreshOfflineSnapshot(context.WithoutCancel(ctx), key, entry)
		default:
		}
	}
	result := copyOfflineSnapshot(entry.result)
	s.syncMu.Unlock()
	if result == nil {
		return nil, fmt.Errorf("云下载快照正在准备，请稍后重试")
	}
	return result, nil
}

func copyOfflineSnapshot(result *domain.OfflineDownloadTaskListResponse) *domain.OfflineDownloadTaskListResponse {
	if result == nil {
		return nil
	}
	copy := *result
	copy.Data = append([]domain.OfflineDownloadTask{}, result.Data...)
	return &copy
}

func (s *OfflineDownloadService) refreshOfflineSnapshot(parent context.Context, key offlineSnapshotKey, entry *offlineSnapshotEntry) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	defer func() {
		s.syncMu.Lock()
		entry.refreshing = false
		s.syncMu.Unlock()
		<-s.snapshotSlots
	}()
	read := func() (*domain.OfflineDownloadTaskListResponse, error) {
		result, err := s.recordDAO.SnapshotPage(ctx, key.accountID, key.status, key.page, key.size)
		if err != nil {
			return nil, err
		}
		s.fillAccountNamesContext(ctx, result.Data)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.syncMu.Lock()
		entry.result, entry.sampled = result, time.Now()
		s.syncMu.Unlock()
		return result, nil
	}
	result, err := read()
	if err != nil {
		logger.WithContext(ctx, "offline_snapshot").Log(logger.WARN, "云下载快照读取失败", nil, err)
		return
	}
	accounts := map[int]bool{}
	if key.accountID > 0 {
		accounts[key.accountID] = true
	}
	for _, record := range result.Data {
		if record.Status == domain.OfflineStatusPending || record.Status == domain.OfflineStatusDownloading {
			accounts[record.Cloud115ID] = true
		}
	}
	synced := false
	for accountID := range accounts {
		if err := ctx.Err(); err != nil {
			return
		}
		ran, err := s.SyncAccountOfflineTasks(ctx, accountID, "", false)
		if err != nil {
			logger.WithContext(ctx, "offline_snapshot").Log(logger.WARN, "云下载后台同步失败", logger.Fields{"account_id": accountID}, err)
			continue
		}
		synced = synced || ran
	}
	if synced {
		if _, err := read(); err != nil {
			logger.WithContext(ctx, "offline_snapshot").Log(logger.WARN, "云下载同步后快照读取失败", nil, err)
		}
	}
}

func (s *OfflineDownloadService) invalidateOfflineSnapshots() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	for key := range s.snapshots {
		delete(s.snapshots, key)
	}
}

func (s *OfflineDownloadService) offlineAccount(ctx context.Context, id int) (*domain.Cloud115, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader, ok := s.cloud115DAO.(interface {
		GetByIDContext(context.Context, int) (*domain.Cloud115, error)
	}); ok {
		return reader.GetByIDContext(ctx, id)
	}
	return s.cloud115DAO.GetByID(id)
}
