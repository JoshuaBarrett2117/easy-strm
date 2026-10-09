-- v45 分享库导出恢复（A2）语义守卫与最小权限边界：回滚。仅由 DBA 显式执行。
-- 更新日期：2026-10-09；维护者：DBA。仅移除 v45 正向创建的对象；幂等，可重复执行。
-- 不删除 v44 账本表/序列，不删除任何磁盘文件，不改变业务数据。
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '30s';

-- 1) 触发器
DO $drop_trg$
DECLARE t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
      't_share_export_recovery_fence','t_share_export_recovery_bootstrap',
      't_share_export_recovery_token','t_share_export_recovery_catalog',
      't_share_export_reconciliation','t_share_export_reconciliation_member',
      't_share_export_reconciliation_ack','t_share_export_reconciliation_action']
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t||'_a2_no_delete', t);
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t||'_a2_no_truncate', t);
  END LOOP;
END $drop_trg$;

DROP TRIGGER IF EXISTS t_share_export_recovery_token_a2_monotonic ON t_share_export_recovery_token;
DROP TRIGGER IF EXISTS t_share_export_recovery_catalog_a2_interval ON t_share_export_recovery_catalog;

-- 2) 守卫函数
DROP FUNCTION IF EXISTS share_export_recovery_token_guard();
DROP FUNCTION IF EXISTS share_export_recovery_catalog_guard();
DROP FUNCTION IF EXISTS share_export_recovery_ledger_no_delete();
DROP FUNCTION IF EXISTS share_export_recovery_ledger_no_truncate();

-- 3) 运行角色（先撤销其全部被授权限，再删除）
DO $role$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'share_export_recovery_app') THEN
    DROP OWNED BY share_export_recovery_app;
    DROP ROLE share_export_recovery_app;
  END IF;
END $role$;

COMMIT;
