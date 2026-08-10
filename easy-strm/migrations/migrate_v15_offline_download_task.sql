-- migrate_v15_offline_download_task.sql
-- 115云下载（离线下载）任务记录表
-- 更新日期：2026-08-05
-- 说明：记录每次通过 easy-strm 提交到 115 离线下载的链接及其状态，
--       task_id 为任务中心（Redis）中的提交批次任务ID，cloud115_id 指向 t_cloud_115.id。
--       不加外键约束：账号删除后保留历史记录，仅在查询时回填账号名称。

CREATE TABLE IF NOT EXISTS t_offline_download_task (
    id            BIGSERIAL PRIMARY KEY,
    task_id       VARCHAR(64)  NOT NULL DEFAULT '',
    cloud115_id   INTEGER      NOT NULL,
    url           TEXT         NOT NULL,
    info_hash     VARCHAR(64)  NOT NULL DEFAULT '',
    name          TEXT         NOT NULL DEFAULT '',
    size          BIGINT       NOT NULL DEFAULT 0,
    status        VARCHAR(32)  NOT NULL DEFAULT 'pending',
    percent       DOUBLE PRECISION NOT NULL DEFAULT 0,
    error_message TEXT         NOT NULL DEFAULT '',
    save_dir_id   VARCHAR(64)  NOT NULL DEFAULT '',
    create_time   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_offline_download_task_task_id ON t_offline_download_task(task_id);
CREATE INDEX IF NOT EXISTS idx_offline_download_task_account_status ON t_offline_download_task(cloud115_id, status);
