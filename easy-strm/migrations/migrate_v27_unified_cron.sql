-- 更新日期：2026-09-08；执行者：Codex
ALTER TABLE t_cron_task ADD COLUMN IF NOT EXISTS task_key TEXT;
ALTER TABLE t_cron_task ADD COLUMN IF NOT EXISTS handler TEXT NOT NULL DEFAULT '';
ALTER TABLE t_cron_task ADD COLUMN IF NOT EXISTS params JSONB NOT NULL DEFAULT '{}';
ALTER TABLE t_cron_task ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Local';
ALTER TABLE t_cron_task ADD COLUMN IF NOT EXISTS builtin BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE t_cron_task ALTER COLUMN cloud115_id DROP NOT NULL;
ALTER TABLE t_cron_task ALTER COLUMN strm_config_id DROP NOT NULL;
UPDATE t_cron_task SET task_key='legacy:'||id WHERE task_key IS NULL;
UPDATE t_cron_task SET handler=task_type,params=jsonb_build_object('strm_config_id',strm_config_id,'cloud115_id',cloud115_id) WHERE handler='';
CREATE UNIQUE INDEX IF NOT EXISTS idx_cron_task_key ON t_cron_task(task_key);
INSERT INTO t_cron_task(task_key,task_name,task_type,handler,cron_expr,status,builtin,params) VALUES
 ('system:cooling','账号冷却恢复','cooling_recovery','cooling_recovery','0 * * * * *','enabled',true,'{}'),
 ('system:logs','日志清理','log_cleanup','log_cleanup','0 0 * * * *','enabled',true,'{}'),
 ('system:identify-cache','识别缓存清理','identify_cache_cleanup','identify_cache_cleanup','0 0 3 * * *','enabled',true,'{"keep_days":30}')
 ON CONFLICT(task_key) DO NOTHING;
CREATE TABLE IF NOT EXISTS t_cron_task_run (
 id BIGSERIAL PRIMARY KEY, cron_task_id INTEGER REFERENCES t_cron_task(id) ON DELETE SET NULL,
 task_id TEXT NOT NULL UNIQUE, trigger_type TEXT NOT NULL, started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 ended_at TIMESTAMPTZ, status TEXT NOT NULL, message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_cron_run_task ON t_cron_task_run(cron_task_id,id DESC);
