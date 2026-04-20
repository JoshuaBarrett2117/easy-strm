package service

import (
	"fmt"
	"strings"
	"time"

	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// DashboardStats Dashboard 统计数据聚合结果
type DashboardStats struct {
	Accounts     AccountStats     `json:"accounts"`
	MediaSources MediaSourceStats `json:"media_sources"`
	StrmFiles    StrmFileStats    `json:"strm_files"`
	Tasks        TaskStats        `json:"tasks"`
	Storage      StorageStats     `json:"storage"`
}

// AccountStats 115账号统计
type AccountStats struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Cooling  int `json:"cooling"`
	Disabled int `json:"disabled"`
}

// MediaSourceStats 媒体源统计
type MediaSourceStats struct {
	Total    int `json:"total"`
	Local    int `json:"local"`
	Cloud115 int `json:"cloud115"`
	Enabled  int `json:"enabled"`
}

// StrmFileStats STRM文件统计
type StrmFileStats struct {
	Total             int        `json:"total"`
	LastGenerationTime *string   `json:"last_generation_time"`
}

// TaskStats 任务统计
type TaskStats struct {
	Running       int `json:"running"`
	CompletedToday int `json:"completed_today"`
	FailedToday    int `json:"failed_today"`
}

// StorageStats 存储空间统计
type StorageStats struct {
	Accounts []AccountStorage `json:"accounts"`
}

// AccountStorage 单个账号的存储空间信息
type AccountStorage struct {
	Name       string  `json:"name"`
	Used       int64   `json:"used"`
	Total      int64   `json:"total"`
	Percentage float64 `json:"percentage"`
}

// DashboardService Dashboard 数据聚合服务
// 聚合 Cloud115DAO、MediaSourceDAO、StrmFileDAO、TaskRedisDAO 的数据
type DashboardService struct {
	cloud115DAO    *dao.Cloud115DAO
	mediaSourceDAO *dao.MediaSourceDAO
	strmFileDAO    *dao.StrmFileDAO
	taskRedisDAO   *dao.TaskRedisDAO
}

// NewDashboardService 创建 Dashboard 服务实例
func NewDashboardService(
	cloud115DAO *dao.Cloud115DAO,
	mediaSourceDAO *dao.MediaSourceDAO,
	strmFileDAO *dao.StrmFileDAO,
	taskRedisDAO *dao.TaskRedisDAO,
) *DashboardService {
	return &DashboardService{
		cloud115DAO:    cloud115DAO,
		mediaSourceDAO: mediaSourceDAO,
		strmFileDAO:    strmFileDAO,
		taskRedisDAO:   taskRedisDAO,
	}
}

// GetDashboardStats 聚合查询所有 Dashboard 统计数据
// 单次调用聚合多数据源，避免前端发起多次请求
func (s *DashboardService) GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{}

	// --- 并行聚合各维度数据，任一失败不影响其他维度 ---
	if err := s.fillAccountStats(stats); err != nil {
		logger.Warnf("DashboardService[GetDashboardStats] 获取账号统计失败: %v", err)
	}
	if err := s.fillMediaSourceStats(stats); err != nil {
		logger.Warnf("DashboardService[GetDashboardStats] 获取媒体源统计失败: %v", err)
	}
	if err := s.fillStrmFileStats(stats); err != nil {
		logger.Warnf("DashboardService[GetDashboardStats] 获取STRM文件统计失败: %v", err)
	}
	if err := s.fillTaskStats(stats); err != nil {
		logger.Warnf("DashboardService[GetDashboardStats] 获取任务统计失败: %v", err)
	}
	if err := s.fillStorageStats(stats); err != nil {
		logger.Warnf("DashboardService[GetDashboardStats] 获取存储统计失败: %v", err)
	}

	return stats, nil
}

