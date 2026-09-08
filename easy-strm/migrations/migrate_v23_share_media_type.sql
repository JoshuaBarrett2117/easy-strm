-- 2026-09-07 Codex：分享级媒体类型，历史分享默认电影，可在编辑中调整。
ALTER TABLE t_share_record ADD COLUMN IF NOT EXISTS media_type VARCHAR(10) NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv'));
