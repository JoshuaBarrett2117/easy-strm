-- migrate_v49_behavior_sync.sql
-- 115 操作事件增量同步（P1-P3）：检查点与身份索引。DBA 审核后手工应用，不注册为启动迁移。
CREATE TABLE t_behavior_checkpoint (
    cloud115_id integer PRIMARY KEY REFERENCES t_cloud_115(id),
    last_event_id bigint NOT NULL DEFAULT 0,
    last_run_at timestamp,
    update_time timestamp
);

CREATE TABLE t_strm_file_index (
    cloud115_id integer NOT NULL REFERENCES t_cloud_115(id),
    file_id text NOT NULL,
    parent_id text NOT NULL,
    strm_path text NOT NULL,
    is_dir boolean NOT NULL DEFAULT false,
    size bigint NOT NULL DEFAULT 0,
    sha1 text NOT NULL DEFAULT '',
    updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    owned boolean NOT NULL DEFAULT false,
    content_hash text NOT NULL DEFAULT '',
    pending_path text NOT NULL DEFAULT '',
    pending_content text NOT NULL DEFAULT '',
    PRIMARY KEY (cloud115_id, file_id),
    UNIQUE (strm_path)
);
CREATE INDEX idx_strm_file_index_parent ON t_strm_file_index(cloud115_id, parent_id);
