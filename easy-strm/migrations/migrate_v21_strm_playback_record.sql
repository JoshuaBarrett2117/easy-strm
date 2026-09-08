-- STRM 直链调用记录，使用数据库持久化。
CREATE TABLE IF NOT EXISTS t_strm_playback_record (
    id BIGSERIAL PRIMARY KEY,
    record_id VARCHAR(64) NOT NULL UNIQUE,
    resource_name TEXT NOT NULL DEFAULT '',
    poster TEXT NOT NULL DEFAULT '',
    direct_url TEXT NOT NULL DEFAULT '',
    called_at TIMESTAMPTZ NOT NULL,
    caller_ip VARCHAR(64) NOT NULL DEFAULT '',
    location VARCHAR(255) NOT NULL DEFAULT '',
    request_method VARCHAR(16) NOT NULL DEFAULT 'GET'
);
CREATE INDEX IF NOT EXISTS idx_strm_playback_record_called_at ON t_strm_playback_record (called_at DESC);
