-- 2026-09-03 Codex：媒体源级元数据来源策略。
ALTER TABLE t_media_source
    ADD COLUMN IF NOT EXISTS metadata_source VARCHAR(20) NOT NULL DEFAULT 'auto';

UPDATE t_media_source
SET metadata_source = 'auto'
WHERE metadata_source IS NULL OR metadata_source NOT IN ('auto', 'tmdb', 'metatube');

COMMENT ON COLUMN t_media_source.metadata_source IS '元数据来源策略（auto/tmdb/metatube）';
