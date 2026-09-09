package main

import (
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"fmt"
)

func buildFullGenerateCronTaskName(strmConfigID int) string {
	return fmt.Sprintf("STRM全量生成-%d", strmConfigID)
}

func GetStrmConfigByID(id int) (*StrmConfig, error) {
	Debug("Getting strm config by ID: %d", id)
	strmConfig := &StrmConfig{}
	err := db.QueryRow(`SELECT id, cloud115_id, net_disk_path, local_path, cron, extension,
		COALESCE(dir_tree_file, ''), COALESCE(sync_mode, 'manual'), COALESCE(source_account, 0),
		COALESCE(target_account, 0), COALESCE(target_directory, ''), auto_cleanup,
		COALESCE(cleanup_threshold, 0), COALESCE(cleanup_policy, ''), COALESCE(max_concurrency, 1),
		create_time, update_time FROM t_strm_config WHERE id = $1`, id).Scan(
		&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath,
		&strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode,
		&strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory,
		&strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy,
		&strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to get strm config by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found strm config by ID %d", id)
	return strmConfig, nil
}

// GetAllStrmConfig 获取所有STRM配置

func GetAllStrmConfig(sortField, sortOrder string) ([]*StrmConfig, error) {
	Debug("Getting all strm configs with sort: %s %s", sortField, sortOrder)

	if sortField == "" {
		sortField = "id"
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}

	query := fmt.Sprintf(`SELECT id, cloud115_id, net_disk_path, local_path, cron, extension,
		COALESCE(dir_tree_file, ''), COALESCE(sync_mode, 'manual'), COALESCE(source_account, 0),
		COALESCE(target_account, 0), COALESCE(target_directory, ''), auto_cleanup,
		COALESCE(cleanup_threshold, 0), COALESCE(cleanup_policy, ''), COALESCE(max_concurrency, 1),
		create_time, update_time FROM t_strm_config ORDER BY %s %s`, sortField, sortOrder)
	rows, err := db.Query(query)
	if err != nil {
		Error("Failed to get all strm configs: %v", err)
		return nil, err
	}
	defer rows.Close()

	var strmConfigList []*StrmConfig
	for rows.Next() {
		strmConfig := &StrmConfig{}
		err := rows.Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath,
			&strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode,
			&strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory,
			&strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy,
			&strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
		if err != nil {
			Error("Failed to scan strm config row: %v", err)
			return nil, err
		}
		strmConfigList = append(strmConfigList, strmConfig)
	}

	if err = rows.Err(); err != nil {
		Error("Error iterating strm config rows: %v", err)
		return nil, err
	}

	Debug("Found %d strm configs", len(strmConfigList))
	return strmConfigList, nil
}

// CreateStrmConfig 将旧入口转发给统一Service，避免配置与任务分开提交。
func CreateStrmConfig(cloud int, netPath, localPath, expr, extension, mode string, source, target int, targetDir string, cleanup bool, threshold int, policy string, concurrency int) (*StrmConfig, error) {
	return strmWriteService().CreateConfigExt(cloud, netPath, localPath, expr, extension, mode, source, target, targetDir, cleanup, threshold, policy, concurrency)
}

// UpdateStrmConfig 将旧入口转发给统一Service。
func UpdateStrmConfig(id, cloud int, netPath, localPath, expr, extension, mode string, source, target int, targetDir string, cleanup bool, threshold int, policy string, concurrency int) (*StrmConfig, error) {
	return strmWriteService().UpdateConfigExt(id, cloud, netPath, localPath, expr, extension, mode, source, target, targetDir, cleanup, threshold, policy, concurrency)
}

// DeleteStrmConfig 删除配置及全部关联调度。
func DeleteStrmConfig(id int) error { return strmWriteService().DeleteConfig(id) }
func strmWriteService() *service.StrmService {
	s := service.NewStrmService(dao.NewStrmConfigDAO(), dao.NewStrmFileDAO(), dao.NewCronTaskDAO())
	s.SetScheduler(scheduler)
	return s
}
