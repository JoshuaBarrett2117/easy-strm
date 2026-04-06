-- MediaManager 模块数据库迁移脚本
-- 版本: v6
-- 创建时间: 2026-03-29
-- 说明: 创建媒体源配置表，支持本地和115云盘媒体源管理

-- ============================================
-- 1. 媒体源配置表
-- ============================================
CREATE TABLE IF NOT EXISTS t_media_source (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,                          -- 媒体源名称
    source_type VARCHAR(20) NOT NULL,                    -- 媒体源类型: 'local' | 'cloud115'
    path VARCHAR(500) NOT NULL,                          -- 本地路径 或 115 CID
    cloud115_id INTEGER,                                 -- 仅 cloud115 类型需要，关联 t_cloud_115
    priority INT DEFAULT 10,                             -- 优先级（多源时优先使用）
    enabled BOOLEAN DEFAULT TRUE,                        -- 是否启用
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 创建时间
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 更新时间
    FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id) ON DELETE SET NULL
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_media_source_type ON t_media_source(source_type);
CREATE INDEX IF NOT EXISTS idx_media_source_enabled ON t_media_source(enabled);
CREATE INDEX IF NOT EXISTS idx_media_source_priority ON t_media_source(priority);

-- 添加注释
COMMENT ON TABLE t_media_source IS '媒体源配置表';
COMMENT ON COLUMN t_media_source.id IS '主键ID';
COMMENT ON COLUMN t_media_source.name IS '媒体源名称，如"本地电影库"';
COMMENT ON COLUMN t_media_source.source_type IS '媒体源类型: local-本地存储, cloud115-115云盘';
COMMENT ON COLUMN t_media_source.path IS '本地绝对路径或115目录CID';
COMMENT ON COLUMN t_media_source.cloud115_id IS '115账号ID，仅cloud115类型需要';
COMMENT ON COLUMN t_media_source.priority IS '优先级，数值越小优先级越高';
COMMENT ON COLUMN t_media_source.enabled IS '是否启用';
COMMENT ON COLUMN t_media_source.create_time IS '创建时间';
COMMENT ON COLUMN t_media_source.update_time IS '更新时间';

-- ============================================
-- 2. 媒体文件缓存表（预留，Phase 2 使用）
-- ============================================
-- CREATE TABLE IF NOT EXISTS t_media_file_cache (
--     id SERIAL PRIMARY KEY,
--     source_id INTEGER NOT NULL,
--     file_path VARCHAR(1000) NOT NULL,
--     file_name VARCHAR(500) NOT NULL,
--     file_size BIGINT,
--     sha1 VARCHAR(40),
--     tmdb_id INTEGER,
--     media_type VARCHAR(20),               -- movie | tv | unknown
--     tmdb_data JSONB,                      -- TMDB 返回的完整数据
--     identified_at TIMESTAMP,
--     create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
--     FOREIGN KEY (source_id) REFERENCES t_media_source(id),
--     UNIQUE (source_id, file_path)
-- );
