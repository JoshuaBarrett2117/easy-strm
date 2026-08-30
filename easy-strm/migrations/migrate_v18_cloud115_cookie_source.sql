-- 更新日期：2026-08-30
-- 执行者：Codex
-- 为 115 Cookie 保存可读的人工获取来源；历史空值由 API 根据 Cookie UID 动态推断。
ALTER TABLE t_cloud_115
    ADD COLUMN IF NOT EXISTS cookie_source VARCHAR(100) NOT NULL DEFAULT '';

UPDATE t_cloud_115 SET cookie_source = '' WHERE cookie_source IS NULL;

ALTER TABLE t_cloud_115
    ALTER COLUMN cookie_source SET DEFAULT '',
    ALTER COLUMN cookie_source SET NOT NULL;

COMMENT ON COLUMN t_cloud_115.cookie_source IS '115 Cookie 获取来源，例如手动录入、微信小程序或支付宝小程序';
