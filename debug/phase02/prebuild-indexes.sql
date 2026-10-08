-- 更新日期：2026-10-08；维护者：Codex。仅 DBA 手工、事务外执行；不得用 psql -1 或嵌入启动链。
\set ON_ERROR_STOP on
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF; END $$;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_strm_export_state_path ON t_strm_export_state(output_path);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_strm_export_history_path ON t_strm_export_history(output_path);
SELECT indexrelid::regclass,indisvalid,indisready,pg_get_indexdef(indexrelid) FROM pg_index
 WHERE indexrelid IN ('idx_strm_export_state_path'::regclass,'idx_strm_export_history_path'::regclass);
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_index WHERE indexrelid IN ('idx_strm_export_state_path'::regclass,'idx_strm_export_history_path'::regclass) AND (NOT indisvalid OR NOT indisready))
 THEN RAISE EXCEPTION 'invalid concurrent index: DBA must review/rebuild before migration'; END IF;
END $$;
