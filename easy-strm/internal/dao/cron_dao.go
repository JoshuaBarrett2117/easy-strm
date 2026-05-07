package dao

import (
	"database/sql"
	"fmt"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// CronTaskDAO 定时任务数据访问层
type CronTaskDAO struct{}

// NewCronTaskDAO 创建定时任务DAO实例
func NewCronTaskDAO() *CronTaskDAO {
	return &CronTaskDAO{}
}

// GetByID 根据ID获取定时任务
func (c *CronTaskDAO) GetByID(id int) (*domain.CronTask, error) {
	task := &domain.CronTask{}
	err := db.QueryRow(
		`SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task WHERE id = $1`,
		id,
	).Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID,
		&task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime,
		&task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("CronTaskDAO[GetByID] 查询失败: %v", err)
	}
	return task, nil
}

// GetByName 根据任务名获取定时任务
func (c *CronTaskDAO) GetByName(taskName string) (*domain.CronTask, error) {
	task := &domain.CronTask{}
	err := db.QueryRow(
		`SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task WHERE task_name = $1`,
		taskName,
	).Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID,
		&task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime,
		&task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("CronTaskDAO[GetByName] 查询失败: %v", err)
	}
	return task, nil
}

// GetByStrmConfigID 根据STRM配置ID获取定时任务
func (c *CronTaskDAO) GetByStrmConfigID(strmConfigID int) (*domain.CronTask, error) {
	task := &domain.CronTask{}
	err := db.QueryRow(
		`SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task WHERE strm_config_id = $1`,
		strmConfigID,
	).Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID,
		&task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime,
		&task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("CronTaskDAO[GetByStrmConfigID] 查询失败: %v", err)
	}
	return task, nil
}

// GetAll 获取所有定时任务
func (c *CronTaskDAO) GetAll() ([]*domain.CronTask, error) {
	rows, err := db.Query(
		`SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("CronTaskDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.CronTask
	for rows.Next() {
		task := &domain.CronTask{}
		err := rows.Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID,
			&task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime,
			&task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
		if err != nil {
			return nil, fmt.Errorf("CronTaskDAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, task)
	}
	return list, nil
}

// GetEnabled 获取所有启用的定时任务
func (c *CronTaskDAO) GetEnabled() ([]*domain.CronTask, error) {
	rows, err := db.Query(
		`SELECT id, task_name, task_type, cloud115_id, strm_config_id, cron_expr, status,
		last_run_time, next_run_time, COALESCE(last_run_status, ''), COALESCE(last_run_message, ''),
		create_time, update_time FROM t_cron_task WHERE status = 'enabled'`)
	if err != nil {
		return nil, fmt.Errorf("CronTaskDAO[GetEnabled] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.CronTask
	for rows.Next() {
		task := &domain.CronTask{}
		err := rows.Scan(&task.ID, &task.TaskName, &task.TaskType, &task.Cloud115ID, &task.StrmConfigID,
			&task.CronExpr, &task.Status, &task.LastRunTime, &task.NextRunTime,
			&task.LastRunStatus, &task.LastRunMessage, &task.CreateTime, &task.UpdateTime)
		if err != nil {
			return nil, fmt.Errorf("CronTaskDAO[GetEnabled] 扫描失败: %v", err)
		}
		list = append(list, task)
	}
	return list, nil
}

// Create 创建定时任务
func (c *CronTaskDAO) Create(taskName, taskType string, cloud115ID, strmConfigID int, cronExpr string) (*domain.CronTask, error) {
	var taskID int
	err := db.QueryRow(
		`INSERT INTO t_cron_task (task_name, task_type, cloud115_id, strm_config_id, cron_expr, status)
		VALUES ($1, $2, $3, $4, $5, 'enabled')
		RETURNING id`,
		taskName, taskType, cloud115ID, strmConfigID, cronExpr,
	).Scan(&taskID)
	if err != nil {
		return nil, fmt.Errorf("CronTaskDAO[Create] 创建失败: %v", err)
	}
	task, err := c.GetByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("CronTaskDAO[Create] 回读失败: %v", err)
	}
	logger.Infof("CronTaskDAO[Create] 创建定时任务成功: %s (ID: %d)", taskName, task.ID)
	return task, nil
}

// Update 更新定时任务
func (c *CronTaskDAO) Update(id int, taskName, taskType, cronExpr, status string) (*domain.CronTask, error) {
	var taskID int
	err := db.QueryRow(
		`UPDATE t_cron_task SET task_name=$1, task_type=$2, cron_expr=$3, status=$4 WHERE id=$5
		RETURNING id`,
		taskName, taskType, cronExpr, status, id,
	).Scan(&taskID)
	if err != nil {
		return nil, fmt.Errorf("CronTaskDAO[Update] 更新失败: %v", err)
	}
	task, err := c.GetByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("CronTaskDAO[Update] 回读失败: %v", err)
	}
	return task, nil
}

// UpdateRunInfo 更新定时任务执行信息
func (c *CronTaskDAO) UpdateRunInfo(id int, lastRunTime, nextRunTime *time.Time, lastRunStatus, lastRunMessage string) error {
	_, err := db.Exec(
		`UPDATE t_cron_task SET last_run_time=$1, next_run_time=$2, last_run_status=$3, last_run_message=$4 WHERE id=$5`,
		lastRunTime, nextRunTime, lastRunStatus, lastRunMessage, id,
	)
	if err != nil {
		return fmt.Errorf("CronTaskDAO[UpdateRunInfo] 更新失败: %v", err)
	}
	return nil
}

// Delete 删除定时任务
func (c *CronTaskDAO) Delete(id int) error {
	result, err := db.Exec("DELETE FROM t_cron_task WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("CronTaskDAO[Delete] 删除失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("CronTaskDAO[Delete] 任务不存在")
	}
	return nil
}

// DeleteByName 根据任务名删除定时任务
func (c *CronTaskDAO) DeleteByName(taskName string) error {
	_, err := db.Exec("DELETE FROM t_cron_task WHERE task_name = $1", taskName)
	if err != nil {
		return fmt.Errorf("CronTaskDAO[DeleteByName] 删除失败: %v", err)
	}
	return nil
}
