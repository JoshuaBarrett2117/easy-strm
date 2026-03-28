-- Easy-STRM V5.0 数据库迁移脚本
-- 执行方式: psql -h <host> -p <port> -U <user> -d <dbname> -f migrate_v5.sql
-- 认证凭据请通过企业内部安全通道获取

-- =============================================
-- 1. 扩展 t_system_config 表结构（Alist配置）
-- =============================================
-- 确保 alist_url 和 alist_token 配置项存在
INSERT INTO t_system_config (config_key, config_val, create_time, update_time)
VALUES ('alist_url', '', NOW(), NOW())
ON CONFLICT (config_key) DO NOTHING;

INSERT INTO t_system_config (config_key, config_val, create_time, update_time)
VALUES ('alist_token', '', NOW(), NOW())
ON CONFLICT (config_key) DO NOTHING;

-- =============================================
-- 验证迁移结果
-- =============================================
SELECT 't_system_config alist columns:' as info;
SELECT config_key, config_val
FROM t_system_config
WHERE config_key IN ('alist_url', 'alist_token')
ORDER BY config_key;