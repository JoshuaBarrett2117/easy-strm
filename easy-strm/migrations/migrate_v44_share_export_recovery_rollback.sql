-- v44 分享库导出恢复（A2）迁移：回滚。仅由 DBA 显式执行，应用启动不得调用。
-- 更新日期：2026-10-09；维护者：DBA。回滚前 DBA 须停止新消费者并确认无运行中恢复事务。
-- 仅删除正向迁移创建的对象，按 FK 安全顺序；幂等，可重复执行。
-- 保留迁移前已存在的共享路径索引以及 v43 检查点对象，不删除任何磁盘文件。
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '30s';
DO $$
BEGIN
  IF to_regclass('public.t_share_export_recovery_fence') IS NOT NULL THEN
    ALTER TABLE t_share_export_recovery_fence DROP CONSTRAINT IF EXISTS share_export_recovery_fence_bootstrap_fk;
    ALTER TABLE t_share_export_recovery_fence DROP CONSTRAINT IF EXISTS share_export_recovery_fence_active_fk;
  END IF;
END $$;
DROP TABLE IF EXISTS t_share_export_reconciliation_action;
DROP TABLE IF EXISTS t_share_export_reconciliation_ack;
DROP TABLE IF EXISTS t_share_export_reconciliation_member;
DROP TABLE IF EXISTS t_share_export_reconciliation;
DROP TABLE IF EXISTS t_share_export_recovery_catalog;
DROP TABLE IF EXISTS t_share_export_recovery_token;
DROP TABLE IF EXISTS t_share_export_recovery_bootstrap;
DROP TABLE IF EXISTS t_share_export_recovery_fence;
DROP SEQUENCE IF EXISTS share_export_recovery_generation_seq;
DROP SEQUENCE IF EXISTS share_export_recovery_epoch_seq;
COMMIT;
