-- 更新日期：2026-10-08；维护者：Codex。两种作品提交顺序的真实 SQL 恢复证据；全部回滚。
\set ON_ERROR_STOP on
BEGIN;
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF; END $$;
SAVEPOINT original;
UPDATE t_share_media_file SET media_id=200000009 WHERE id=300000001;
UPDATE t_share_export_source_state SET media_id=200000009,work_key=(SELECT work_key FROM t_share_media WHERE id=200000009),export_keys='["synthetic-B-first-key"]' WHERE source_file_id=300000001;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE work_key=(SELECT work_key FROM t_share_media WHERE id=200000001) AND pending_export_keys @> '"phase02-owned-1"'::jsonb)
 THEN RAISE EXCEPTION 'B-first overwrote OLD A evidence'; END IF;
END $$;
UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]' WHERE work_key=(SELECT work_key FROM t_share_media WHERE id=200000001);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE source_file_id=300000001 AND work_key=(SELECT work_key FROM t_share_media WHERE id=200000009) AND state='active')
 THEN RAISE EXCEPTION 'A completion overwrote B current checkpoint'; END IF;
END $$;
ROLLBACK TO SAVEPOINT original;
UPDATE t_share_media_file SET media_id=200000009 WHERE id=300000001;
UPDATE t_share_export_source_state SET state='stale' WHERE source_file_id=300000001 AND work_key=(SELECT work_key FROM t_share_media WHERE id=200000001);
UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]' WHERE work_key=(SELECT work_key FROM t_share_media WHERE id=200000001);
UPDATE t_share_export_source_state SET media_id=200000009,work_key=(SELECT work_key FROM t_share_media WHERE id=200000009),export_keys='["synthetic-A-first-key"]',state='active' WHERE source_file_id=300000001;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE source_file_id=300000001 AND work_key=(SELECT work_key FROM t_share_media WHERE id=200000009) AND state='active')
 THEN RAISE EXCEPTION 'A-first did not converge to B'; END IF;
END $$;
ROLLBACK;
