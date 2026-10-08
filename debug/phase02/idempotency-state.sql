-- 更新日期：2026-10-08；维护者：Codex。DBA 隔离库模拟 ready 后又收到 producer；不是应用建立基线。
\set ON_ERROR_STOP on
BEGIN;
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF; END $$;
UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]';
UPDATE t_share_export_consumer SET config_revision=17,prepared_revision=17,completed_revision=17,
 config_fingerprint='synthetic-idempotency-keep',baseline_state='ready',legacy_outputs_reconciled=TRUE;
INSERT INTO t_share_export_dirty_work(work_key,revision,acked_revision,pending_export_keys,last_seen_run_id)
 VALUES('phase02-synthetic-sentinel',9,8,'["phase02-synthetic-half-success"]','synthetic-before-repeat')
 ON CONFLICT(work_key) DO NOTHING;
COMMIT;
