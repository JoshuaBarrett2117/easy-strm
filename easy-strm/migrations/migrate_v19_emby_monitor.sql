-- migrate_v19_emby_monitor.sql
-- Emby 观影监控、自采播放历史与媒体首次发现记录。

CREATE TABLE IF NOT EXISTS t_emby_playback_event (
    id BIGSERIAL PRIMARY KEY,
    server_id INTEGER NOT NULL REFERENCES t_emby_server(id) ON DELETE CASCADE,
    session_id VARCHAR(200) NOT NULL,
    user_id VARCHAR(200) NOT NULL DEFAULT '',
    user_name VARCHAR(300) NOT NULL DEFAULT '',
    item_id VARCHAR(200) NOT NULL,
    item_type VARCHAR(50) NOT NULL DEFAULT '',
    item_name VARCHAR(500) NOT NULL DEFAULT '',
    series_id VARCHAR(200) NOT NULL DEFAULT '',
    series_name VARCHAR(500) NOT NULL DEFAULT '',
    image_tag VARCHAR(300) NOT NULL DEFAULT '',
    client_name VARCHAR(300) NOT NULL DEFAULT '',
    device_name VARCHAR(300) NOT NULL DEFAULT '',
    playback_method VARCHAR(100) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    watched_seconds BIGINT NOT NULL DEFAULT 0,
    last_seen_at TIMESTAMPTZ NOT NULL,
    is_paused BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_emby_playback_active_session
    ON t_emby_playback_event(server_id, session_id) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_emby_playback_server_started
    ON t_emby_playback_event(server_id, started_at DESC);

CREATE TABLE IF NOT EXISTS t_emby_monitor_item (
    server_id INTEGER NOT NULL REFERENCES t_emby_server(id) ON DELETE CASCADE,
    item_id VARCHAR(200) NOT NULL,
    item_type VARCHAR(50) NOT NULL DEFAULT '',
    name VARCHAR(500) NOT NULL DEFAULT '',
    series_name VARCHAR(500) NOT NULL DEFAULT '',
    production_year INTEGER NOT NULL DEFAULT 0,
    image_tag VARCHAR(300) NOT NULL DEFAULT '',
    date_created TIMESTAMPTZ NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    is_baseline BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY(server_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_emby_monitor_item_created
    ON t_emby_monitor_item(server_id, date_created DESC);
CREATE INDEX IF NOT EXISTS idx_emby_monitor_item_seen
    ON t_emby_monitor_item(server_id, first_seen_at DESC);

CREATE TABLE IF NOT EXISTS t_emby_monitor_state (
    server_id INTEGER PRIMARY KEY REFERENCES t_emby_server(id) ON DELETE CASCADE,
    baseline_completed_at TIMESTAMPTZ,
    last_session_poll_at TIMESTAMPTZ,
    last_item_poll_at TIMESTAMPTZ,
    plugin_available BOOLEAN NOT NULL DEFAULT FALSE,
    last_error TEXT NOT NULL DEFAULT '',
    update_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
