-- 更新日期：2026-09-26；维护者：Codex
-- 仅追加缺失 ID，不覆盖已有表达式、开关和优先级；重放不重复追加。
-- 不推断裸数字、日期期号或原盘碟号。当前引擎无上下文条件，此类输入保留待核对。
DO $migration$
DECLARE
    additions jsonb := $rules$[
  {
    "id": "review_season_episode",
    "name": "分隔式季集号",
    "pattern": "(?i)^(?P<title>.+?)\\s+S(?P<season>\\d{1,3})\\s+EP?\\s*(?P<episode>0*[1-9]\\d{0,2})(?:\\s+.*)?$",
    "example": "Douluo.Dalu.S01.EP072.2019.mkv",
    "priority": 11,
    "media_type": "tv",
    "enabled": true,
    "default_season": 1,
    "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。"
  },
  {
    "id": "review_chinese_episode",
    "name": "中文单集及话数",
    "pattern": "^(?P<title>[^第]+?)\\s*第(?P<episode>0*[1-9]\\d{0,2})[集话話](?:\\s+.*)?$",
    "example": "塞外奇侠传第15集.mp4",
    "priority": 41,
    "media_type": "tv",
    "enabled": true,
    "default_season": 1,
    "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。"
  },
  {
    "id": "review_compact_ep",
    "name": "紧凑或带年份的EP单集",
    "pattern": "(?i)^(?P<title>.+?)(?:\\s+(?P<year>19\\d{2}|20\\d{2}))?\\s*EP\\s*(?P<episode>0*[1-9]\\d{0,2})(?:\\s+.*)?$",
    "example": "如来神掌.2002.EP02.DVDRip.mkv",
    "priority": 49,
    "media_type": "tv",
    "enabled": true,
    "default_season": 1,
    "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。"
  },
  {
    "id": "review_fansub_episode",
    "name": "字幕组片名与方括号集号",
    "pattern": "^\\[[^\\[\\]]+\\]\\s*\\[(?P<title>[^\\[\\]]+)\\]\\s*\\[(?P<episode>0*[1-9]\\d{0,2})\\](?:\\s*\\[[^\\[\\]]*\\])*\\s*$",
    "example": "[UHA-WINGS][Kimetsu no Yaiba][01][x264 1080p][CHS].mp4",
    "priority": 42,
    "media_type": "tv",
    "enabled": true,
    "default_season": 1,
    "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。"
  },
  {
    "id": "review_parenthesis_episode",
    "name": "中文片名与括号集号",
    "pattern": "^(?P<title>[\\p{Han}]{2,})\\s*[（(](?P<episode>0*[1-9]\\d{0,2})[）)]$",
    "example": "九阴真经 (18).mkv",
    "priority": 43,
    "media_type": "tv",
    "enabled": true,
    "default_season": 1,
    "description": "手动核对失败样本补充；仅解析明确命名，仍需数据源核验作品。"
  }
]$rules$::jsonb;
    stored jsonb;
    merged jsonb;
BEGIN
    SELECT config_val::jsonb INTO stored FROM t_system_config
    WHERE config_key='filename_recognition_rules' FOR UPDATE;
    IF stored IS NULL OR jsonb_typeof(stored) <> 'array' THEN
        RAISE EXCEPTION 'filename_recognition_rules 必须先初始化为规则数组';
    END IF;
    SELECT stored || COALESCE(jsonb_agg(rule ORDER BY ordinal), '[]'::jsonb)
    INTO merged FROM jsonb_array_elements(additions) WITH ORDINALITY AS a(rule,ordinal)
    WHERE NOT EXISTS (
        SELECT 1 FROM jsonb_array_elements(stored) AS existing(rule)
        WHERE existing.rule->>'id'=a.rule->>'id'
    );
    IF jsonb_array_length(merged) > 50 THEN
        RAISE EXCEPTION '追加识别规则后超过 50 条，请先整理自定义规则';
    END IF;
    IF merged IS DISTINCT FROM stored THEN
        UPDATE t_system_config SET config_val=merged::text, update_time=NOW()
        WHERE config_key='filename_recognition_rules';
    END IF;
END $migration$;
