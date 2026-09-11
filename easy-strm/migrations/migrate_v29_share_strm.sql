-- 2026-09-09 Codex：持久化按需转存的单文件播放映射。
CREATE TABLE IF NOT EXISTS t_share_strm (
 id UUID PRIMARY KEY,
 payload JSONB NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
