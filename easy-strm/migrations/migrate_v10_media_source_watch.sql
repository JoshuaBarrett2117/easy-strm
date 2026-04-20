-- Easy-STRM V10 数据库迁移脚本
-- 版本: v10
-- 创建时间: 2026-04-09
-- 说明: 媒体源增加自动整理与监控相关字段

-- ============================================
-- 1. 媒体源表增加 auto_organize 字段
--    是否自动整理该媒体源的新文件
-- ============================================
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'auto_organize') THEN
        ALTER TABLE t_media_source ADD COLUMN auto_organize BOOLEAN DEFAULT FALSE;
    END IF;
END $$;

COMMENT ON COLUMN t_media_source.auto_organize IS '是否自动整理新文件';

-- ============================================
-- 2. 媒体源表增加 watch_enabled 字段
--    是否启用目录监控（本地用fsnotify，115用轮询）
-- ============================================
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'watch_enabled') THEN
        ALTER TABLE t_media_source ADD COLUMN watch_enabled BOOLEAN DEFAULT FALSE;
    END IF;
END $$;

COMMENT ON COLUMN t_media_source.watch_enabled IS '是否启用目录监控';

-- ============================================
-- 3. 媒体源表增加 watch_interval 字段
--    115云盘轮询间隔（秒），默认1800秒（30分钟）
-- ============================================
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'watch_interval') THEN
        ALTER TABLE t_media_source ADD COLUMN watch_interval INT DEFAULT 1800;
    END IF;
END $$;

COMMENT ON COLUMN t_media_source.watch_interval IS '115云盘监控轮询间隔（秒）';

-- ============================================
-- 验证迁移结果
-- ============================================
SELECT column_name, data_type, column_default FROM information_schema.columns WHERE table_name = 't_media_source' ORDER BY ordinal_position;