// fillAccountStats 填充115账号统计：按状态分组计数
func (s *DashboardService) fillAccountStats(stats *DashboardStats) error {
	accounts, err := s.cloud115DAO.GetAll("id", "ASC")
	if err != nil {
		return fmt.Errorf("查询账号列表失败: %v", err)
	}

	result := AccountStats{Total: len(accounts)}
	for _, acc := range accounts {
		switch acc.Status {
		case domain.AccountStatusActive:
			result.Active++
		case domain.AccountStatusCooling:
			result.Cooling++
		case domain.AccountStatusDisabled:
			result.Disabled++
		default:
			// 未知状态归入 Active（兼容历史数据）
			result.Active++
		}
	}
	stats.Accounts = result
	return nil
}

// fillMediaSourceStats 填充媒体源统计：按类型和启用状态分组
func (s *DashboardService) fillMediaSourceStats(stats *DashboardStats) error {
	sources, err := s.mediaSourceDAO.GetAll("", "")
	if err != nil {
		return fmt.Errorf("查询媒体源列表失败: %v", err)
	}

	result := MediaSourceStats{Total: len(sources)}
	for _, src := range sources {
		if src.SourceType == domain.SourceTypeLocal {
			result.Local++
		} else if src.SourceType == domain.SourceTypeCloud115 {
			result.Cloud115++
		}
		if src.Enabled {
			result.Enabled++
		}
	}
	stats.MediaSources = result
	return nil
}

// fillStrmFileStats 填充STRM文件统计：总数和最近生成时间
func (s *DashboardService) fillStrmFileStats(stats *DashboardStats) error {
	total, err := s.strmFileDAO.CountAll()
	if err != nil {
		return fmt.Errorf("统计STRM文件总数失败: %v", err)
	}

	lastTime, err := s.strmFileDAO.GetLastGenerationTime()
	if err != nil {
		return fmt.Errorf("查询最近生成时间失败: %v", err)
	}

	result := StrmFileStats{Total: total}
	if lastTime != nil {
		formatted := lastTime.Format(time.RFC3339)
		result.LastGenerationTime = &formatted
	}
	stats.StrmFiles = result
	return nil
}

// fillTaskStats 填充任务统计：运行中/今日完成/今日失败
func (s *DashboardService) fillTaskStats(stats *DashboardStats) error {
	tasks, err := s.taskRedisDAO.GetAll()
	if err != nil {
		return fmt.Errorf("查询任务列表失败: %v", err)
	}

	todayStr := time.Now().Format("2006-01-02")
	result := TaskStats{}
	for _, task := range tasks {
		status, _ := task["status"].(string)
		createTime, _ := task["create_time"].(string)

		if status == "running" {
			result.Running++
		}
		// 按创建时间前缀匹配今日日期
		if strings.HasPrefix(createTime, todayStr) {
			if status == "completed" {
				result.CompletedToday++
			} else if status == "failed" {
				result.FailedToday++
			}
		}
	}
	stats.Tasks = result
	return nil
}

// fillStorageStats 填充存储空间统计：从数据库 quota_used 读取已用空间
// NOTE: total 字段依赖外部回调（115 API），若未注入则返回 0
func (s *DashboardService) fillStorageStats(stats *DashboardStats) error {
	accounts, err := s.cloud115DAO.GetAll("id", "ASC")
	if err != nil {
		return fmt.Errorf("查询账号列表失败: %v", err)
	}

	var accountStorages []AccountStorage
	for _, acc := range accounts {
		storage := AccountStorage{
			Name:  acc.Name,
			Used:  acc.QuotaUsed,
			Total: 0, // total 需要通过 115 API 获取，此处暂不填充
		}
		if storage.Total > 0 {
			storage.Percentage = float64(storage.Used) / float64(storage.Total) * 100
		}
		accountStorages = append(accountStorages, storage)
	}

	// 避免返回 null，空列表返回 []
	if accountStorages == nil {
		accountStorages = []AccountStorage{}
	}
	stats.Storage = StorageStats{Accounts: accountStorages}
	return nil
}
