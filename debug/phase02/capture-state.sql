-- 更新日期：2026-10-08；维护者：Codex。只输出稳定哈希，不输出配置值或凭据；用于复跑/回滚的状态比较。
\set ON_ERROR_STOP on
\pset format unaligned
\pset tuples_only on
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION 'only isolated easy_strm_test'; END IF; END $$;
SELECT 'record',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY id),'')) FROM t_share_record AS item;
SELECT 'media',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY id),'')) FROM t_share_media AS item;
SELECT 'file',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY id),'')) FROM t_share_media_file AS item;
SELECT 'episode',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY file_id,season_number,episode_number),'')) FROM t_share_media_file_episode AS item;
SELECT 'output',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY owner_key,export_key),'')) FROM t_strm_export_state AS item;
SELECT 'history',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY owner_key,output_path),'')) FROM t_strm_export_history AS item;
SELECT 'legacy-file',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY id),'')) FROM t_strm_file AS item;
SELECT 'playback',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY id),'')) FROM t_share_strm AS item;
\if :include_checkpoints
SELECT 'source-state',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY source_file_id),'')) FROM t_share_export_source_state AS item;
SELECT 'dirty',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY work_key),'')) FROM t_share_export_dirty_work AS item;
SELECT 'consumer',count(*),md5(COALESCE(string_agg(to_jsonb(item)::text,'' ORDER BY consumer),'')) FROM t_share_export_consumer AS item;
\endif
COMMIT;
