-- 2026-09-13 Codex：修复普通 STRM 识别缓存漏存海报产生的历史空值。
WITH latest_tmdb AS (
    SELECT DISTINCT ON (tmdb_id, media_type) tmdb_id, media_type, poster_path
    FROM t_tmdb_cache
    WHERE COALESCE(poster_path, '') <> ''
    ORDER BY tmdb_id, media_type, update_time DESC, id DESC
)
UPDATE t_identify_cache identify
SET poster_path = latest.poster_path, updated_at = NOW()
FROM latest_tmdb latest
WHERE COALESCE(identify.poster_path, '') = ''
  AND identify.tmdb_id = latest.tmdb_id
  AND identify.media_type = latest.media_type;

-- 资源名仍为原文件名的历史记录可以无歧义地按原有文件名哈希补回。
UPDATE t_strm_playback_record playback
SET poster = identify.poster_path
FROM t_identify_cache identify
WHERE COALESCE(playback.poster, '') = ''
  AND COALESCE(identify.poster_path, '') <> ''
  AND identify.file_hash = md5(lower(playback.resource_name));

-- 显式 TMDB/MetaTube 策略的哈希带来源前缀，按唯一文件名补齐这部分历史记录。
WITH unique_file_poster AS (
    SELECT lower(trim(file_name)) AS normalized_name, min(poster_path) AS poster
    FROM t_identify_cache
    WHERE COALESCE(file_name, '') <> '' AND COALESCE(poster_path, '') <> ''
    GROUP BY lower(trim(file_name))
    HAVING count(DISTINCT poster_path) = 1
)
UPDATE t_strm_playback_record playback
SET poster = candidate.poster
FROM unique_file_poster candidate
WHERE COALESCE(playback.poster, '') = ''
  AND lower(trim(playback.resource_name)) = candidate.normalized_name;

-- 已记录为作品标题时，仅在同名识别结果只有一个海报的情况下补回，避免同名作品串图。
WITH unique_title_poster AS (
    SELECT lower(trim(title)) AS normalized_title, min(poster_path) AS poster
    FROM t_identify_cache
    WHERE COALESCE(title, '') <> '' AND COALESCE(poster_path, '') <> ''
    GROUP BY lower(trim(title))
    HAVING count(DISTINCT poster_path) = 1
)
UPDATE t_strm_playback_record playback
SET poster = candidate.poster
FROM unique_title_poster candidate
WHERE COALESCE(playback.poster, '') = ''
  AND lower(trim(playback.resource_name)) = candidate.normalized_title;
