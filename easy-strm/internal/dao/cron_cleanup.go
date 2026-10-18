package dao

import (
	"database/sql"
	"easy-strm/internal/domain"
	"fmt"
)

// CleanupTargets 返回任务的全部目标，包含停用目标以支持编辑回显。
func (d *CronTaskDAO) CleanupTargets(id int) ([]domain.CronCleanupTarget, error) {
	rows, err := db.Query(`SELECT id,cron_task_id,cloud115_id,dir_path,COALESCE(dir_cid,''),enabled,create_time,update_time FROM t_cron_cleanup_target WHERE cron_task_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.CronCleanupTarget{}
	for rows.Next() {
		var target domain.CronCleanupTarget
		if err := rows.Scan(&target.ID, &target.CronTaskID, &target.Cloud115ID, &target.DirPath, &target.DirCID, &target.Enabled, &target.CreateTime, &target.UpdateTime); err != nil {
			return nil, err
		}
		result = append(result, target)
	}
	return result, rows.Err()
}

// ValidateCleanupAccount 只检查账号存在，不读取凭据或调用115。
func (d *CronTaskDAO) ValidateCleanupAccount(id int) error {
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM t_cloud_115 WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("115账号不存在: %d", id)
	}
	return nil
}

func lockCleanupDefinition(tx *sql.Tx, id int) error {
	// 配合v48唯一索引，在并发创建和更新时返回可读错误。
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(115,48)`); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM t_cron_task WHERE handler='cleanup_directories' AND id<>$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("全局只能有一个定时清空目录任务，请编辑已有任务")
	}
	return nil
}

func saveCleanupTargets(tx *sql.Tx, task *domain.CronTask) error {
	if _, err := tx.Exec(`DELETE FROM t_cron_cleanup_target WHERE cron_task_id=$1`, task.ID); err != nil {
		return err
	}
	for i := range task.CleanupTargets {
		target := &task.CleanupTargets[i]
		// 不保存调用方的CID，避免后续将缓存误当作删除授权。
		target.DirCID = ""
		target.CronTaskID = task.ID
		err := tx.QueryRow(`INSERT INTO t_cron_cleanup_target(cron_task_id,cloud115_id,dir_path,enabled) VALUES($1,$2,$3,$4) RETURNING id,create_time,update_time`, task.ID, target.Cloud115ID, target.DirPath, target.Enabled).Scan(&target.ID, &target.CreateTime, &target.UpdateTime)
		if err != nil {
			return err
		}
	}
	return nil
}
