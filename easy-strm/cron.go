package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// CronScheduler 全局cron调度器
type CronScheduler struct {
	cron   *cron.Cron
	entrys map[int]cron.EntryID
	mu     sync.RWMutex
}

var scheduler *CronScheduler

// InitCronScheduler 初始化cron调度器
func InitCronScheduler() error {
	Debug("Initializing cron scheduler")

	scheduler = &CronScheduler{
		cron:   cron.New(cron.WithSeconds(), cron.WithLocation(time.Local)),
		entrys: make(map[int]cron.EntryID),
	}

	scheduler.cron.Start()
	Info("Cron scheduler started")

	// 添加账号冷却恢复定时任务，每分钟执行一次
	_, err := scheduler.cron.AddFunc("0 * * * * *", func() {
		RecoverCoolingAccounts()
	})
	if err != nil {
		Warn("Failed to add cooling account recovery task: %v", err)
	} else {
		Info("账号冷却恢复定时任务已添加 (每分钟执行)")
	}

	if err := LoadCronTasksFromDB(); err != nil {
		Error("Failed to load cron tasks from database: %v", err)
		return err
	}

	return nil
}

// RecoverCoolingAccounts 恢复超过冷却时间的账号（由定时任务调用）
func RecoverCoolingAccounts() {
	const coolingDuration = 5 * time.Minute

	Debug("[cooling] 开始检查冷却账号...")

	// 获取所有cooling状态的账号
	rows, err := db.Query("SELECT id, name, status, cooling_start_time FROM t_cloud_115 WHERE status = 'cooling'")
	if err != nil {
		Error("[cooling] 查询冷却账号失败: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now()
	recoveredCount := 0

	for rows.Next() {
		var id int
		var name string
		var status string
		var coolingStartTime *time.Time

		if err := rows.Scan(&id, &name, &status, &coolingStartTime); err != nil {
			Error("[cooling] 扫描账号行失败: %v", err)
			continue
		}

		if coolingStartTime == nil {
			// 如果没有冷却开始时间，视为已超过冷却时间，直接恢复
			if _, err := db.Exec("UPDATE t_cloud_115 SET status = 'active', cooling_start_time = NULL WHERE id = $1", id); err != nil {
				Error("[cooling] 恢复账号 %d 失败: %v", id, err)
				continue
			}
			recoveredCount++
			Info("[cooling] 恢复账号 %s (ID: %d) - 无冷却开始时间", name, id)
			continue
		}

		// 检查冷却时间是否已超过5分钟
		coolingElapsed := now.Sub(*coolingStartTime)
		if coolingElapsed >= coolingDuration {
			if _, err := db.Exec("UPDATE t_cloud_115 SET status = 'active', cooling_start_time = NULL WHERE id = $1", id); err != nil {
				Error("[cooling] 恢复账号 %d 失败: %v", id, err)
				continue
			}
			recoveredCount++
			Info("[cooling] 恢复账号 %s (ID: %d) - 冷却时间 %.0f 分钟", name, id, coolingElapsed.Minutes())
		}
	}

	if recoveredCount > 0 {
		Info("[cooling] 本次共恢复 %d 个账号", recoveredCount)
	} else {
		Debug("[cooling] 没有需要恢复的冷却账号")
	}
}

// LoadCronTasksFromDB 从数据库加载定时任务
func LoadCronTasksFromDB() error {
	Debug("Loading cron tasks from database")

	tasks, err := GetEnabledCronTasks()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if err := scheduler.AddTask(task); err != nil {
			Warn("Failed to add cron task %s: %v", task.TaskName, err)
			continue
		}
		Info("Loaded cron task: %s (ID: %d)", task.TaskName, task.ID)
	}

	return nil
}

// AddTask 添加定时任务到调度器
func (s *CronScheduler) AddTask(task *CronTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entrys[task.ID]; exists {
		Debug("Task %d already exists, removing first", task.ID)
		s.cron.Remove(s.entrys[task.ID])
		delete(s.entrys, task.ID)
	}

	cronExpr := task.CronExpr
	fields := strings.Fields(cronExpr)
	if len(fields) == 5 {
		cronExpr = "0 " + cronExpr
		Debug("Converting 5-field cron expression to 6-field: %s -> %s", task.CronExpr, cronExpr)
	}

	entryID, err := s.cron.AddFunc(cronExpr, func() {
		ExecuteCronTask(task)
	})
	if err != nil {
		return fmt.Errorf("failed to add cron task: %v", err)
	}

	s.entrys[task.ID] = entryID

	nextRun := s.cron.Entry(entryID).Next
	task.NextRunTime = &nextRun
	UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, task.LastRunStatus, task.LastRunMessage)

	Debug("Added cron task %s (ID: %d), next run: %v", task.TaskName, task.ID, nextRun)
	return nil
}

