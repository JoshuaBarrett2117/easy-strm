-- 更新日期：2026-09-27；维护者：Codex
-- 只修正历史宽泛数字集号表达式；保留用户改写过的规则、启用状态及优先级。
DO $migration$
DECLARE
    stored jsonb;
    replacement text := $pattern$(?i)^(?P<title>[^\[\]【】()（）0-9]+?)\s+(?P<episode>0*[1-9]\d{0,2})(?:\s+(?:2160p|1080p|720p|480p)(?:\s+.*)?)?$$pattern$;
BEGIN
    SELECT config_val::jsonb INTO stored FROM t_system_config
    WHERE config_key='filename_recognition_rules' FOR UPDATE;
    IF stored IS NULL OR jsonb_typeof(stored) <> 'array' THEN
        RAISE EXCEPTION 'filename_recognition_rules 必须先初始化为规则数组';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(stored) AS item(rule)
        WHERE rule->>'id'='tv_number_episode'
          AND rule->>'pattern'=$legacy$^(?P<title>.+?)\s+(?P<episode>\d{1,3})(?:\s+.*)?$$legacy$
    ) THEN
        UPDATE t_system_config SET config_val=(
            SELECT jsonb_agg(CASE
                WHEN rule->>'id'='tv_number_episode'
                 AND rule->>'pattern'=$legacy$^(?P<title>.+?)\s+(?P<episode>\d{1,3})(?:\s+.*)?$$legacy$
                THEN rule || jsonb_build_object('pattern', replacement,
                    'description', '仅识别纯文本片名后的独立集号，可接明确画质标记；排除声道、年份、方括号原盘信息和数字片名。')
                ELSE rule END ORDER BY ordinal)::text
            FROM jsonb_array_elements(stored) WITH ORDINALITY AS item(rule,ordinal)
        ), update_time=NOW() WHERE config_key='filename_recognition_rules';
    END IF;
END $migration$;
