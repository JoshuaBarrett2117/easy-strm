-- migrate_v17_emby_management.sql
-- Emby 多实例管理与媒体源实例关联。

CREATE TABLE IF NOT EXISTS t_emby_server (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    base_url VARCHAR(1000) NOT NULL,
    api_key TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_emby_server_default
    ON t_emby_server (is_default) WHERE is_default = TRUE;

ALTER TABLE t_media_source ADD COLUMN IF NOT EXISTS emby_server_id INTEGER;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_media_source_emby_server') THEN
        ALTER TABLE t_media_source
            ADD CONSTRAINT fk_media_source_emby_server
            FOREIGN KEY (emby_server_id) REFERENCES t_emby_server(id) ON DELETE SET NULL;
    END IF;
END $$;

INSERT INTO t_emby_server(name, base_url, api_key, enabled, is_default)
SELECT '默认 Emby', url.config_val, COALESCE(key.config_val, ''),
       COALESCE(enabled.config_val, 'false') IN ('true', '1'), TRUE
FROM t_system_config url
LEFT JOIN t_system_config key ON key.config_key = 'emby_api_key'
LEFT JOIN t_system_config enabled ON enabled.config_key = 'emby_enabled'
WHERE url.config_key = 'emby_url' AND BTRIM(url.config_val) <> ''
  AND NOT EXISTS (SELECT 1 FROM t_emby_server);

UPDATE t_media_source
SET emby_server_id = (SELECT id FROM t_emby_server ORDER BY is_default DESC, id ASC LIMIT 1)
WHERE emby_library_id <> '' AND emby_server_id IS NULL;