// RemoveTask 从调度器移除定时任务
func (s *CronScheduler) RemoveTask(taskID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entrys[taskID]; exists {
		s.cron.Remove(entryID)
		delete(s.entrys, taskID)
		Debug("Removed cron task ID: %d", taskID)
	}
}

// UpdateTask 更新调度器中的定时任务
func (s *CronScheduler) UpdateTask(task *CronTask) error {
	s.RemoveTask(task.ID)
	if task.Status == "enabled" {
		return s.AddTask(task)
	}
	return nil
}

// GetNextRunTime 获取任务的下次执行时间
func (s *CronScheduler) GetNextRunTime(taskID int) *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entryID, exists := s.entrys[taskID]; exists {
		next := s.cron.Entry(entryID).Next
		return &next
	}
	return nil
}

// StopScheduler 停止调度器
func StopScheduler() {
	if scheduler != nil {
		scheduler.cron.Stop()
		Info("Cron scheduler stopped")
	}
}

// ExecuteCronTask 执行定时任务
func ExecuteCronTask(task *CronTask) {
	Info("[cron] Executing cron task: %s (ID: %d), type: %s", task.TaskName, task.ID, task.TaskType)

	now := time.Now()
	task.LastRunTime = &now

	var taskType TaskType
	if task.TaskType == "full_generate" {
		taskType = TaskTypeStrmGenerate
	} else {
		taskType = TaskTypeIncrementalSync
	}

	taskStatus, err := CreateTask(fmt.Sprintf("cron_%d_%d", task.ID, now.Unix()), taskType, task.TaskName)
	if err != nil {
		Error("[cron] Failed to create task for cron job: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", fmt.Sprintf("创建任务失败: %v", err))
		return
	}

	UpdateTaskStatus(taskStatus.TaskID, TaskStatusRunning)

	strmConfig, err := GetStrmConfigByID(task.StrmConfigID)
	if err != nil {
		Error("[cron] Failed to get strm config: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", fmt.Sprintf("获取STRM配置失败: %v", err))
		SetTaskError(taskStatus.TaskID, fmt.Sprintf("获取STRM配置失败: %v", err))
		return
	}

	cloud115, err := GetCloud115ByID(task.Cloud115ID)
	if err != nil {
		Error("[cron] Failed to get cloud115 account: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", fmt.Sprintf("获取115账号失败: %v", err))
		SetTaskError(taskStatus.TaskID, fmt.Sprintf("获取115账号失败: %v", err))
		return
	}

	var successMsg string
	if task.TaskType == "full_generate" {
		result, err := RunFullStrmGenerate(strmConfig, cloud115, taskStatus.TaskID)
		if err != nil {
			Error("[cron] Full STRM generate failed: %v", err)
			UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", err.Error())
			SetTaskError(taskStatus.TaskID, err.Error())
			return
		}
		successMsg = fmt.Sprintf("成功: 生成 %d 个STRM文件", result.Total)
	} else {
		result, err := RunIncrementalSync(strmConfig, cloud115, taskStatus.TaskID)
		if err != nil {
			Error("[cron] Incremental sync failed: %v", err)
			UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", err.Error())
			SetTaskError(taskStatus.TaskID, err.Error())
			return
		}
		successMsg = fmt.Sprintf("成功: 新增 %d, 删除 %d, 跳过 %d", result.Added, result.Deleted, result.Skipped)
	}

	nextRun := scheduler.GetNextRunTime(task.ID)
	task.NextRunTime = nextRun

	UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "success", successMsg)
	UpdateTaskStatus(taskStatus.TaskID, TaskStatusCompleted)

	Info("[cron] Cron task completed: %s (ID: %d), %s", task.TaskName, task.ID, successMsg)
}

