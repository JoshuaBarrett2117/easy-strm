-- TMDB 模块数据库迁移脚本
-- 版本: v7
-- 创建时间: 2026-03-29
-- 说明: 创建 TMDB 缓存表和更名预设表，支持媒体识别和智能更名

-- ============================================
-- 1. TMDB 缓存表
-- ============================================
CREATE TABLE IF NOT EXISTS t_tmdb_cache (
    id SERIAL PRIMARY KEY,
    query_key VARCHAR(500) NOT NULL,                     -- 查询键（文件名或搜索词）
    media_type VARCHAR(20) NOT NULL,                     -- 媒体类型: movie | tv
    tmdb_id INTEGER NOT NULL,                            -- TMDB ID
    title VARCHAR(500),                                  -- 标题
    original_title VARCHAR(500),                         -- 原始标题
    year INTEGER,                                        -- 年份
    poster_path VARCHAR(500),                            -- 海报路径
    overview TEXT,                                       -- 简介
    vote_average DECIMAL(3,1),                           -- 评分
    release_date VARCHAR(20),                            -- 发布日期
    first_air_date VARCHAR(20),                          -- 首播日期（剧集）
    season_number INTEGER,                               -- 季数（剧集）
    episode_number INTEGER,                              -- 集数（剧集）
    raw_data JSONB,                                      -- TMDB 原始数据
    expire_at TIMESTAMP NOT NULL,                        -- 缓存过期时间
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 创建时间
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 更新时间
    UNIQUE (query_key, media_type)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_tmdb_cache_query_key ON t_tmdb_cache(query_key);
CREATE INDEX IF NOT EXISTS idx_tmdb_cache_tmdb_id ON t_tmdb_cache(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_tmdb_cache_expire_at ON t_tmdb_cache(expire_at);

-- 添加注释
COMMENT ON TABLE t_tmdb_cache IS 'TMDB 缓存表';
COMMENT ON COLUMN t_tmdb_cache.id IS '主键ID';
COMMENT ON COLUMN t_tmdb_cache.query_key IS '查询键，文件名或搜索词';
COMMENT ON COLUMN t_tmdb_cache.media_type IS '媒体类型: movie-电影, tv-剧集';
COMMENT ON COLUMN t_tmdb_cache.tmdb_id IS 'TMDB ID';
COMMENT ON COLUMN t_tmdb_cache.title IS '标题';
COMMENT ON COLUMN t_tmdb_cache.original_title IS '原始标题';
COMMENT ON COLUMN t_tmdb_cache.year IS '年份';
COMMENT ON COLUMN t_tmdb_cache.poster_path IS '海报路径';
COMMENT ON COLUMN t_tmdb_cache.overview IS '简介';
COMMENT ON COLUMN t_tmdb_cache.vote_average IS '评分';
COMMENT ON COLUMN t_tmdb_cache.release_date IS '发布日期';
COMMENT ON COLUMN t_tmdb_cache.first_air_date IS '首播日期（剧集）';
COMMENT ON COLUMN t_tmdb_cache.season_number IS '季数（剧集）';
COMMENT ON COLUMN t_tmdb_cache.episode_number IS '集数（剧集）';
COMMENT ON COLUMN t_tmdb_cache.raw_data IS 'TMDB 原始数据';
COMMENT ON COLUMN t_tmdb_cache.expire_at IS '缓存过期时间';
COMMENT ON COLUMN t_tmdb_cache.create_time IS '创建时间';
COMMENT ON COLUMN t_tmdb_cache.update_time IS '更新时间';

-- ============================================
-- 2. 更名预设方案表
-- ============================================
CREATE TABLE IF NOT EXISTS t_rename_preset (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,                          -- 方案名称
    media_type VARCHAR(20) NOT NULL,                     -- 媒体类型: movie | tv
    template VARCHAR(500) NOT NULL,                      -- 命名模板
    enabled BOOLEAN DEFAULT TRUE,                        -- 是否启用
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 创建时间
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP      -- 更新时间
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_rename_preset_media_type ON t_rename_preset(media_type);
CREATE INDEX IF NOT EXISTS idx_rename_preset_enabled ON t_rename_preset(enabled);

-- 添加注释
COMMENT ON TABLE t_rename_preset IS '更名预设方案表';
COMMENT ON COLUMN t_rename_preset.id IS '主键ID';
COMMENT ON COLUMN t_rename_preset.name IS '方案名称';
COMMENT ON COLUMN t_rename_preset.media_type IS '媒体类型: movie-电影, tv-剧集';
COMMENT ON COLUMN t_rename_preset.template IS '命名模板，支持 Jinja2 语法: {{ title }}, {{ year }}, {{ videoFormat }}, {{ season }}, {{ episode }}, {{ fileExt }}等';
COMMENT ON COLUMN t_rename_preset.enabled IS '是否启用';
COMMENT ON COLUMN t_rename_preset.create_time IS '创建时间';
COMMENT ON COLUMN t_rename_preset.update_time IS '更新时间';

-- ============================================
-- 3. 插入默认更名预设
-- ============================================
INSERT INTO t_rename_preset (name, media_type, template, enabled) VALUES
('电影（官方）', 'movie', '{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{% if year %} ({{ year }}){% endif %}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}', true),
('电影（简洁）', 'movie', '{{ title }}{{ fileExt }}', true),
('剧集（官方）', 'tv', '{{ title }}{% if year %} ({{ year }}){% endif %}/Season {{ "%02d"|format(season|int) }}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{% if videoFormat %} [{{ videoFormat }}]{% endif %}{{ fileExt }}', true),
('剧集（简洁）', 'tv', '{{ title }}/S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{{ fileExt }}', true);

-- ============================================
-- 4. 媒体文件识别缓存表
-- ============================================
CREATE TABLE IF NOT EXISTS t_media_file_cache (
    id SERIAL PRIMARY KEY,
    source_id INTEGER NOT NULL,                          -- 媒体源ID
    file_path VARCHAR(1000) NOT NULL,                    -- 文件路径
    file_name VARCHAR(500) NOT NULL,                     -- 文件名
    file_size BIGINT,                                    -- 文件大小
    sha1 VARCHAR(40),                                    -- SHA1
    tmdb_id INTEGER,                                     -- TMDB ID
    media_type VARCHAR(20),                              -- 媒体类型: movie | tv | unknown
    season_number INTEGER,                               -- 季数
    episode_number INTEGER,                              -- 集数
    tmdb_data JSONB,                                     -- TMDB 返回的完整数据
    identified_at TIMESTAMP,                             -- 识别时间
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 创建时间
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,     -- 更新时间
    FOREIGN KEY (source_id) REFERENCES t_media_source(id) ON DELETE CASCADE,
    UNIQUE (source_id, file_path)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_media_file_cache_source_id ON t_media_file_cache(source_id);
CREATE INDEX IF NOT EXISTS idx_media_file_cache_tmdb_id ON t_media_file_cache(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_media_file_cache_media_type ON t_media_file_cache(media_type);

-- 添加注释
COMMENT ON TABLE t_media_file_cache IS '媒体文件识别缓存表';
COMMENT ON COLUMN t_media_file_cache.id IS '主键ID';
COMMENT ON COLUMN t_media_file_cache.source_id IS '媒体源ID';
COMMENT ON COLUMN t_media_file_cache.file_path IS '文件路径';
COMMENT ON COLUMN t_media_file_cache.file_name IS '文件名';
COMMENT ON COLUMN t_media_file_cache.file_size IS '文件大小';
COMMENT ON COLUMN t_media_file_cache.sha1 IS '文件SHA1';
COMMENT ON COLUMN t_media_file_cache.tmdb_id IS 'TMDB ID';
COMMENT ON COLUMN t_media_file_cache.media_type IS '媒体类型: movie-电影, tv-剧集, unknown-未知';
COMMENT ON COLUMN t_media_file_cache.season_number IS '季数';
COMMENT ON COLUMN t_media_file_cache.episode_number IS '集数';
COMMENT ON COLUMN t_media_file_cache.tmdb_data IS 'TMDB 返回的完整数据';
COMMENT ON COLUMN t_media_file_cache.identified_at IS '识别时间';
COMMENT ON COLUMN t_media_file_cache.create_time IS '创建时间';
COMMENT ON COLUMN t_media_file_cache.update_time IS '更新时间';
