-- 更新日期：2026-10-08；维护者：Codex。仅植入合成检查点供 SQL/EXPLAIN，不是实际 full 成功凭证。
\set ON_ERROR_STOP on
BEGIN;
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF; END $$;
INSERT INTO t_share_export_source_state(source_file_id,share_id,media_id,work_key,file_version,share_version,available,cancelled,episode_sig,export_keys,state)
 SELECT f.id,f.share_id,f.media_id,m.work_key,f.version,s.version,f.available,s.share_cancelled,
 COALESCE((SELECT string_agg(e.season_number::text||':'||e.episode_number::text,',' ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),''),
 jsonb_build_array('phase02-owned-'||(m.id-200000000)),CASE WHEN f.available AND NOT s.share_cancelled AND f.status='identified' THEN 'active' ELSE 'stale' END
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id
 WHERE f.id BETWEEN 300000001 AND 300100000 ON CONFLICT(source_file_id) DO NOTHING;
SELECT share_export_enqueue(ARRAY(SELECT work_key FROM t_share_media ORDER BY work_key),'synthetic-seed');
COMMIT;
ANALYZE t_share_export_source_state;
ANALYZE t_share_export_dirty_work;
SELECT 'synthetic fixture counts',
 (SELECT count(*) FROM t_share_media) AS works,
 (SELECT count(*) FROM t_share_media_file) AS files,
 (SELECT count(*) FROM t_share_export_source_state) AS checkpoints,
 (SELECT count(*) FROM t_strm_export_state) AS outputs;