// IncrementalSyncResult 增量同步结果
type IncrementalSyncResult struct {
	Added   int
	Deleted int
	Skipped int
}

// FullGenerateResult 全量生成结果
type FullGenerateResult struct {
	Total int
}

// RunIncrementalSync 执行增量同步
func RunIncrementalSync(strmConfig *StrmConfig, cloud115 *Cloud115, taskID string) (*IncrementalSyncResult, error) {
	Info("Running incremental sync for config ID: %d", strmConfig.ID)

	result := &IncrementalSyncResult{}

	client := NewClient(&Config{ServerURL: "http://localhost:8082"})

	cid, err := client.GetCIDByPath(strmConfig.NetDiskPath, cloud115.ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("获取网盘目录CID失败: %v", err)
	}

	cidInt := 0
	fmt.Sscanf(cid, "%d", &cidInt)

	rootName := filepath.Base(strmConfig.NetDiskPath)
	if rootName == "" || rootName == "." || rootName == "/" {
		rootName = "根目录"
	}

	exportResp, err := client.ExportDirectoryTree115(fmt.Sprintf("%d", cidInt), fmt.Sprintf("U_1_%d", cidInt), cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("导出目录树失败: %v", err)
	}

	if !exportResp.State {
		return nil, fmt.Errorf("导出目录树失败: %s", exportResp.Message)
	}

	exportId := exportResp.Data.ExportID.String()
	Info("Directory tree export triggered, export_id: %s", exportId)

	var pickCode string
	maxRetries := 60
	for i := 0; i < maxRetries; i++ {
		// 等待目录树导出时也检查取消标记
		if IsTaskCancelled(taskID) {
			Info("[cron] 任务在等待目录树导出时被取消: %s", taskID)
			return nil, fmt.Errorf("任务已取消")
		}

		statusResp, err := client.GetExportDirectoryTreeStatus(exportId, cloud115.Cookie)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		data := statusResp.GetFirstData()
		if data == nil {
			time.Sleep(5 * time.Second)
			continue
		}

		if data.Status == 2 && data.PickCode != "" {
			pickCode = data.PickCode
			break
		} else if data.Status == 3 || data.Status == -1 {
			return nil, fmt.Errorf("目录树导出失败，状态: %d", data.Status)
		}

		time.Sleep(5 * time.Second)
	}

	if pickCode == "" {
		return nil, fmt.Errorf("等待目录树导出超时")
	}

	fileData, err := client.DownloadDirectoryTreeFile(pickCode, cloud115.ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("下载目录树文件失败: %v", err)
	}

	entries, err := Parse115DirTreeFile(fileData)
	if err != nil {
		return nil, fmt.Errorf("解析目录树文件失败: %v", err)
	}

	dirTree := BuildTreeFromExport(entries, cid, rootName)

	extensions := strings.Split(strmConfig.Extension, ",")
	targetExts := make([]string, 0)
	for _, ext := range extensions {
		ext = strings.TrimSpace(ext)
		if ext != "" {
			targetExts = append(targetExts, strings.ToLower(strings.TrimPrefix(ext, ".")))
		}
	}

	collection := &VideoCollection{
		Videos: []VideoFile{},
	}
	extractVideoFiles(dirTree, "", strmConfig.NetDiskPath, cid, targetExts, collection, strmConfig.Cloud115Id, true)

	existingFiles, err := GetStrmFilesByConfigID(strmConfig.ID)
	if err != nil {
		return nil, fmt.Errorf("获取已有STRM文件记录失败: %v", err)
	}

	existingMap := make(map[string]*StrmFile)
	for _, f := range existingFiles {
		existingMap[f.FilePath] = f
	}

	cloudFilesMap := make(map[string]VideoFile)
	for _, video := range collection.Videos {
		cloudFilesMap[video.RelativePath] = video
	}

	for path, existingFile := range existingMap {
		// 协作式取消检查
		if IsTaskCancelled(taskID) {
			Info("增量同步任务已被取消(删除阶段): %s", taskID)
			return result, nil
		}

		if _, exists := cloudFilesMap[path]; !exists {
			if err := os.Remove(existingFile.LocalStrmPath); err != nil && !os.IsNotExist(err) {
				Warn("删除STRM文件失败 %s: %v", existingFile.LocalStrmPath, err)
			} else {
				DeleteStrmFileByPath(strmConfig.ID, path)
				result.Deleted++
				Debug("删除STRM文件: %s", existingFile.LocalStrmPath)
			}
		}
	}

	generator := NewStrmGeneratorWithServer(strmConfig.LocalPath, GetConfig().ServerURL, ".strm")
	generator.ProgressCallback = func(totalFiles, processedFiles, successFiles, failedFiles int) {
		UpdateTaskProgress(taskID, totalFiles, processedFiles, successFiles, failedFiles)
	}

	for _, video := range collection.Videos {
		// 协作式取消检查
		if IsTaskCancelled(taskID) {
			Info("增量同步任务已被取消: %s", taskID)
			return result, nil
		}

		if existingFile, exists := existingMap[video.RelativePath]; exists {
			if existingFile.PickCode == video.PickCode && existingFile.Sha1 == video.Sha1 {
				result.Skipped++
				continue
			}
		}

		localStrmPath, err := generator.GenerateSingleStrmFile(video, strmConfig.NetDiskPath)
		if err != nil {
			Warn("生成STRM文件失败 %s: %v", video.Name, err)
			continue
		}

		UpsertStrmFile(strmConfig.ID, video.Name, video.RelativePath, video.PickCode, video.Sha1, int64(video.Size), localStrmPath)
		result.Added++
		Debug("生成STRM文件: %s", localStrmPath)
	}

	Info("增量同步完成: 新增 %d, 删除 %d, 跳过 %d", result.Added, result.Deleted, result.Skipped)
	return result, nil
}

