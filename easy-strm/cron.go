package main

import (
	"context"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CronScheduler 复用业务层统一调度实例。
type CronScheduler = service.CronService

var scheduler *CronScheduler

// InitCronScheduler 注册现有业务与维护方法，路由依赖就绪后启动。
func InitCronScheduler() error {
	scheduler = service.NewCronService(dao.NewCronTaskDAO())
	scheduler.SetTaskService(service.NewTaskService(dao.NewTaskRedisDAOWithGlobal()))
	params := []service.CronParameter{{Key: "cloud115_id", Label: "115账号ID"}, {Key: "strm_config_id", Label: "STRM配置ID"}}
	for _, kind := range []string{"full_generate", "incremental_sync"} {
		name := "STRM全量生成"
		if kind == "incremental_sync" {
			name = "STRM增量同步"
		}
		scheduler.Register(service.CronHandler{Key: kind, Name: name, Parameters: params, Execute: func(ctx context.Context, t *domain.CronTask, id string) (string, error) {
			cfg, e := GetStrmConfigByID(t.StrmConfigID)
			if e != nil {
				return "", e
			}
			if cfg == nil {
				return "", fmt.Errorf("STRM配置不存在")
			}
			cloud, e := GetCloud115ByID(t.Cloud115ID)
			if e != nil {
				return "", e
			}
			if cloud == nil {
				return "", fmt.Errorf("115账号不存在")
			}
			if t.Handler == "full_generate" {
				r, e := RunFullStrmGenerate(cfg, cloud, id)
				if e != nil {
					return "", e
				}
				return fmt.Sprintf("生成%d个STRM文件", r.Total), nil
			}
			r, e := RunIncrementalSync(cfg, cloud, id)
			if e != nil {
				return "", e
			}
			return fmt.Sprintf("新增%d，删除%d，跳过%d", r.Added, r.Deleted, r.Skipped), nil
		}})
	}
	scheduler.Register(service.CronHandler{Key: "log_cleanup", Name: "日志清理", Parameters: []service.CronParameter{}, Execute: func(ctx context.Context, t *domain.CronTask, id string) (string, error) {
		if logger == nil {
			return "", fmt.Errorf("日志服务未初始化")
		}
		days := logger.keepDays
		cfg, e := GetSystemConfigByKey("log_save_day_limit")
		if e != nil {
			return "", e
		}
		if cfg != nil {
			var value int
			if _, e = fmt.Sscanf(cfg.ConfigVal, "%d", &value); e == nil && value > 0 {
				days = value
			}
		}
		return "日志清理完成", cleanOldLogs(logger.logDir, days)
	}})
	scheduler.Register(service.CronHandler{Key: "identify_cache_cleanup", Name: "识别缓存清理", Parameters: []service.CronParameter{{Key: "keep_days", Label: "保留天数", Default: 30}}, Execute: func(ctx context.Context, t *domain.CronTask, id string) (string, error) {
		n, e := dao.NewIdentifyCacheDAO().DeleteOlderThan(service.CronInt(t.Params, "keep_days"))
		return fmt.Sprintf("已清理%d条缓存", n), e
	}})
	return nil
}

// LoadCronTasksFromDB 统一装载任务。
func LoadCronTasksFromDB() error { return scheduler.LoadTasksFromDB() }

// StopScheduler 停止调度。
func StopScheduler() {
	if scheduler != nil {
		scheduler.Stop()
	}
}

// ExecuteCronTask 将旧入口转发给统一执行器。
func ExecuteCronTask(task *CronTask) {
	if _, err := scheduler.Run(task.ID, "manual"); err != nil {
		Error("任务启动失败: %v", err)
	}
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
	release, err := scheduler.AcquireStrmExecution(strmConfig.ID, taskID)
	if err != nil {
		return nil, err
	}
	defer release()
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
		if IsTaskCancelled(taskID) {
			return nil, fmt.Errorf("任务已取消")
		}
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
	release, err := scheduler.AcquireStrmExecution(strmConfig.ID, taskID)
	if err != nil {
		return nil, err
	}
	defer release()
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
		if IsTaskCancelled(taskID) {
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

	result.Total = len(collection.Videos)
	Info("[cron] Found %d files for full STRM generation", result.Total)
	UpdateTaskProgress(taskID, result.Total, 0, 0, 0)

	// 全量生成必须先清空目标目录，避免已从网盘移除的旧文件继续残留。
	generator := NewStrmGeneratorWithServer(strmConfig.LocalPath, GetConfig().ServerURL, ".strm")
	if IsTaskCancelled(taskID) {
		return nil, fmt.Errorf("任务已取消")
	}
	if err := generator.CleanupStrmFiles(); err != nil {
		return nil, fmt.Errorf("清空STRM目标目录失败: %v", err)
	}

	if err := DeleteStrmFilesByConfigID(strmConfig.ID); err != nil {
		return nil, fmt.Errorf("删除旧STRM记录失败: %w", err)
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
	if failedFiles > 0 {
		return result, fmt.Errorf("生成结束：成功%d，失败%d", successFiles, failedFiles)
	}
	return result, nil
}
