-- 定时清空目录（原 migrations/migrate_v48_cron_dir_cleanup.sql，原需 DBA 手工执行）。
-- 幂等；不预置任务，已有 share_transfer_dir 配置不覆盖。
-- 与原脚本的差异：存在多个 cleanup_directories 任务时只告警并跳过单例唯一索引（原脚本直接中止），应用层仍会拒绝新增第二个。
-- +goose Up
-- +goose StatementBegin
SET LOCAL lock_timeout = '60s';
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
DO $single$
BEGIN
    IF to_regclass('uq_cron_task_cleanup_directories_single') IS NOT NULL THEN
        RETURN;
    END IF;
    IF (SELECT count(*) FROM t_cron_task WHERE handler = 'cleanup_directories') > 1 THEN
        RAISE WARNING 'cron dir cleanup: 存在多个 cleanup_directories 任务，跳过单例唯一索引，请手工合并后重启';
        RETURN;
    END IF;
    CREATE UNIQUE INDEX uq_cron_task_cleanup_directories_single
        ON t_cron_task(handler) WHERE handler = 'cleanup_directories';
END $single$;
INSERT INTO t_system_config(config_key,config_val)
VALUES('share_transfer_dir','/EmbyCache')
ON CONFLICT(config_key) DO NOTHING;
-- +goose StatementEnd
