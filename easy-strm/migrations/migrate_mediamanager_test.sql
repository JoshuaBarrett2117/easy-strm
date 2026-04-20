-- ===================== MediaManager 模块测试数据库初始化脚本 =====================
-- 作者：质量保障专家
-- 日期：2026-03-29
-- 说明：创建测试所需的数据库表结构

-- 创建测试数据库（如果不存在）
-- CREATE DATABASE easy_strm_test;

-- 连接到测试数据库
-- \c easy_strm_test;

-- 创建媒体源配置表
CREATE TABLE IF NOT EXISTS t_media_source (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    source_type VARCHAR(20) NOT NULL CHECK (source_type IN ('local', 'cloud115')),
    path VARCHAR(500) NOT NULL,
    cloud115_id INTEGER,
    priority INT DEFAULT 10,
    enabled BOOLEAN DEFAULT TRUE,
    organize_target_path VARCHAR(1000) DEFAULT '',
    media_type VARCHAR(20) DEFAULT 'all',
    conflict_policy VARCHAR(20) DEFAULT 'skip',
    operation_mode VARCHAR(20) DEFAULT 'move',
    auto_organize BOOLEAN DEFAULT FALSE,
    watch_enabled BOOLEAN DEFAULT FALSE,
    watch_interval INT DEFAULT 1800,
    emby_library_id TEXT DEFAULT '',
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (cloud115_id) REFERENCES t_cloud_115(id) ON DELETE SET NULL
);

-- 创建媒体文件缓存表
CREATE TABLE IF NOT EXISTS t_media_file_cache (
    id SERIAL PRIMARY KEY,
    source_id INTEGER NOT NULL,
    file_path VARCHAR(1000) NOT NULL,
    file_name VARCHAR(500) NOT NULL,
    file_size BIGINT,
    sha1 VARCHAR(40),
    tmdb_id INTEGER,
    media_type VARCHAR(20) CHECK (media_type IN ('movie', 'tv', 'unknown')),
    tmdb_data JSONB,
    identified_at TIMESTAMP,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_id) REFERENCES t_media_source(id) ON DELETE CASCADE,
    UNIQUE (source_id, file_path)
);

-- 创建重命名预设方案表
CREATE TABLE IF NOT EXISTS t_rename_preset (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    media_type VARCHAR(20) NOT NULL CHECK (media_type IN ('movie', 'tv')),
    template VARCHAR(500) NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建自动整理规则表
CREATE TABLE IF NOT EXISTS t_organize_rule (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    source_ids INTEGER[] NOT NULL,
    target_source_id INTEGER NOT NULL,
    target_path VARCHAR(500),
    media_type VARCHAR(20) NOT NULL CHECK (media_type IN ('movie', 'tv', 'all')),
    rename_preset_id INTEGER,
    move_mode VARCHAR(10) DEFAULT 'copy' CHECK (move_mode IN ('copy', 'move')),
    conflict_policy VARCHAR(20) DEFAULT 'skip' CHECK (conflict_policy IN ('skip', 'overwrite', 'suffix')),
    cron_expr VARCHAR(50),
    enabled BOOLEAN DEFAULT TRUE,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (target_source_id) REFERENCES t_media_source(id),
    FOREIGN KEY (rename_preset_id) REFERENCES t_rename_preset(id)
);

-- 创建任务操作日志表（用于回滚）
CREATE TABLE IF NOT EXISTS t_task_log (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL CHECK (action IN ('rename', 'move', 'copy', 'delete')),
    source_path VARCHAR(1000) NOT NULL,
    target_path VARCHAR(1000),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'success', 'failed', 'rolled_back')),
    error_message TEXT,
    rollback_available BOOLEAN DEFAULT FALSE,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引以提升查询性能
CREATE INDEX IF NOT EXISTS idx_media_source_type ON t_media_source(source_type);
CREATE INDEX IF NOT EXISTS idx_media_source_enabled ON t_media_source(enabled);
CREATE INDEX IF NOT EXISTS idx_media_file_source ON t_media_file_cache(source_id);
CREATE INDEX IF NOT EXISTS idx_media_file_sha1 ON t_media_file_cache(sha1);
CREATE INDEX IF NOT EXISTS idx_task_log_task_id ON t_task_log(task_id);

-- 插入测试数据
INSERT INTO t_media_source (name, source_type, path, priority, enabled) VALUES
('测试本地电影库', 'local', 'D:\TestMovies', 10, true),
('测试本地剧集库', 'local', 'D:\TestTVShows', 20, true);

-- 清理测试数据（运行测试后执行）
-- DELETE FROM t_media_source WHERE name LIKE '测试%';
-- DELETE FROM t_media_file_cache WHERE file_name LIKE '测试%';
