-- migrate_v16_cron_task_identity.sql
-- 修复 STRM 全量定时任务名称冲突
-- 更新日期：2026-08-29
-- 执行者：Codex
-- 说明：task_name 改为展示字段；任务身份由 strm_config_id 和 task_type 共同确定。

ALTER TABLE t_cron_task DROP CONSTRAINT IF EXISTS t_cron_task_task_name_key;

UPDATE t_cron_task
SET task_name = 'STRM全量生成-' || strm_config_id::text
WHERE task_type = 'full_generate'
  AND task_name IS DISTINCT FROM 'STRM全量生成-' || strm_config_id::text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_cron_task_config_type
    ON t_cron_task(strm_config_id, task_type);
