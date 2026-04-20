-- Easy-STRM V13 数据库迁移脚本
-- 版本: v13
-- 创建时间: 2026-04-19
-- 说明: 媒体源新增监控目录字段，115 监控与源路径解耦
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'watch_path') THEN
        ALTER TABLE t_media_source ADD COLUMN watch_path VARCHAR(1000) DEFAULT '';
    END IF;
END $$;

COMMENT ON COLUMN t_media_source.watch_path IS '监控目录；115 自动监控时使用该目录而不是源路径';

UPDATE t_media_source
SET watch_path = COALESCE(NULLIF(watch_path, ''), path);
