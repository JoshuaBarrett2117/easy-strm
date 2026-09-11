-- 更新日期：2026-09-10；执行者：Codex
-- 识别测试的完整初始化规则；已有自定义规则、优先级及启用状态保持不变。
DO $migration$
DECLARE
    defaults jsonb := $rules$[
  {
    "id": "tv_sxe_compact",
    "name": "紧凑型 SxxExx 合并剧集",
    "description": "支持中文标题紧贴 S01E01，以及 S01E01E02 连续合并集。",
    "pattern": "(?i)^(?P<title>.+?)\\s*S(?P<season>\\d{1,2})E(?P<episode>\\d{1,3})(?P<episodes>(?:\\s*E\\d{1,3})*)\\s*$",
    "media_type": "tv", "example": "举重妖精金福珠S01E01.mkv", "default_season": 0, "enabled": true, "priority": 9
  },
  {
    "id": "tv_sxe",
    "name": "SxxExx 标准剧集",
    "description": "支持片名紧贴S01E01、特别篇S00及S01E01E02合并集；episodes保留后续全部集号。",
    "pattern": "(?i)^(?P<title>.+?)\\s*S(?P<season>\\d{1,2})E(?P<episode>\\d{1,3})(?P<episodes>(?:\\s*E\\d{1,3})*)(?:\\s+.*)?$",
    "media_type": "tv",
    "example": "你是谁 - S01E01E02 .mp4",
    "default_season": 0,
    "enabled": true,
    "priority": 10
  },
  {
    "id": "tv_x",
    "name": "数字 x 数字剧集",
    "description": "适用于 1x02、02x15 等命名。",
    "pattern": "(?i)^(?P<title>.+?)\\s+(?P<season>\\d{1,2})x(?P<episode>\\d{1,3})(?:\\s+.*)?$",
    "media_type": "tv",
    "example": "Breaking Bad - 1x02 - Cat's in the Bag.mkv",
    "default_season": 0,
    "enabled": true,
    "priority": 20
  },
  {
    "id": "tv_words",
    "name": "Season Episode 剧集",
    "description": "适用于英文 Season 2 Episode 3 命名。",
    "pattern": "(?i)^(?P<title>.+?)\\s+Season\\s*(?P<season>\\d{1,2})\\s+Episode\\s*(?P<episode>\\d{1,3})(?:\\s+.*)?$",
    "media_type": "tv",
    "example": "The Office Season 2 Episode 3.mp4",
    "default_season": 0,
    "enabled": true,
    "priority": 30
  },
  {
    "id": "tv_chinese",
    "name": "中文季集",
    "description": "适用于第2季第5集等中文命名，季数需使用阿拉伯数字。",
    "pattern": "^(?P<title>.+?)\\s*第(?P<season>\\d{1,2})季\\s*第(?P<episode>\\d{1,3})集(?:\\s+.*)?$",
    "media_type": "tv",
    "example": "庆余年 第2季 第05集.mp4",
    "default_season": 0,
    "enabled": true,
    "priority": 40
  },
  {
    "id": "tv_episode",
    "name": "EP 单集",
    "description": "仅包含集数时按默认季识别，默认第 1 季。",
    "pattern": "(?i)^(?P<title>.+?)\\s+(?:EP?|Episode)\\s*(?P<episode>\\d{1,3})(?:\\s+.*)?$",
    "media_type": "tv",
    "example": "葬送的芙莉莲 EP08.mp4",
    "default_season": 1,
    "enabled": true,
    "priority": 50
  },
  {
    "id": "movie_year",
    "name": "电影标题与年份",
    "description": "适用于标题后包含四位发行年份的电影命名。",
    "pattern": "^(?P<title>.+?)\\s+(?P<year>19\\d{2}|20\\d{2})(?:\\s+.*)?$",
    "media_type": "movie",
    "example": "Inception.2010.1080p.BluRay.mkv",
    "default_season": 0,
    "enabled": true,
    "priority": 60
  },
  {
    "id": "anime_number",
    "name": "动漫纯集数",
    "description": "可能与年份或标题数字冲突，确认资源命名稳定后再启用。",
    "pattern": "^(?P<title>.+?)\\s+(?P<episode>\\d{1,4})(?:v\\d+)?(?:\\s+.*)?$",
    "media_type": "tv",
    "example": "海贼王 - 1120.mp4",
    "default_season": 1,
    "enabled": false,
    "priority": 70
  }
]$rules$::jsonb;
    stored jsonb;
BEGIN
    INSERT INTO t_system_config(config_key,config_val)
    VALUES ('filename_recognition_rules', defaults::text)
    ON CONFLICT(config_key) DO NOTHING;
    BEGIN
        SELECT config_val::jsonb INTO stored FROM t_system_config
        WHERE config_key='filename_recognition_rules' FOR UPDATE;
    EXCEPTION WHEN invalid_text_representation THEN
        RETURN;
    END;
    IF jsonb_typeof(stored) = 'array' THEN
        UPDATE t_system_config SET config_val=(
            SELECT jsonb_agg(CASE WHEN rule->>'id'='tv_sxe' AND rule->>'pattern'=$legacy$(?i)^(?P<title>.+?)\s+S(?P<season>\d{1,2})E(?P<episode>\d{1,3})(?:\s+.*)?$$legacy$
                THEN rule || jsonb_build_object('pattern',defaults->0->>'pattern','description',defaults->0->>'description')
                ELSE rule END ORDER BY ordinal)::text
            FROM jsonb_array_elements(stored) WITH ORDINALITY AS item(rule,ordinal)
        ), update_time=NOW()
        WHERE config_key='filename_recognition_rules'
          AND EXISTS (SELECT 1 FROM jsonb_array_elements(stored) AS item(rule)
              WHERE rule->>'id'='tv_sxe' AND rule->>'pattern'=$legacy$(?i)^(?P<title>.+?)\s+S(?P<season>\d{1,2})E(?P<episode>\d{1,3})(?:\s+.*)?$$legacy$);
    END IF;
END $migration$;
