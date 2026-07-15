package main

import (
	"fmt"
	"time"
)

func GetCronTaskByID(id int) (*CronTask, error) {
	Debug("Getting cron task by ID: %d", id)
	task := &CronTask{}
	err := db.QueryRow("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE id = $1", id).Scan(
		&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		Error("Failed to get cron task by ID %d: %v", id, err)
		return nil, err
	}
	return task, nil
}

// GetCronTaskByName 根据任务名获取定时任务

func GetCronTaskByName(taskName string) (*CronTask, error) {
	Debug("Getting cron task by name: %s", taskName)
	task := &CronTask{}
	err := db.QueryRow("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE task_name = $1", taskName).Scan(
		&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// GetCronTaskByStrmConfigID 根据STRM配置ID获取定时任务

func GetCronTaskByStrmConfigID(strmConfigID int) (*CronTask, error) {
	Debug("Getting cron task by strm config ID: %d", strmConfigID)
	task := &CronTask{}
	err := db.QueryRow("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE strm_config_id = $1", strmConfigID).Scan(
		&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// GetAllCronTasks 获取所有定时任务

func GetAllCronTasks() ([]*CronTask, error) {
	Debug("Getting all cron tasks")
	rows, err := db.Query("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task ORDER BY id")
	if err != nil {
		Error("Failed to get all cron tasks: %v", err)
		return nil, err
	}
	defer rows.Close()

	var taskList []*CronTask
	for rows.Next() {
		task := &CronTask{}
		err := rows.Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
		if err != nil {
			Error("Failed to scan cron task row: %v", err)
			return nil, err
		}
		taskList = append(taskList, task)
	}

	return taskList, nil
}

// GetEnabledCronTasks 获取所有启用的定时任务

func GetEnabledCronTasks() ([]*CronTask, error) {
	Debug("Getting enabled cron tasks")
	rows, err := db.Query("SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status, last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''), create_time, update_time FROM t_cron_task WHERE status = 'enabled'")
	if err != nil {
		Error("Failed to get enabled cron tasks: %v", err)
		return nil, err
	}
	defer rows.Close()

	var taskList []*CronTask
	for rows.Next() {
		task := &CronTask{}
		err := rows.Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID, &task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime, &task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
		if err != nil {
			Error("Failed to scan cron task row: %v", err)
			return nil, err
		}
		taskList = append(taskList, task)
	}

	return taskList, nil
}

// CreateCronTask 创建定时任务

func CreateCronTask(taskName, taskType string, cloud115ID, strmConfigID int, cronExpr string) (*CronTask, error) {
	Debug("Creating cron task: %s", taskName)
	var taskID int
	err := db.QueryRow(
		"INSERT INTO t_cron_task (task_name, task_type, cloud115_id, strm_config_id, cron_expr, status) VALUES ($1, $2, $3, $4, $5, 'enabled') RETURNING id",
		taskName, taskType, cloud115ID, strmConfigID, cronExpr,
	).Scan(&taskID)
	if err != nil {
		Error("Failed to create cron task: %v", err)
		return nil, err
	}
	task, err := GetCronTaskByID(taskID)
	if err != nil {
		Error("Failed to reload cron task after create: %v", err)
		return nil, err
	}
	Info("Created cron task: %s (ID: %d)", taskName, task.ID)
	return task, nil
}

// UpdateCronTask 更新定时任务

func UpdateCronTask(id int, taskName, taskType, cronExpr, status string) (*CronTask, error) {
	Debug("Updating cron task with ID: %d", id)
	var taskID int
	err := db.QueryRow(
		"UPDATE t_cron_task SET task_name = $1, task_type = $2, cron_expr = $3, status = $4 WHERE id = $5 RETURNING id",
		taskName, taskType, cronExpr, status, id,
	).Scan(&taskID)
	if err != nil {
		Error("Failed to update cron task with ID %d: %v", id, err)
		return nil, err
	}
	task, err := GetCronTaskByID(taskID)
	if err != nil {
		Error("Failed to reload cron task after update: %v", err)
		return nil, err
	}
	Info("Updated cron task (ID: %d)", task.ID)
	return task, nil
}

// UpdateCronTaskRunInfo 更新定时任务执行信息

func UpdateCronTaskRunInfo(id int, lastRunTime, nextRunTime *time.Time, lastRunStatus, lastRunMessage string) error {
	Debug("Updating cron task run info for ID: %d", id)
	_, err := db.Exec(
		"UPDATE t_cron_task SET last_run_time = $1, next_run_time = $2, last_run_status = $3, last_run_message = $4 WHERE id = $5",
		lastRunTime, nextRunTime, lastRunStatus, lastRunMessage, id,
	)
	if err != nil {
		Error("Failed to update cron task run info: %v", err)
		return err
	}
	return nil
}

// DeleteCronTask 删除定时任务

func DeleteCronTask(id int) error {
	Debug("Deleting cron task with ID: %d", id)
	result, err := db.Exec("DELETE FROM t_cron_task WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete cron task with ID %d: %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no cron task found with ID %d", id)
	}

	Info("Deleted cron task with ID: %d", id)
	return nil
}

// DeleteCronTaskByName 根据任务名删除定时任务

func DeleteCronTaskByName(taskName string) error {
	Debug("Deleting cron task by name: %s", taskName)
	_, err := db.Exec("DELETE FROM t_cron_task WHERE task_name = $1", taskName)
	if err != nil {
		Error("Failed to delete cron task by name: %v", err)
		return err
	}
	return nil
}
