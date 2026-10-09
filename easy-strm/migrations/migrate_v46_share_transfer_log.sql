-- 更新日期：2026-10-09；维护者：Codex。
-- 原启动链未执行 v14 建表，导致新库和缺表的旧库均无法写转存日志。
-- v46 接替缺失的建表语义，保留 v14 文件不动，不执行其五个旧索引。
-- 列与索引以 /opt/data/output/easy-strm-phase03/D1-DBA-review.md 为基线。
-- 本次仅完成离线应用修复；实际 SQL 执行、目标库及演练须另行取得 DBA 批准。

CREATE TABLE IF NOT EXISTS t_share_transfer_log (
    id                 SERIAL PRIMARY KEY,
    task_id            VARCHAR(64)   NOT NULL,
    share_code         VARCHAR(32)   NOT NULL DEFAULT '',
    share_folder_name  VARCHAR(512)  NOT NULL DEFAULT '',
    file_name          VARCHAR(512)  NOT NULL DEFAULT '',
    file_pick_code     VARCHAR(64)   NOT NULL DEFAULT '',
    file_size          BIGINT        NOT NULL DEFAULT 0,
    file_sha1          VARCHAR(128)  NOT NULL DEFAULT '',
    cloud115_id        INTEGER       NOT NULL DEFAULT 0,
    target_directory   VARCHAR(1024) NOT NULL DEFAULT '',
    status             VARCHAR(32)   NOT NULL DEFAULT 'pending',
    error_message      TEXT          NOT NULL DEFAULT '',
    is_second_transfer BOOLEAN       NOT NULL DEFAULT FALSE,
    create_time        TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time        TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- GetByTaskId 按任务过滤并按 id 升序读取。
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_task_id_id
    ON t_share_transfer_log (task_id, id);

-- UpdateStatus 按任务与 Fid 精确更新；file_pick_code 是历史列名，实际存储 Fid。
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_task_pick
    ON t_share_transfer_log (task_id, file_pick_code);
