-- Easy-STRM V12 数据库迁移脚本
-- 版本: v12
-- 创建时间: 2026-04-19
-- 说明: 媒体源增加自动整理默认配置字段

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'media_type') THEN
        ALTER TABLE t_media_source ADD COLUMN media_type VARCHAR(20) DEFAULT 'all';
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'conflict_policy') THEN
        ALTER TABLE t_media_source ADD COLUMN conflict_policy VARCHAR(20) DEFAULT 'skip';
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 't_media_source' AND column_name = 'operation_mode') THEN
        ALTER TABLE t_media_source ADD COLUMN operation_mode VARCHAR(20) DEFAULT 'move';
    END IF;
END $$;

COMMENT ON COLUMN t_media_source.media_type IS '自动整理默认媒体类型（all/movie/tv）';
COMMENT ON COLUMN t_media_source.conflict_policy IS '自动整理默认冲突策略（skip/overwrite/suffix）';
COMMENT ON COLUMN t_media_source.operation_mode IS '自动整理默认操作方式（move/copy/hardlink/symlink）';

UPDATE t_media_source
SET media_type = COALESCE(NULLIF(media_type, ''), 'all'),
    conflict_policy = COALESCE(NULLIF(conflict_policy, ''), 'skip'),
    operation_mode = COALESCE(NULLIF(operation_mode, ''), 'move');
