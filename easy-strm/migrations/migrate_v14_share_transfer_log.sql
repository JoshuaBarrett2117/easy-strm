-- migrate_v14_share_transfer_log: 115分享链接一键转存日志表
-- 用于记录每次分享转存任务的逐文件执行日志

CREATE TABLE IF NOT EXISTS t_share_transfer_log (
    id                SERIAL PRIMARY KEY,
    task_id           VARCHAR(64)  NOT NULL,                       -- 转存任务ID，格式 share-transfer-{uuid}
    share_code        VARCHAR(32)  NOT NULL DEFAULT '',            -- 115分享码
    share_folder_name VARCHAR(512) NOT NULL DEFAULT '',            -- 分享文件夹名称
    file_name         VARCHAR(512) NOT NULL DEFAULT '',            -- 文件名
    file_pick_code    VARCHAR(64)  NOT NULL DEFAULT '',            -- 文件pickcode
    file_size         BIGINT       NOT NULL DEFAULT 0,            -- 文件大小（字节）
    file_sha1         VARCHAR(128) NOT NULL DEFAULT '',            -- 文件SHA1
    cloud115_id       INTEGER      NOT NULL DEFAULT 0,            -- 目标115账号ID
    target_directory  VARCHAR(1024) NOT NULL DEFAULT '',           -- 目标目录路径
    status            VARCHAR(32)  NOT NULL DEFAULT 'pending',    -- 状态：pending/success/skipped/failed
    error_message     TEXT         NOT NULL DEFAULT '',            -- 错误信息
    is_second_transfer BOOLEAN     NOT NULL DEFAULT FALSE,        -- 是否二传（非秒传）
    create_time       TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time       TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 索引1: 按任务ID查询所有日志
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_task_id ON t_share_transfer_log (task_id);

-- 索引2: 按分享码查询
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_share_code ON t_share_transfer_log (share_code);

-- 索引3: 按任务ID+状态查询（用于获取失败文件列表）
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_task_status ON t_share_transfer_log (task_id, status);

-- 索引4: 按目标账号查询
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_cloud115_id ON t_share_transfer_log (cloud115_id);

-- 索引5: 按创建时间降序查询
CREATE INDEX IF NOT EXISTS idx_share_transfer_log_create_time ON t_share_transfer_log (create_time DESC);
