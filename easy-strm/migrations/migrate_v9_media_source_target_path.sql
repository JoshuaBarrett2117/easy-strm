-- Easy-STRM V9 数据库迁移脚本
-- 版本: v9
-- 创建时间: 2026-04-09
-- 说明: 媒体源增加整理目的地目录字段

-- ============================================
-- 1. 媒体源表增加 organize_target_path 字段
-- ============================================
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'organize_target_path') THEN
        ALTER TABLE t_media_source ADD COLUMN organize_target_path VARCHAR(1000) DEFAULT '';
    END IF;
END $$;

COMMENT ON COLUMN t_media_source.organize_target_path IS '整理目的地目录（根目录），如 /已整理';

-- ============================================
-- 验证迁移结果
-- ============================================
SELECT column_name, data_type, column_default FROM information_schema.columns WHERE table_name = 't_media_source' ORDER BY ordinal_position;
