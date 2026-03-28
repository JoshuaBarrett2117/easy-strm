-- Easy-STRM V4.0 数据库迁移脚本
-- 执行方式: psql -h <host> -p <port> -U <user> -d <dbname> -f migrate_v4.sql
-- 认证凭据请通过企业内部安全通道获取

-- =============================================
-- 1. 扩展 t_cloud_115 表结构（秒传方式）
-- =============================================
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS transfer_method VARCHAR(50) DEFAULT '';

-- 添加 CHECK 约束
ALTER TABLE t_cloud_115 DROP CONSTRAINT IF EXISTS chk_transfer_method;
ALTER TABLE t_cloud_115 ADD CONSTRAINT chk_transfer_method
  CHECK (transfer_method IN ('115driver', 'go115', 'alist', 'elevengo', ''));

-- =============================================
-- 2. 扩展 t_cloud_115 表结构（alist配置）
-- =============================================
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS alist_url VARCHAR(500);
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS alist_token VARCHAR(255);

-- =============================================
-- 验证迁移结果
-- =============================================
SELECT 't_cloud_115 transfer_method columns:' as info;
SELECT column_name, data_type, column_default
FROM information_schema.columns
WHERE table_name = 't_cloud_115'
  AND column_name IN ('transfer_method', 'alist_url', 'alist_token')
ORDER BY ordinal_position;
