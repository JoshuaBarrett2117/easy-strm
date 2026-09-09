-- 2026-09-08 Codex：允许混合分享按单条媒体自动判断，不改变已有显式类型选择。
ALTER TABLE t_share_record DROP CONSTRAINT IF EXISTS t_share_record_media_type_check;
ALTER TABLE t_share_record ADD CONSTRAINT t_share_record_media_type_check CHECK (media_type IN ('auto','movie','tv'));
ALTER TABLE t_share_record ALTER COLUMN media_type SET DEFAULT 'auto';
