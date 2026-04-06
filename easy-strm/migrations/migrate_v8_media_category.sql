-- ============================================
-- 媒体分类数据表
-- ============================================
CREATE TABLE IF NOT EXISTS t_media_category (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    media_type VARCHAR(20) NOT NULL,             -- movie | tv
    target_path VARCHAR(1000) NOT NULL,
    match_rules JSONB,                           -- 匹配规则: {"genres": ["Animation"], "keywords": ["动漫"]}
    enabled BOOLEAN DEFAULT TRUE,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_media_category_type ON t_media_category (media_type);

COMMENT ON TABLE t_media_category IS '媒体分类表';
COMMENT ON COLUMN t_media_category.id IS '主键ID';
COMMENT ON COLUMN t_media_category.name IS '分类名称';
COMMENT ON COLUMN t_media_category.media_type IS '媒体类型';
COMMENT ON COLUMN t_media_category.target_path IS '目标存储目录';
COMMENT ON COLUMN t_media_category.match_rules IS '分类匹配规则（JSON格式）';
COMMENT ON COLUMN t_media_category.enabled IS '是否启用';

-- 初始化图中分类策略；default=true 的未分类只在其他规则未命中后兜底。
WITH defaults(name, media_type, target_path, match_rules, enabled) AS (
    VALUES
('动画电影', 'movie', '/电影/动画电影', '{"genre_ids":[16]}'::jsonb, true),
('华语电影', 'movie', '/电影/华语电影', '{"languages":["zh","cn"]}'::jsonb, true),
('外语电影', 'movie', '/电影/外语电影', '{"languages":["en","ja","ko","fr","de","es","it","ru","nl","pt","th","hi"]}'::jsonb, true),
('未分类', 'movie', '/电影/未分类', '{"default":true}'::jsonb, true),
('国漫', 'tv', '/电视剧/国漫', '{"genre_ids":[16],"countries":["CN","TW","HK"]}'::jsonb, true),
('日番', 'tv', '/电视剧/日番', '{"genre_ids":[16],"countries":["JP"]}'::jsonb, true),
('纪录片', 'tv', '/电视剧/纪录片', '{"genre_ids":[99]}'::jsonb, true),
('儿童', 'tv', '/电视剧/儿童', '{"genre_ids":[10762]}'::jsonb, true),
('综艺', 'tv', '/电视剧/综艺', '{"genre_ids":[10764,10767]}'::jsonb, true),
('国产剧', 'tv', '/电视剧/国产剧', '{"countries":["CN","TW","HK"]}'::jsonb, true),
('欧美剧', 'tv', '/电视剧/欧美剧', '{"countries":["US","FR","GB","UK","DE","ES","IT","NL","PT","RU"]}'::jsonb, true),
('日韩剧', 'tv', '/电视剧/日韩剧', '{"countries":["JP","KP","KR","TH","IN","SG"]}'::jsonb, true),
('未分类', 'tv', '/电视剧/未分类', '{"default":true}'::jsonb, true)
),
updated AS (
    UPDATE t_media_category c
    SET target_path = d.target_path,
        match_rules = d.match_rules,
        enabled = d.enabled,
        update_time = NOW()
    FROM defaults d
    WHERE c.name = d.name AND c.media_type = d.media_type
    RETURNING c.name, c.media_type
)
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled)
SELECT name, media_type, target_path, match_rules, enabled FROM defaults v
WHERE NOT EXISTS (
    SELECT 1 FROM t_media_category
    WHERE name = v.name AND media_type = v.media_type
);
