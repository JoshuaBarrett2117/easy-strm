-- 更新日期：2026-10-08；维护者：Codex。v43 下真实 100k 文件 DML 的 producer/锁/WAL 测量，全部回滚。
\set ON_ERROR_STOP on
\timing on
BEGIN;
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF; END $$;
SELECT pg_current_wal_insert_lsn() AS wal_before \gset
EXPLAIN (ANALYZE, BUFFERS, WAL, FORMAT JSON)
UPDATE t_share_media_file SET file_size=file_size+1 WHERE id BETWEEN 300000001 AND 300100000;
DO $$ BEGIN
 IF (SELECT count(*) FROM t_share_media_file)<>100000 OR (SELECT count(*) FROM t_share_export_dirty_work WHERE revision>acked_revision)<>20000
 THEN RAISE EXCEPTION 'bulk producer did not enqueue all 20k works from 100k files'; END IF;
END $$;
SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(), :'wal_before') AS measured_cluster_wal_bytes;
SELECT mode,count(*) FROM pg_locks WHERE pid=pg_backend_pid() GROUP BY mode ORDER BY mode;
ROLLBACK;
