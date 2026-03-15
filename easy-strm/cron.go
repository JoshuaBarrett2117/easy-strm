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

	if err := LoadCronTasksFromDB(); err != nil {
		Error("Failed to load cron tasks from database: %v", err)
		return err
	}

	return nil
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

	entryID, err := s.cron.AddFunc(task.CronExpr, func() {
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
	Info("Executing cron task: %s (ID: %d)", task.TaskName, task.ID)

	now := time.Now()
	task.LastRunTime = &now

	taskStatus, err := CreateTask(fmt.Sprintf("cron_%d_%d", task.ID, now.Unix()), TaskTypeIncrementalSync, task.TaskName)
	if err != nil {
		Error("Failed to create task for cron job: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", fmt.Sprintf("创建任务失败: %v", err))
		return
	}

	UpdateTaskStatus(taskStatus.TaskID, TaskStatusRunning)

	strmConfig, err := GetStrmConfigByID(task.StrmConfigID)
	if err != nil {
		Error("Failed to get strm config: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", fmt.Sprintf("获取STRM配置失败: %v", err))
		SetTaskError(taskStatus.TaskID, fmt.Sprintf("获取STRM配置失败: %v", err))
		return
	}

	cloud115, err := GetCloud115ByID(task.Cloud115ID)
	if err != nil {
		Error("Failed to get cloud115 account: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", fmt.Sprintf("获取115账号失败: %v", err))
		SetTaskError(taskStatus.TaskID, fmt.Sprintf("获取115账号失败: %v", err))
		return
	}

	result, err := RunIncrementalSync(strmConfig, cloud115, taskStatus.TaskID)
	if err != nil {
		Error("Incremental sync failed: %v", err)
		UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "failed", err.Error())
		SetTaskError(taskStatus.TaskID, err.Error())
		return
	}

	nextRun := scheduler.GetNextRunTime(task.ID)
	task.NextRunTime = nextRun

	successMsg := fmt.Sprintf("成功: 新增 %d, 删除 %d, 跳过 %d", result.Added, result.Deleted, result.Skipped)
	UpdateCronTaskRunInfo(task.ID, task.LastRunTime, task.NextRunTime, "success", successMsg)
	UpdateTaskStatus(taskStatus.TaskID, TaskStatusCompleted)

	Info("Cron task completed: %s (ID: %d), %s", task.TaskName, task.ID, successMsg)
}

// IncrementalSyncResult 增量同步结果
type IncrementalSyncResult struct {
	Added   int
	Deleted int
	Skipped int
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
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			targetExts = append(targetExts, strings.ToLower(ext))
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
