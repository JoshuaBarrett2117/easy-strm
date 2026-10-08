-- 更新日期：2026-10-08；维护者：Codex。DBA 测量 C3 单事务 fanout 时间/锁/WAL；全部回滚。
\set ON_ERROR_STOP on
\timing on
BEGIN;
SET LOCAL application_name='easy-strm-phase02-fanout';
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION '仅允许 easy_strm_test'; END IF; END $$;
SELECT config_revision,prepared_revision,config_fingerprint,baseline_state,protocol_version FROM t_share_export_consumer WHERE consumer='share:default' FOR UPDATE;
EXPLAIN (ANALYZE, BUFFERS, WAL, FORMAT TEXT)
SELECT share_export_enqueue(ARRAY(
 SELECT work_key FROM t_share_media WHERE work_key<>''
 UNION SELECT work_key FROM t_share_export_source_state WHERE work_key<>''
 UNION SELECT work_key FROM t_share_export_dirty_work ORDER BY work_key),'baseline');
UPDATE t_share_export_consumer SET config_fingerprint='synthetic-fanout-measurement',prepared_revision=config_revision,protocol_version=1,baseline_state='building',updated_at=now() WHERE consumer='share:default';
SELECT count(*) AS pending_works FROM t_share_export_dirty_work WHERE revision > acked_revision;
SELECT mode,count(*) FROM pg_locks WHERE pid=pg_backend_pid() GROUP BY mode ORDER BY mode;
ROLLBACK;