// RunFullStrmGenerate 执行全量生成STRM文件
func RunFullStrmGenerate(strmConfig *StrmConfig, cloud115 *Cloud115, taskID string) (*FullGenerateResult, error) {
	Info("[cron] Running full STRM generate for config ID: %d", strmConfig.ID)

	result := &FullGenerateResult{}

	client := NewClient(&Config{ServerURL: "http://localhost:8082"})
	Info("[cron] STRM output directory: %s", strmConfig.LocalPath)

	cid, err := client.GetCIDByPath(strmConfig.NetDiskPath, cloud115.ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("获取网盘目录CID失败: %v", err)
	}

	cidInt := 0
	fmt.Sscanf(cid, "%d", &cidInt)

	rootName := filepath.Base(strmConfig.NetDiskPath)
	if rootName == "" || rootName == "." || rootName == "/" {
		rootName = "根目录"
	}

	exportResp, err := client.ExportDirectoryTree115(fmt.Sprintf("%d", cidInt), fmt.Sprintf("U_1_%d", cidInt), cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("导出目录树失败: %v", err)
	}

	if !exportResp.State {
		return nil, fmt.Errorf("导出目录树失败: %s", exportResp.Message)
	}

	exportId := exportResp.Data.ExportID.String()
	Info("[cron] Directory tree export triggered, export_id: %s", exportId)

	var pickCode string
	maxRetries := 60
	for i := 0; i < maxRetries; i++ {
		statusResp, err := client.GetExportDirectoryTreeStatus(exportId, cloud115.Cookie)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		data := statusResp.GetFirstData()
		if data == nil {
			time.Sleep(5 * time.Second)
			continue
		}

		if data.Status == 2 && data.PickCode != "" {
			pickCode = data.PickCode
			break
		} else if data.Status == 3 || data.Status == -1 {
			return nil, fmt.Errorf("目录树导出失败，状态: %d", data.Status)
		}

		time.Sleep(5 * time.Second)
	}

	if pickCode == "" {
		return nil, fmt.Errorf("等待目录树导出超时")
	}

	fileData, err := client.DownloadDirectoryTreeFile(pickCode, cloud115.ID, cloud115.Cookie)
	if err != nil {
		return nil, fmt.Errorf("下载目录树文件失败: %v", err)
	}

	entries, err := Parse115DirTreeFile(fileData)
	if err != nil {
		return nil, fmt.Errorf("解析目录树文件失败: %v", err)
	}

	dirTree := BuildTreeFromExport(entries, cid, rootName)

	extensions := strings.Split(strmConfig.Extension, ",")
	targetExts := make([]string, 0)
	for _, ext := range extensions {
		ext = strings.TrimSpace(ext)
		if ext != "" {
			targetExts = append(targetExts, strings.ToLower(strings.TrimPrefix(ext, ".")))
		}
	}

	collection := &VideoCollection{
		Videos: []VideoFile{},
	}
	extractVideoFiles(dirTree, "", strmConfig.NetDiskPath, cid, targetExts, collection, strmConfig.Cloud115Id, true)

	result.Total = len(collection.Videos)
	Info("[cron] Found %d files for full STRM generation", result.Total)
	UpdateTaskProgress(taskID, result.Total, 0, 0, 0)

	// 全量生成必须先清空目标目录，避免已从网盘移除的旧文件继续残留。
	generator := NewStrmGeneratorWithServer(strmConfig.LocalPath, GetConfig().ServerURL, ".strm")
	if err := generator.CleanupStrmFiles(); err != nil {
		return nil, fmt.Errorf("清空STRM目标目录失败: %v", err)
	}

	if err := DeleteStrmFilesByConfigID(strmConfig.ID); err != nil {
		Warn("[cron] Failed to delete existing STRM file records: %v", err)
	}

	if result.Total == 0 {
		return result, nil
	}

	processedFiles := 0
	successFiles := 0
	failedFiles := 0

	for i, video := range collection.Videos {
		// 协作式取消检查：每次迭代前检查取消标记
		if IsTaskCancelled(taskID) {
			Info("[cron] 任务已被取消: %s, 已处理 %d/%d", taskID, i, result.Total)
			return result, nil
		}

		// 恢复时跳过已处理的文件
		fileID := video.PickCode
		if fileID == "" {
			fileID = video.Sha1
		}
		if fileID != "" && IsFileProcessed(taskID, fileID) {
			Debug("[cron] 跳过已处理文件: %s", video.Name)
			continue
		}

		localStrmPath, err := generator.GenerateSingleStrmFile(video, strmConfig.NetDiskPath)
		processedFiles++
		if err != nil {
			failedFiles++
			UpdateTaskProgress(taskID, result.Total, processedFiles, successFiles, failedFiles)
			Warn("[cron] 生成STRM文件失败 %s: %v", video.Name, err)
			continue
		}

		UpsertStrmFile(strmConfig.ID, video.Name, video.RelativePath, video.PickCode, video.Sha1, int64(video.Size), localStrmPath)
		successFiles++
		UpdateTaskProgress(taskID, result.Total, processedFiles, successFiles, failedFiles)
		Debug("[cron] 生成STRM文件: %s", localStrmPath)

		// 记录已处理的文件ID，用于恢复时跳过
		if fileID != "" {
			AddProcessedFileID(taskID, fileID)
		}

		if (i+1)%10 == 0 {
			Info("[cron] Full STRM generation progress: %d/%d", i+1, result.Total)
		}
	}

	Info("[cron] Full STRM generation completed: total=%d success=%d failed=%d output=%s", result.Total, successFiles, failedFiles, strmConfig.LocalPath)
	return result, nil
}
