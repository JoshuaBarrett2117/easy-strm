package main

import (
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

// CreateStrmConfig 创建STRM配置

func CreateStrmConfig(cloud115Id int, netDiskPath, localPath, cron, extension string, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfig, error) {
	Debug("Creating new strm config")
	strmConfig := &StrmConfig{}
	if syncMode == "" {
		syncMode = "manual"
	}
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}
	err := db.QueryRow(
		`INSERT INTO t_strm_config (cloud115_id, net_disk_path, local_path, cron, extension, sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, COALESCE(dir_tree_file, ''), sync_mode,
		source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency,
	).Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode, &strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory, &strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy, &strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to create strm config: %v", err)
		return nil, err
	}
	Info("Created new strm config (ID: %d)", strmConfig.ID)

	if cron != "" {
		taskName := buildFullGenerateCronTaskName(strmConfig.ID)
		cronTask, err := CreateCronTask(taskName, "full_generate", cloud115Id, strmConfig.ID, cron)
		if err != nil {
			Warn("Failed to create cron task for strm config: %v", err)
		} else {
			Info("Created cron task (ID: %d) for strm config (ID: %d)", cronTask.ID, strmConfig.ID)
			if scheduler != nil {
				if err := scheduler.AddTask(cronTask); err != nil {
					Warn("Failed to add cron task to scheduler: %v", err)
				}
			}
		}
	}

	return strmConfig, nil
}

// UpdateStrmConfig 更新STRM配置

func UpdateStrmConfig(id, cloud115Id int, netDiskPath, localPath, cron, extension string, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfig, error) {
	Debug("Updating strm config with ID: %d", id)
	strmConfig := &StrmConfig{}
	if syncMode == "" {
		syncMode = "manual"
	}
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}
	err := db.QueryRow(
		`UPDATE t_strm_config SET cloud115_id = $1, net_disk_path = $2, local_path = $3, cron = $4, extension = $5, sync_mode = $6, source_account = $7, target_account = $8, target_directory = $9, auto_cleanup = $10, cleanup_threshold = $11, cleanup_policy = $12, max_concurrency = $13 WHERE id = $14
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, COALESCE(dir_tree_file, ''), sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency, id,
	).Scan(&strmConfig.ID, &strmConfig.Cloud115Id, &strmConfig.NetDiskPath, &strmConfig.LocalPath, &strmConfig.Cron, &strmConfig.Extension, &strmConfig.DirTreeFile, &strmConfig.SyncMode, &strmConfig.SourceAccount, &strmConfig.TargetAccount, &strmConfig.TargetDirectory, &strmConfig.AutoCleanup, &strmConfig.CleanupThreshold, &strmConfig.CleanupPolicy, &strmConfig.MaxConcurrency, &strmConfig.CreateTime, &strmConfig.UpdateTime)
	if err != nil {
		Error("Failed to update strm config with ID %d: %v", id, err)
		return nil, err
	}
	Info("Updated strm config (ID: %d)", strmConfig.ID)

	existingTask, _ := GetCronTaskByStrmConfigID(strmConfig.ID)

	if cron != "" {
		taskName := buildFullGenerateCronTaskName(strmConfig.ID)
		if existingTask != nil {
			_, err = UpdateCronTask(existingTask.ID, taskName, "full_generate", cron, existingTask.Status)
			if err != nil {
				Warn("Failed to update cron task: %v", err)
			} else {
				if scheduler != nil {
					updatedTask, _ := GetCronTaskByID(existingTask.ID)
					if updatedTask != nil {
						if err := scheduler.UpdateTask(updatedTask); err != nil {
							Warn("Failed to update cron task in scheduler: %v", err)
						} else {
							// 重新获取任务数据，因为 scheduler.UpdateTask 会更新 next_run_time
							if finalTask, err := GetCronTaskByID(existingTask.ID); err == nil {
								Debug("Cron task next run time updated: %v", finalTask.NextRunTime)
							}
						}
					}
				}
			}
		} else {
			cronTask, err := CreateCronTask(taskName, "full_generate", cloud115Id, strmConfig.ID, cron)
			if err != nil {
				Warn("Failed to create cron task: %v", err)
			} else {
				Info("Created cron task (ID: %d) for strm config(ID: %d)", cronTask.ID, strmConfig.ID)
				if scheduler != nil {
					if err := scheduler.AddTask(cronTask); err != nil {
						Warn("Failed to add cron task to scheduler: %v", err)
					}
				}
			}
		}
	} else {
		if existingTask != nil {
			if scheduler != nil {
				scheduler.RemoveTask(existingTask.ID)
			}
			err := DeleteCronTaskByName(existingTask.TaskName)
			if err != nil {
				Warn("Failed to delete cron task: %v", err)
			}
		}
	}

	return strmConfig, nil
}

// DeleteStrmConfig 删除STRM配置

func DeleteStrmConfig(id int) error {
	Debug("Deleting strm config with ID: %d", id)

	existingTask, _ := GetCronTaskByStrmConfigID(id)
	if existingTask != nil {
		if scheduler != nil {
			scheduler.RemoveTask(existingTask.ID)
		}
		err := DeleteCronTaskByName(existingTask.TaskName)
		if err != nil {
			Warn("Failed to delete cron task: %v", err)
		}
	}

	result, err := db.Exec("DELETE FROM t_strm_config WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete strm config with ID %d: %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		Error("Failed to get rows affected for delete operation: %v", err)
		return err
	}

	if rowsAffected == 0 {
		Debug("No strm config found with ID %d for deletion", id)
		return fmt.Errorf("no strm config found with ID %d", id)
	}

	Info("Deleted strm config with ID: %d", id)
	return nil
}
