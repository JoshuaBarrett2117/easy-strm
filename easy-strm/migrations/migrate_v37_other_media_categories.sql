-- 为已有数据库补充覆盖范围更完整的分类兜底。
-- 这些规则位于未分类之前：已识别但不属于现有地区/语言集合的媒体进入“其他”，
-- 只有没有可用分类信息或规则明确无法归类时才保留在“未分类”。
INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled)
SELECT '其他电影', 'movie', '/电影/其他电影', '{"default":true}'::jsonb, true
WHERE NOT EXISTS (
    SELECT 1 FROM t_media_category WHERE name = '其他电影' AND media_type = 'movie'
);

INSERT INTO t_media_category (name, media_type, target_path, match_rules, enabled)
SELECT '其他剧集', 'tv', '/电视剧/其他剧集', '{"default":true}'::jsonb, true
WHERE NOT EXISTS (
    SELECT 1 FROM t_media_category WHERE name = '其他剧集' AND media_type = 'tv'
);
