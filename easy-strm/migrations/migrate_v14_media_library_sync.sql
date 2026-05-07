-- Easy-STRM V14 数据库迁移脚本
-- 版本: v14
-- 创建时间: 2026-05-06
-- 说明: 媒体库同步索引、任务步骤、待处理清单

CREATE TABLE IF NOT EXISTS t_media_sync_index (
    id SERIAL PRIMARY KEY,
    source_id INTEGER NOT NULL,
    source_type VARCHAR(32) NOT NULL DEFAULT '',
    source_file_id VARCHAR(1000) NOT NULL DEFAULT '',
    source_path VARCHAR(2000) NOT NULL DEFAULT '',
    source_name VARCHAR(1000) NOT NULL DEFAULT '',
    source_pick_code VARCHAR(128) NOT NULL DEFAULT '',
    source_sha1 VARCHAR(128) NOT NULL DEFAULT '',
    source_size BIGINT NOT NULL DEFAULT 0,
    source_modified_time TIMESTAMP,
    target_path VARCHAR(2000) NOT NULL DEFAULT '',
    strm_path VARCHAR(2000) NOT NULL DEFAULT '',
    metadata_path VARCHAR(2000) NOT NULL DEFAULT '',
    media_server_type VARCHAR(32) NOT NULL DEFAULT '',
    media_server_library_id VARCHAR(255) NOT NULL DEFAULT '',
    tmdb_id INTEGER NOT NULL DEFAULT 0,
    media_type VARCHAR(32) NOT NULL DEFAULT '',
    identity_status VARCHAR(32) NOT NULL DEFAULT 'unknown',
    sync_status VARCHAR(32) NOT NULL DEFAULT 'active',
    last_change_type VARCHAR(32) NOT NULL DEFAULT 'created',
    last_task_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_media_sync_index_source_file
    ON t_media_sync_index(source_id, source_file_id);

CREATE INDEX IF NOT EXISTS idx_media_sync_index_source
    ON t_media_sync_index(source_id, sync_status);

CREATE INDEX IF NOT EXISTS idx_media_sync_index_tmdb
    ON t_media_sync_index(tmdb_id, media_type);

CREATE TABLE IF NOT EXISTS t_task_step (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(128) NOT NULL,
    step_key VARCHAR(64) NOT NULL,
    step_name VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    sort_order INTEGER NOT NULL DEFAULT 0,
    input_summary TEXT NOT NULL DEFAULT '',
    output_summary TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_task_step_task_step
    ON t_task_step(task_id, step_key);

CREATE INDEX IF NOT EXISTS idx_task_step_task
    ON t_task_step(task_id, sort_order);

CREATE TABLE IF NOT EXISTS t_pending_media_item (
    id SERIAL PRIMARY KEY,
    source_kind VARCHAR(32) NOT NULL DEFAULT 'manual',
    source_id INTEGER NOT NULL DEFAULT 0,
    source_file_id VARCHAR(1000) NOT NULL DEFAULT '',
    source_path VARCHAR(2000) NOT NULL DEFAULT '',
    title VARCHAR(1000) NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    media_type VARCHAR(32) NOT NULL DEFAULT '',
    season INTEGER NOT NULL DEFAULT 0,
    episode INTEGER NOT NULL DEFAULT 0,
    tmdb_id INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    reason TEXT NOT NULL DEFAULT '',
    related_task_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pending_media_status
    ON t_pending_media_item(status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_pending_media_source
    ON t_pending_media_item(source_id, source_file_id);

CREATE OR REPLACE FUNCTION update_media_library_timestamp() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_media_sync_index'::regclass AND tgname = 'update_media_sync_index_timestamp_trigger') THEN
        CREATE TRIGGER update_media_sync_index_timestamp_trigger
        BEFORE UPDATE ON t_media_sync_index
        FOR EACH ROW
        EXECUTE FUNCTION update_media_library_timestamp();
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_task_step'::regclass AND tgname = 'update_task_step_timestamp_trigger') THEN
        CREATE TRIGGER update_task_step_timestamp_trigger
        BEFORE UPDATE ON t_task_step
        FOR EACH ROW
        EXECUTE FUNCTION update_media_library_timestamp();
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 't_pending_media_item'::regclass AND tgname = 'update_pending_media_item_timestamp_trigger') THEN
        CREATE TRIGGER update_pending_media_item_timestamp_trigger
        BEFORE UPDATE ON t_pending_media_item
        FOR EACH ROW
        EXECUTE FUNCTION update_media_library_timestamp();
    END IF;
END $$;
