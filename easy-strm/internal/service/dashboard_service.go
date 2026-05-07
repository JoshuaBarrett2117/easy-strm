package service

import (
	"fmt"
	"runtime"
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

// DashboardOverview Dashboard 首页总览结果
type DashboardOverview struct {
	Stats        DashboardStats     `json:"stats"`
	StrmTask     TaskOverview       `json:"strm_task"`
	ArchiveTask  TaskOverview       `json:"archive_task"`
	RecentTasks  []map[string]any   `json:"recent_tasks"`
	RecentIngest []RecentIngestItem `json:"recent_ingest"`
}

// TaskOverview 首页任务摘要
type TaskOverview struct {
	Pending   int  `json:"pending"`
	FileCount int  `json:"file_count"`
	Running   bool `json:"running"`
	Total     int  `json:"total"`
}

// RecentIngestItem 最近入库项
type RecentIngestItem struct {
	TaskID      string `json:"task_id"`
	TaskName    string `json:"task_name"`
	TaskType    string `json:"task_type"`
	Status      string `json:"status"`
	UpdateTime  string `json:"update_time"`
	SourceName  string `json:"source_name"`
	ResultBrief string `json:"result_brief"`
}

// DashboardResourceMonitor Dashboard 资源监控结果
type DashboardResourceMonitor struct {
	MemoryBytes    uint64 `json:"memory_bytes"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	SystemBytes    uint64 `json:"system_bytes"`
	Goroutines     int    `json:"goroutines"`
	CPUCores       int    `json:"cpu_cores"`
	RunningTasks   int    `json:"running_tasks"`
	ActiveAccounts int    `json:"active_accounts"`
	EnabledSources int    `json:"enabled_sources"`
	SampledAt      string `json:"sampled_at"`
}

// DashboardTrendPoint Dashboard 趋势点
type DashboardTrendPoint struct {
	Date    string `json:"date"`
	Value   int    `json:"value"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

// DashboardTrend Dashboard 趋势结果
type DashboardTrend struct {
	Days   int                   `json:"days"`
	Type   string                `json:"type"`
	Points []DashboardTrendPoint `json:"points"`
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
	Total              int     `json:"total"`
	LastGenerationTime *string `json:"last_generation_time"`
}

// TaskStats 任务统计
type TaskStats struct {
	Running        int `json:"running"`
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

// GetDashboardOverview 获取首页总览数据
func (s *DashboardService) GetDashboardOverview() (*DashboardOverview, error) {
	stats, err := s.GetDashboardStats()
	if err != nil {
		return nil, err
	}

	tasks, err := s.taskRedisDAO.GetUnified()
	if err != nil {
		return nil, fmt.Errorf("查询任务总览失败: %v", err)
	}

	overview := &DashboardOverview{
		Stats:        *stats,
		StrmTask:     s.buildTaskOverview(tasks, []string{"strm_generate"}),
		ArchiveTask:  s.buildTaskOverview(tasks, []string{"organize", "watch_auto_organize", "scrape"}),
		RecentTasks:  s.limitTasks(tasks, 8),
		RecentIngest: s.buildRecentIngest(tasks, 8),
	}

	return overview, nil
}

// GetDashboardResourceMonitor 获取 Dashboard 资源监控数据
func (s *DashboardService) GetDashboardResourceMonitor() (*DashboardResourceMonitor, error) {
	stats, err := s.GetDashboardStats()
	if err != nil {
		return nil, err
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return &DashboardResourceMonitor{
		MemoryBytes:    mem.Alloc,
		HeapAllocBytes: mem.HeapAlloc,
		SystemBytes:    mem.Sys,
		Goroutines:     runtime.NumGoroutine(),
		CPUCores:       runtime.NumCPU(),
		RunningTasks:   stats.Tasks.Running,
		ActiveAccounts: stats.Accounts.Active,
		EnabledSources: stats.MediaSources.Enabled,
		SampledAt:      time.Now().Format(time.RFC3339),
	}, nil
}

// GetDashboardTrend 获取 Dashboard 趋势数据
func (s *DashboardService) GetDashboardTrend(kind string, days int) (*DashboardTrend, error) {
	if days <= 0 {
		days = 7
	}

	tasks, err := s.taskRedisDAO.GetAll()
	if err != nil {
		return nil, fmt.Errorf("查询任务趋势失败: %v", err)
	}

	type pointCounter struct {
		value   int
		success int
		failed  int
	}

	acceptedTypes := map[string]bool{}
	switch kind {
	case "archive":
		acceptedTypes["organize"] = true
		acceptedTypes["watch_auto_organize"] = true
		acceptedTypes["scrape"] = true
	default:
		kind = "strm"
		acceptedTypes["strm_generate"] = true
	}

	start := time.Now().AddDate(0, 0, -(days - 1))
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())

	buckets := make(map[string]*pointCounter, days)
	for i := 0; i < days; i++ {
		current := start.AddDate(0, 0, i)
		key := current.Format("2006-01-02")
		buckets[key] = &pointCounter{}
	}

	for _, task := range tasks {
		taskType, _ := task["task_type"].(string)
		if !acceptedTypes[taskType] {
			continue
		}

		createTime, _ := task["create_time"].(string)
		if len(createTime) < 10 {
			continue
		}
		key := createTime[:10]
		counter, exists := buckets[key]
		if !exists {
			continue
		}

		counter.value++
		status, _ := task["status"].(string)
		if status == "completed" {
			counter.success++
		} else if status == "failed" {
			counter.failed++
		}
	}

	points := make([]DashboardTrendPoint, 0, days)
	for i := 0; i < days; i++ {
		current := start.AddDate(0, 0, i)
		key := current.Format("2006-01-02")
		counter := buckets[key]
		points = append(points, DashboardTrendPoint{
			Date:    key,
			Value:   counter.value,
			Success: counter.success,
			Failed:  counter.failed,
		})
	}

	return &DashboardTrend{
		Days:   days,
		Type:   kind,
		Points: points,
	}, nil
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

func (s *DashboardService) buildTaskOverview(tasks []map[string]interface{}, acceptedTypes []string) TaskOverview {
	typeSet := make(map[string]bool, len(acceptedTypes))
	for _, item := range acceptedTypes {
		typeSet[item] = true
	}

	overview := TaskOverview{}
	for _, task := range tasks {
		taskType, _ := task["task_type"].(string)
		if !typeSet[taskType] {
			continue
		}
		overview.Total++

		status, _ := task["status"].(string)
		if status == "pending" || status == "running" {
			overview.Pending++
		}
		if status == "running" {
			overview.Running = true
		}

		switch value := task["total_files"].(type) {
		case float64:
			overview.FileCount += int(value)
		case int:
			overview.FileCount += value
		}
	}

	return overview
}

func (s *DashboardService) limitTasks(tasks []map[string]interface{}, limit int) []map[string]any {
	if limit <= 0 || len(tasks) == 0 {
		return []map[string]any{}
	}
	if len(tasks) < limit {
		limit = len(tasks)
	}

	result := make([]map[string]any, 0, limit)
	for _, task := range tasks[:limit] {
		result = append(result, task)
	}
	return result
}

func (s *DashboardService) buildRecentIngest(tasks []map[string]interface{}, limit int) []RecentIngestItem {
	if limit <= 0 {
		return []RecentIngestItem{}
	}

	result := make([]RecentIngestItem, 0, limit)
	for _, task := range tasks {
		taskType, _ := task["task_type"].(string)
		status, _ := task["status"].(string)
		if status != "completed" {
			continue
		}
		if taskType != "organize" && taskType != "watch_auto_organize" && taskType != "strm_generate" {
			continue
		}

		metadata, _ := task["metadata"].(map[string]interface{})
		sourceName, _ := metadata["source_name"].(string)
		resultBrief, _ := metadata["result_summary"].(string)
		if resultBrief == "" {
			resultBrief = "任务已完成"
		}

		taskID, _ := task["task_id"].(string)
		taskName, _ := task["task_name"].(string)
		updateTime, _ := task["update_time"].(string)

		result = append(result, RecentIngestItem{
			TaskID:      taskID,
			TaskName:    taskName,
			TaskType:    taskType,
			Status:      status,
			UpdateTime:  updateTime,
			SourceName:  sourceName,
			ResultBrief: resultBrief,
		})

		if len(result) >= limit {
			break
		}
	}

	return result
}
