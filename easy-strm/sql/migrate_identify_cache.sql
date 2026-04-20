-- 识别结果缓存表
-- 用于存储文件名的识别结果，支持跨媒体源复用

CREATE TABLE IF NOT EXISTS t_identify_cache (
    id              SERIAL PRIMARY KEY,
    file_hash       VARCHAR(64) NOT NULL,              -- 文件名hash（用于精确匹配）
    file_name       VARCHAR(500) NOT NULL,            -- 原文件名（用于展示）
    media_type      VARCHAR(20) NOT NULL,             -- movie / tv
    tmdb_id         INTEGER,                          -- TMDB ID
    title           VARCHAR(255),                    -- 识别标题
    original_title  VARCHAR(255),                    -- 原标题
    year            INTEGER,                          -- 年份
    season_number   INTEGER DEFAULT 0,               -- 季号
    episode_number  INTEGER DEFAULT 0,               -- 集号
    poster_path     VARCHAR(500),                    -- 海报路径
    is_manual       BOOLEAN DEFAULT FALSE,           -- 是否手动识别
    source_id       INTEGER,                          -- 来源媒体源ID（可选）
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW()
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_identify_cache_file_hash ON t_identify_cache(file_hash);
CREATE INDEX IF NOT EXISTS idx_identify_cache_is_manual ON t_identify_cache(is_manual);
CREATE INDEX IF NOT EXISTS idx_identify_cache_created_at ON t_identify_cache(created_at);

-- 注释
COMMENT ON TABLE t_identify_cache IS '识别结果缓存表 - 存储文件名识别结果，支持跨媒体源复用';
COMMENT ON COLUMN t_identify_cache.file_hash IS '文件名hash';
COMMENT ON COLUMN t_identify_cache.is_manual IS '是否手动识别（手动识别结果永久保留）';
