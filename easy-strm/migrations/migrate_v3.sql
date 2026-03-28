-- Easy-STRM V3.0 数据库迁移脚本
-- 执行方式: psql -h 192.168.31.12 -p 15432 -U joshua -d easy_strm -f migrate_v3.sql
-- 认证凭据请通过企业内部安全通道获取

-- =============================================
-- 1. 扩展 t_cloud_115 表结构（新字段）
-- =============================================
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS account_type VARCHAR(20) DEFAULT 'resource';
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS quota_used BIGINT DEFAULT 0;
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS priority INT DEFAULT 5;
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active';
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS cooling_start_time TIMESTAMP NULL;

-- 添加 CHECK 约束
ALTER TABLE t_cloud_115 DROP CONSTRAINT IF EXISTS chk_account_type;
ALTER TABLE t_cloud_115 ADD CONSTRAINT chk_account_type
  CHECK (account_type IN ('resource', 'vip', 'both'));

ALTER TABLE t_cloud_115 DROP CONSTRAINT IF EXISTS chk_115_status;
ALTER TABLE t_cloud_115 ADD CONSTRAINT chk_115_status
  CHECK (status IN ('active', 'cooling', 'disabled'));

-- =============================================
-- 2. 扩展 t_strm_config 表结构（同步策略）
-- =============================================
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS sync_mode VARCHAR(20) DEFAULT 'manual';
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS source_account INT DEFAULT 0;
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS target_account INT DEFAULT 0;
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS target_directory VARCHAR(500);
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS auto_cleanup BOOLEAN DEFAULT FALSE;
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS cleanup_threshold INT DEFAULT 0;
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS cleanup_policy VARCHAR(50);
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS max_concurrency INT DEFAULT 1;

-- =============================================
-- 3. 创建通知配置表
-- =============================================
CREATE TABLE IF NOT EXISTS t_notification_config (
  id SERIAL PRIMARY KEY,
  channel VARCHAR(20) NOT NULL UNIQUE,
  config JSONB NOT NULL DEFAULT '{}',
  enabled BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- =============================================
-- 4. 创建同步任务表
-- =============================================
CREATE TABLE IF NOT EXISTS t_sync_task (
  id SERIAL PRIMARY KEY,
  task_type VARCHAR(50) NOT NULL,
  source_account_id INT NOT NULL,
  target_account_id INT NOT NULL,
  status VARCHAR(20) DEFAULT 'pending',
  progress INT DEFAULT 0,
  total_count INT DEFAULT 0,
  success_count INT DEFAULT 0,
  fail_count INT DEFAULT 0,
  error_message TEXT,
  started_at TIMESTAMP,
  finished_at TIMESTAMP,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- =============================================
-- 5. 创建播放日志表
-- =============================================
CREATE TABLE IF NOT EXISTS t_play_log (
  id SERIAL PRIMARY KEY,
  file_id VARCHAR(100) NOT NULL,
  user_id INT,
  play_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  duration INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_play_log_file_id ON t_play_log(file_id);
CREATE INDEX IF NOT EXISTS idx_play_log_play_time ON t_play_log(play_time);

-- =============================================
-- 6. 创建 SHA1 缓存表
-- =============================================
CREATE TABLE IF NOT EXISTS t_sha1_cache (
  id SERIAL PRIMARY KEY,
  sha1 VARCHAR(40) NOT NULL UNIQUE,
  file_cid VARCHAR(100),
  file_name VARCHAR(500),
  file_size BIGINT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sha1_cache_sha1 ON t_sha1_cache(sha1);
CREATE INDEX IF NOT EXISTS idx_sha1_cache_expires ON t_sha1_cache(expires_at);

-- =============================================
-- 验证迁移结果
-- =============================================
SELECT 't_cloud_115 columns:' as info;
SELECT column_name FROM information_schema.columns WHERE table_name = 't_cloud_115' ORDER BY ordinal_position;

SELECT 't_strm_config columns:' as info;
SELECT column_name FROM information_schema.columns WHERE table_name = 't_strm_config' ORDER BY ordinal_position;

SELECT 'New tables:' as info;
SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name LIKE 't_%' ORDER BY table_name;