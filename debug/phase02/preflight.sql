-- 更新日期：2026-10-08；维护者：Codex。DBA 专用只读前置核验；不替代独立目标/备份确认。
\set ON_ERROR_STOP on
BEGIN READ ONLY;
DO $$ BEGIN
 IF current_database()<>'easy_strm_test' OR current_schema()<>'public' THEN RAISE EXCEPTION 'only easy_strm_test/public'; END IF;
 IF current_setting('server_version_num')::integer<>170006 THEN RAISE EXCEPTION 'DBA approval targets PostgreSQL 17.6; re-review other versions'; END IF;
 IF EXISTS(SELECT 1 FROM t_share_record WHERE name NOT LIKE 'phase02-synthetic-%')
 OR EXISTS(SELECT 1 FROM t_share_media WHERE title NOT LIKE 'phase02-synthetic-%')
 OR EXISTS(SELECT 1 FROM t_share_media_file WHERE file_id NOT LIKE 'phase02-synthetic-%')
 OR EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path NOT LIKE '/phase02-synthetic/%')
 OR EXISTS(SELECT 1 FROM t_strm_export_history WHERE output_path NOT LIKE '/phase02-synthetic/%')
 OR EXISTS(SELECT 1 FROM t_strm_file WHERE local_strm_path NOT LIKE '/phase02-synthetic/%')
 OR EXISTS(SELECT 1 FROM t_share_strm WHERE payload->>'phase02_synthetic' IS DISTINCT FROM 'true')
 THEN RAISE EXCEPTION 'non-harness business/output data: stop; never clean automatically'; END IF;
END $$;
SELECT current_database(),current_schema(),current_setting('server_version');
SELECT tablename,indexname,indexdef FROM pg_indexes WHERE schemaname=current_schema() AND tablename IN
 ('t_share_media','t_share_media_file','t_share_media_file_episode','t_strm_export_state','t_strm_export_history') ORDER BY tablename,indexname;
SELECT conrelid::regclass,conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid IN
 ('t_share_media_file'::regclass,'t_share_media_file_episode'::regclass,'t_strm_file'::regclass) ORDER BY conrelid,conname;
COMMIT;
