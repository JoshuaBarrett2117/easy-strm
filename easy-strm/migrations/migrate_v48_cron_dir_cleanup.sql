-- 更新日期：2026-10-18；维护者：Codex。
-- 执行前先备份并验证可恢复性；本迁移仅由 DBA 手工执行，禁止应用启动执行或生产代码嵌入。
-- 不预置任务；保留已有系统设置和用户任务行。
-- 删除目标表会丢失数据，因此不提供回滚脚本。手工回滚须具备备份、停用任务、回退二进制并经 DBA 审核；保留系统设置和用户任务行。
BEGIN;
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '2min';

DO $pre$
DECLARE
    cleanup_task_ids INTEGER[];
BEGIN
    IF to_regclass('t_cron_task') IS NULL
       OR to_regclass('t_cloud_115') IS NULL
       OR to_regclass('t_system_config') IS NULL
       OR to_regclass('idx_cron_task_key') IS NULL THEN
        RAISE EXCEPTION 'v48 缺少前置表或 idx_cron_task_key 索引，请 DBA 先核验已有迁移';
    END IF;

    SELECT array_agg(id ORDER BY id) INTO cleanup_task_ids
    FROM t_cron_task
    WHERE handler = 'cleanup_directories';
    IF cardinality(cleanup_task_ids) > 1 THEN
        RAISE EXCEPTION 'v48 存在多个 cleanup_directories 任务，任务 ids=%，请 DBA 先处理', cleanup_task_ids;
    END IF;
END $pre$;

CREATE TABLE IF NOT EXISTS t_cron_cleanup_target (
    id BIGSERIAL PRIMARY KEY,
    cron_task_id INTEGER NOT NULL REFERENCES t_cron_task(id) ON DELETE CASCADE,
    cloud115_id INTEGER NOT NULL REFERENCES t_cloud_115(id) ON DELETE CASCADE,
    dir_path VARCHAR(1024) NOT NULL,
    dir_cid TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_cleanup_target_path CHECK(dir_path ~ '^/[^/]' AND dir_path !~ '/$' AND dir_path !~ '//'),
    CONSTRAINT ck_cleanup_target_cid CHECK(dir_cid IS NULL OR dir_cid ~ '^[1-9][0-9]*$')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cron_cleanup_target
    ON t_cron_cleanup_target(cron_task_id,cloud115_id,dir_path);
CREATE INDEX IF NOT EXISTS idx_cron_cleanup_target_account
    ON t_cron_cleanup_target(cloud115_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cron_task_cleanup_directories_single
    ON t_cron_task(handler) WHERE handler = 'cleanup_directories';

INSERT INTO t_system_config(config_key,config_val)
VALUES('share_transfer_dir','/EmbyCache')
ON CONFLICT(config_key) DO NOTHING;
COMMIT;
