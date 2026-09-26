package service

import (
	"context"
	"easy-strm/internal/domain"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ShareTaskSettings 表示整个分享扫描与识别任务的总时限，0为无限制。
type ShareTaskSettings struct {
	TimeoutMinutes  int `json:"timeout_minutes"`
	SyncWorkers     int `json:"sync_workers"`
	IdentifyWorkers int `json:"identify_workers"`
	WorkerCount     int `json:"worker_count,omitempty"`
}

// ShareTaskSettingsStore 复用系统配置存储，支持本地测试替换。
type ShareTaskSettingsStore interface {
	GetByKey(string) (*domain.SystemConfig, error)
	Upsert(string, string) error
}

const shareTaskTimeoutKey = "share_identify_timeout_minutes"
const shareSyncWorkersKey = "share_sync_workers"
const shareIdentifyWorkersKey = "share_identify_workers"
const shareTaskWorkerCountKey = "share_identify_worker_count"
const defaultShareWorkerCount = 4

func parseShareWorker(value string, key string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 16 {
		return 0, fmt.Errorf("%s 配置需为1至16的整数", key)
	}
	return n, nil
}

// SetTaskSettingsStore 在路由装配阶段设置持久化配置存储。
func (s *ShareRecordService) SetTaskSettingsStore(store ShareTaskSettingsStore) {
	s.taskSettingsStore = store
}

// GetTaskSettings 获取任务时限；缺省为无限制，不影响各外部接口自身超时。
func (s *ShareRecordService) GetTaskSettings() (ShareTaskSettings, error) {
	value := ShareTaskSettings{}
	value.SyncWorkers, value.IdentifyWorkers = 3, 3
	value.WorkerCount = defaultShareWorkerCount
	if s.taskSettingsStore == nil {
		return value, nil
	}
	row, err := s.taskSettingsStore.GetByKey(shareTaskTimeoutKey)
	if err != nil {
		return value, err
	}
	if row == nil {
		row, err = s.taskSettingsStore.GetByKey(shareSyncWorkersKey)
		if err != nil {
			return value, err
		}
		if row != nil && row.ConfigKey == shareSyncWorkersKey {
			value.SyncWorkers, err = parseShareWorker(row.ConfigVal, "分享同步 worker 数")
			if err != nil {
				return value, err
			}
		}
		row, err = s.taskSettingsStore.GetByKey(shareIdentifyWorkersKey)
		if err != nil {
			return value, err
		}
		if row != nil && row.ConfigKey == shareIdentifyWorkersKey {
			value.IdentifyWorkers, err = parseShareWorker(row.ConfigVal, "分享识别 worker 数")
			if err != nil {
				return value, err
			}
		}
		return value, nil
	}
	minutes, err := strconv.Atoi(row.ConfigVal)
	if err != nil || minutes < 0 || minutes > 43200 {
		return value, fmt.Errorf("分享识别超时配置无效，请重新保存")
	}
	value.TimeoutMinutes = minutes
	for key, target := range map[string]*int{shareSyncWorkersKey: &value.SyncWorkers, shareIdentifyWorkersKey: &value.IdentifyWorkers} {
		row, err := s.taskSettingsStore.GetByKey(key)
		if err != nil {
			return value, err
		}
		if row != nil && row.ConfigKey == key {
			*target, err = parseShareWorker(row.ConfigVal, key)
			if err != nil {
				return value, err
			}
		}
	}
	value.WorkerCount = value.IdentifyWorkers
	if row, getErr := s.taskSettingsStore.GetByKey(shareTaskWorkerCountKey); getErr == nil && row != nil {
		workers, parseErr := parseShareWorker(row.ConfigVal, "分享识别并发数")
		if parseErr != nil {
			return value, parseErr
		}
		value.WorkerCount = workers
	} else if getErr != nil {
		return value, getErr
	}
	return value, nil
}

// ValidateShareWorkerSettings 校验系统配置中的分享 worker 数量。
func ValidateShareWorkerSettings(configs map[string]string) error {
	for key, label := range map[string]string{shareSyncWorkersKey: "分享同步 worker 数", shareIdentifyWorkersKey: "分享识别 worker 数"} {
		if value, ok := configs[key]; ok {
			if _, err := parseShareWorker(value, label); err != nil {
				return err
			}
		}
	}
	return nil
}

// SaveTaskSettings 保存后对新发起任务生效，不改变正在运行任务的上下文。
func (s *ShareRecordService) SaveTaskSettings(value ShareTaskSettings) error {
	if value.TimeoutMinutes < 0 || value.TimeoutMinutes > 43200 {
		return fmt.Errorf("总时限需为0至43200分钟，0表示无限制")
	}
	if s.taskSettingsStore == nil {
		return fmt.Errorf("任务配置存储未初始化")
	}
	if value.WorkerCount == 0 {
		if value.IdentifyWorkers > 0 {
			value.WorkerCount = value.IdentifyWorkers
		} else {
			value.WorkerCount = defaultShareWorkerCount
		}
	}
	if value.WorkerCount < 1 || value.WorkerCount > 16 {
		return fmt.Errorf("分享识别并发数需为1至16的整数")
	}
	if err := s.taskSettingsStore.Upsert(shareTaskTimeoutKey, strconv.Itoa(value.TimeoutMinutes)); err != nil {
		return err
	}
	return s.taskSettingsStore.Upsert(shareTaskWorkerCountKey, strconv.Itoa(value.WorkerCount))
}

func newShareTaskContext(parent context.Context, minutes int) (context.Context, context.CancelFunc) {
	if minutes == 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, time.Duration(minutes)*time.Minute)
}

func (s *ShareRecordService) finishShareTaskContext(taskID string, err error, minutes int) {
	if errors.Is(err, context.Canceled) {
		_ = s.tasks.UpdateStatus(taskID, "cancelled")
		return
	}
	_ = s.tasks.SetError(taskID, fmt.Sprintf("分享识别任务达到配置的总时限（%d分钟），已保存处理结果；可调整为无限制后重新发起", minutes))
}
