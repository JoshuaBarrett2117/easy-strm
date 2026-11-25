-- 115 操作事件增量同步（原 migrations/migrate_v49_behavior_sync.sql + migrate_v50_behavior_sync_config.sql，原需 DBA 手工执行）。
-- 幂等：
--   * 表不存在：直接建成 v50 的最终结构（checkpoint 按 (strm_config_id, cloud115_id) 复合主键）。
--   * 只装过 v49（缺 strm_config_id）：按 v50 的方式迁移旧游标——每个 STRM 配置继承所属账号的旧游标，删除旧全局行，重建复合主键。
--   * 已装过 v50：不做任何改动。
-- t_strm_config.behavior_sync_enabled / behavior_sync_interval 已在基线中。
-- +goose Up
-- +goose StatementBegin
SET LOCAL lock_timeout = '60s';
CREATE TABLE IF NOT EXISTS t_behavior_checkpoint (
    cloud115_id integer NOT NULL REFERENCES t_cloud_115(id),
    last_event_id bigint NOT NULL DEFAULT 0,
    last_run_at timestamp,
    update_time timestamp,
    strm_config_id integer NOT NULL REFERENCES t_strm_config(id),
    PRIMARY KEY (strm_config_id, cloud115_id)
);
DO $checkpoint$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid='t_behavior_checkpoint'::regclass AND attname='strm_config_id' AND NOT attisdropped) THEN
        RETURN;
    END IF;
    ALTER TABLE t_behavior_checkpoint ADD COLUMN strm_config_id integer REFERENCES t_strm_config(id);
    ALTER TABLE t_behavior_checkpoint DROP CONSTRAINT IF EXISTS t_behavior_checkpoint_pkey;
    INSERT INTO t_behavior_checkpoint(strm_config_id, cloud115_id, last_event_id, last_run_at, update_time)
    SELECT sc.id, sc.cloud115_id, COALESCE(legacy.last_event_id, 0), legacy.last_run_at, legacy.update_time
    FROM t_strm_config sc
    LEFT JOIN t_behavior_checkpoint legacy ON legacy.cloud115_id = sc.cloud115_id AND legacy.strm_config_id IS NULL
    WHERE NOT EXISTS (
        SELECT 1 FROM t_behavior_checkpoint cp WHERE cp.strm_config_id = sc.id AND cp.cloud115_id = sc.cloud115_id
    );
    DELETE FROM t_behavior_checkpoint WHERE strm_config_id IS NULL;
    ALTER TABLE t_behavior_checkpoint ALTER COLUMN strm_config_id SET NOT NULL;
    ALTER TABLE t_behavior_checkpoint ADD PRIMARY KEY (strm_config_id, cloud115_id);
END $checkpoint$;
CREATE TABLE IF NOT EXISTS t_strm_file_index (
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
CREATE INDEX IF NOT EXISTS idx_strm_file_index_parent ON t_strm_file_index(cloud115_id, parent_id);
-- +goose StatementEnd
