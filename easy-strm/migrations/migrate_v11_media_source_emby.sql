-- 为 t_media_source 表添加 Emby 媒体库绑定字段
-- 允许每个媒体源可选绑定一个 Emby 媒体库，整理完成后自动触发该库刷新
ALTER TABLE t_media_source ADD COLUMN IF NOT EXISTS emby_library_id TEXT DEFAULT '';
