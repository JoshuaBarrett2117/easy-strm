-- 更新日期：2026-10-08；维护者：Codex。验证 rollback 后首次安装不能伪装可信。
\set ON_ERROR_STOP on
DO $$ BEGIN
 IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF;
 IF NOT EXISTS(SELECT 1 FROM t_share_export_consumer WHERE config_revision=1 AND prepared_revision=0 AND completed_revision=0 AND baseline_state='required' AND NOT legacy_outputs_reconciled)
 OR EXISTS(SELECT 1 FROM t_share_export_source_state) OR EXISTS(SELECT 1 FROM t_share_export_dirty_work)
 THEN RAISE EXCEPTION 'reinstallation is not a fresh untrusted baseline'; END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid IN ('t_share_export_source_state'::regclass,'t_share_export_dirty_work'::regclass,'t_share_export_consumer'::regclass))
 THEN RAISE EXCEPTION 'checkpoint tables must have no business FK'; END IF;
END $$;
