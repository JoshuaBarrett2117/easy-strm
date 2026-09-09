package dao

import (
	"database/sql"
	"easy-strm/internal/domain"
	"fmt"
)

// SaveWithSchedule 在同一事务保存STRM配置及其全量调度，返回需要移除的旧调度ID。
func (d *StrmConfigDAO) SaveWithSchedule(c *domain.StrmConfig) ([]int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	args := []interface{}{c.Cloud115Id, c.NetDiskPath, c.LocalPath, c.Cron, c.Extension, c.SyncMode, c.SourceAccount, c.TargetAccount, c.TargetDirectory, c.AutoCleanup, c.CleanupThreshold, c.CleanupPolicy, c.MaxConcurrency}
	query := `INSERT INTO t_strm_config(cloud115_id,net_disk_path,local_path,cron,extension,sync_mode,source_account,target_account,target_directory,auto_cleanup,cleanup_threshold,cleanup_policy,max_concurrency) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	if c.ID > 0 {
		query = `UPDATE t_strm_config SET cloud115_id=$1,net_disk_path=$2,local_path=$3,cron=$4,extension=$5,sync_mode=$6,source_account=$7,target_account=$8,target_directory=$9,auto_cleanup=$10,cleanup_threshold=$11,cleanup_policy=$12,max_concurrency=$13 WHERE id=$14`
		args = append(args, c.ID)
	}
	err = tx.QueryRow(query+` RETURNING id,COALESCE(dir_tree_file,''),create_time,update_time`, args...).Scan(&c.ID, &c.DirTreeFile, &c.CreateTime, &c.UpdateTime)
	if err != nil {
		return nil, err
	}
	retired := []int{}
	if c.Cron == "" {
		rows, e := tx.Query(`DELETE FROM t_cron_task WHERE strm_config_id=$1 AND task_type='full_generate' RETURNING id`, c.ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id int
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return nil, e
			}
			retired = append(retired, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	} else {
		_, err = tx.Exec(`INSERT INTO t_cron_task(task_key,task_name,task_type,handler,params,timezone,cloud115_id,strm_config_id,cron_expr,status)
   VALUES('task:'||md5(random()::text||clock_timestamp()::text),$2,'full_generate','full_generate',jsonb_build_object('strm_config_id',$1::integer,'cloud115_id',$3::integer),'Local',$3,$1,$4,'enabled')
   ON CONFLICT(strm_config_id,task_type) DO UPDATE SET cloud115_id=EXCLUDED.cloud115_id,params=EXCLUDED.params,handler=EXCLUDED.handler,cron_expr=EXCLUDED.cron_expr`, c.ID, fmt.Sprintf("STRM全量生成-%d", c.ID), c.Cloud115Id, c.Cron)
		if err != nil {
			return nil, err
		}
	}
	// 配置更换账号时，增量调度同步关联账号；保留其周期、启停与展示名称。
	_, err = tx.Exec(`UPDATE t_cron_task SET cloud115_id=$1,params=jsonb_set(params,'{cloud115_id}',to_jsonb($1::integer)) WHERE strm_config_id=$2`, c.Cloud115Id, c.ID)
	if err != nil {
		return nil, err
	}
	return retired, tx.Commit()
}

// DeleteWithSchedules 原子删除配置，级联删除调度，返回所有需卸载的调度ID。
func (d *StrmConfigDAO) DeleteWithSchedules(id int) ([]int, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var locked int
	if err = tx.QueryRow(`SELECT id FROM t_strm_config WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT id FROM t_cron_task WHERE strm_config_id=$1`, id)
	if err != nil {
		return nil, err
	}
	ids := []int{}
	for rows.Next() {
		var v int
		if err = rows.Scan(&v); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result, err := tx.Exec(`DELETE FROM t_strm_config WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, sql.ErrNoRows
	}
	return ids, tx.Commit()
}

// GetByConfig 返回指定配置的全量、增量调度ID。
func (d *CronTaskDAO) GetByConfig(id int) ([]int, error) {
	rows, e := db.Query(`SELECT id FROM t_cron_task WHERE strm_config_id=$1 ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var v int
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		ids = append(ids, v)
	}
	return ids, rows.Err()
}
