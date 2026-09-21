-- 更新日期：2026-09-21；执行者：Codex
-- 分享 ISO 的裸季号由分享识别器处理，不写入通用文件名规则，避免无 episode 组的规则使整组配置失效。
DO $migration$
BEGIN
    INSERT INTO t_system_config(config_key, config_val)
    VALUES ('share_disc_recognition_version', '1')
    ON CONFLICT (config_key) DO NOTHING;
END $migration$;
