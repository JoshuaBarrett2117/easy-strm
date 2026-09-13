package dao

import (
	"database/sql"
	"easy-strm/internal/domain"
	"encoding/json"
	"fmt"
	"time"
)

// Hydrate 补充通用字段，供旧STRM入口复用。
func (d *CronTaskDAO) Hydrate(t *domain.CronTask) error {
	var raw []byte
	err := db.QueryRow(`SELECT COALESCE(task_key,''),handler,params,timezone,builtin FROM t_cron_task WHERE id=$1`, t.ID).Scan(&t.TaskKey, &t.Handler, &raw, &t.Timezone, &t.Builtin)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &t.Params)
}

// SaveDefinition 保存通用配置，并保持STRM页面的关联配置一致。
func (d *CronTaskDAO) SaveDefinition(t *domain.CronTask) error {
	raw, err := json.Marshal(t.Params)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var config, cloud interface{}
	if t.StrmConfigID > 0 {
		config = t.StrmConfigID
		cloud = t.Cloud115ID
	}
	if t.ID == 0 {
		err = tx.QueryRow(`INSERT INTO t_cron_task(task_key,task_name,task_type,handler,params,timezone,builtin,cloud115_id,strm_config_id,cron_expr,status) VALUES($1,$2,$3,$4,$5,$6,false,$7,$8,$9,$10) RETURNING id`, t.TaskKey, t.TaskName, t.Handler, t.Handler, raw, t.Timezone, cloud, config, t.CronExpr, t.Status).Scan(&t.ID)
	} else {
		// 更换处理器或绑定配置时，清除原配置的全量周期镜像。
		_, err = tx.Exec(`UPDATE t_strm_config SET cron='' WHERE id IN (SELECT strm_config_id FROM t_cron_task WHERE id=$1 AND handler='full_generate' AND (handler<>$2 OR strm_config_id IS DISTINCT FROM $3::integer))`, t.ID, t.Handler, config)
		if err != nil {
			return err
		}
		var result sql.Result
		result, err = tx.Exec(`UPDATE t_cron_task SET task_name=$1,task_type=$2,handler=$3,params=$4,timezone=$5,cloud115_id=$6,strm_config_id=$7,cron_expr=$8,status=$9 WHERE id=$10`, t.TaskName, t.Handler, t.Handler, raw, t.Timezone, cloud, config, t.CronExpr, t.Status, t.ID)
		if err == nil {
			n, e := result.RowsAffected()
			err = e
			if n == 0 && err == nil {
				err = sql.ErrNoRows
			}
		}
	}
	if err != nil {
		return err
	}
	if t.Handler == "full_generate" {
		_, err = tx.Exec(`UPDATE t_strm_config SET cron=$1 WHERE id=$2`, t.CronExpr, t.StrmConfigID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ValidateStrm 检查任务关联的配置与账号。
func (d *CronTaskDAO) ValidateStrm(config, cloud int) error {
	var found int
	err := db.QueryRow(`SELECT id FROM t_strm_config WHERE id=$1 AND cloud115_id=$2`, config, cloud).Scan(&found)
	if err != nil {
		return fmt.Errorf("STRM配置不存在或不属于所选账号: %w", err)
	}
	return nil
}

// StartRun 保存每次触发，包括因并发而跳过的记录。
func (d *CronTaskDAO) StartRun(id int, taskID, trigger, status string) error {
	_, err := db.Exec(`INSERT INTO t_cron_task_run(cron_task_id,task_id,trigger_type,status) VALUES($1,$2,$3,$4)`, id, taskID, trigger, status)
	return err
}

// FinishRun 保存执行终态。
func (d *CronTaskDAO) FinishRun(taskID, status, message string) error {
	_, err := db.Exec(`UPDATE t_cron_task_run SET status=$1,message=$2,ended_at=now() WHERE task_id=$3`, status, message, taskID)
	return err
}

// MarkRunStarted 区分排队记录与已经进入处理器的运行记录。
func (d *CronTaskDAO) MarkRunStarted(taskID string) error {
	_, err := db.Exec(`UPDATE t_cron_task_run SET status='running' WHERE task_id=$1`, taskID)
	return err
}

// UpdateNextRun 只更新计划时间，避免编辑任务覆盖并发执行的结果。
func (d *CronTaskDAO) UpdateNextRun(id int, next *time.Time) error {
	_, err := db.Exec(`UPDATE t_cron_task SET next_run_time=$1 WHERE id=$2`, next, id)
	return err
}

// RecoverRuns 重启不补跑，将未完成的执行标记为中断。
func (d *CronTaskDAO) RecoverRuns() error {
	_, err := db.Exec(`WITH interrupted AS (UPDATE t_cron_task_run SET status='interrupted',message='服务重启导致执行中断',ended_at=now() WHERE status IN ('running','pending') RETURNING cron_task_id) UPDATE t_cron_task SET last_run_status='interrupted',last_run_message='服务重启导致执行中断' WHERE id IN(SELECT cron_task_id FROM interrupted)`)
	return err
}

// Runs 返回某个调度的执行历史。
func (d *CronTaskDAO) Runs(id, page, size int) (domain.ShareLibraryPage, error) {
	var raw []byte
	out := domain.ShareLibraryPage{}
	err := db.QueryRow(`WITH runs AS(SELECT id,task_id,trigger_type,started_at,ended_at,status,message FROM t_cron_task_run WHERE cron_task_id=$1) SELECT (SELECT count(*) FROM runs),COALESCE((SELECT json_agg(p) FROM(SELECT * FROM runs ORDER BY id DESC LIMIT $2 OFFSET $3)p),'[]'::json)`, id, size, (page-1)*size).Scan(&out.Total, &raw)
	out.Data = json.RawMessage(raw)
	return out, err
}
