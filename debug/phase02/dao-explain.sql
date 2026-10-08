-- 更新日期：2026-10-08；维护者：Codex。由 Go 测试从实际 DAO 常量/构建器生成，禁止手工漂移。
\set ON_ERROR_STOP on
\pset format unaligned
\pset tuples_only on
DO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION '仅允许 easy_strm_test'; END IF; END $$;
PREPARE phase02_before_full(integer) AS
WITH stats AS (
 SELECT f.media_id,
  count(DISTINCT f.share_id) FILTER (WHERE f.available AND NOT s.share_cancelled) source_count,
  count(*) FILTER (WHERE f.available AND NOT s.share_cancelled) file_count,
  max(f.created_at) collected_at
 FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id
 WHERE f.status='identified' AND f.media_id IS NOT NULL GROUP BY f.media_id
), works AS (
 SELECT m.id,m.work_key,m.tmdb_id,m.title,m.original_title,m.media_type library_media_type,m.media_year,m.rating,
  m.result,m.poster_path,m.genre_ids,m.country_codes,st.source_count,st.file_count,st.collected_at,(st.source_count>0) available
 FROM t_share_media m JOIN stats st ON st.media_id=m.id
)  SELECT f.id,f.share_id,s.name,m.work_key,s.url,s.password,f.file_name,f.file_id,m.result || jsonb_build_object('_media_id',m.id,'_file_version',f.version,'_share_version',s.version),
	 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json),count(*) OVER()
	 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id JOIN works w ON w.work_key=m.work_key
	 WHERE w.work_key IN (SELECT work_key FROM works WHERE TRUE) AND f.available AND NOT s.share_cancelled AND f.status='identified' AND f.id>$1 ORDER BY f.id LIMIT 100;
PREPARE phase02_before_work_and_conflicts(text,integer) AS
WITH stats AS (
 SELECT f.media_id,
  count(DISTINCT f.share_id) FILTER (WHERE f.available AND NOT s.share_cancelled) source_count,
  count(*) FILTER (WHERE f.available AND NOT s.share_cancelled) file_count,
  max(f.created_at) collected_at
 FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id
 WHERE f.status='identified' AND f.media_id IS NOT NULL GROUP BY f.media_id
), works AS (
 SELECT m.id,m.work_key,m.tmdb_id,m.title,m.original_title,m.media_type library_media_type,m.media_year,m.rating,
  m.result,m.poster_path,m.genre_ids,m.country_codes,st.source_count,st.file_count,st.collected_at,(st.source_count>0) available
 FROM t_share_media m JOIN stats st ON st.media_id=m.id
)  SELECT f.id,f.share_id,s.name,m.work_key,s.url,s.password,f.file_name,f.file_id,m.result || jsonb_build_object('_media_id',m.id,'_file_version',f.version,'_share_version',s.version),
	 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json),count(*) OVER()
	 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id JOIN works w ON w.work_key=m.work_key
	 WHERE w.work_key IN (SELECT work_key FROM works WHERE TRUE AND work_key=$1) AND f.available AND NOT s.share_cancelled AND f.status='identified' AND f.id>$2 ORDER BY f.id LIMIT 100;
PREPARE phase02_before_locked_file(integer[],integer) AS
WITH stats AS (
 SELECT f.media_id,
  count(DISTINCT f.share_id) FILTER (WHERE f.available AND NOT s.share_cancelled) source_count,
  count(*) FILTER (WHERE f.available AND NOT s.share_cancelled) file_count,
  max(f.created_at) collected_at
 FROM t_share_media_file f JOIN t_share_record s ON s.id=f.share_id
 WHERE f.status='identified' AND f.media_id IS NOT NULL GROUP BY f.media_id
), works AS (
 SELECT m.id,m.work_key,m.tmdb_id,m.title,m.original_title,m.media_type library_media_type,m.media_year,m.rating,
  m.result,m.poster_path,m.genre_ids,m.country_codes,st.source_count,st.file_count,st.collected_at,(st.source_count>0) available
 FROM t_share_media m JOIN stats st ON st.media_id=m.id
)  SELECT f.id,f.share_id,s.name,m.work_key,s.url,s.password,f.file_name,f.file_id,m.result || jsonb_build_object('_media_id',m.id,'_file_version',f.version,'_share_version',s.version),
	 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json),count(*) OVER()
	 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id JOIN works w ON w.work_key=m.work_key
	 WHERE w.work_key IN (SELECT work_key FROM works WHERE TRUE) AND f.available AND NOT s.share_cancelled AND f.status='identified' AND m.work_key IN (SELECT selected_media.work_key FROM t_share_media_file selected_file JOIN t_share_media selected_media ON selected_media.id=selected_file.media_id WHERE selected_file.id=ANY($1::integer[])) AND f.id>$2 ORDER BY f.id LIMIT 100;
PREPARE phase02_after_work_and_conflicts(text,integer) AS
SELECT f.id,f.share_id,s.name,m.work_key,s.url,s.password,f.file_name,f.file_id,m.result || jsonb_build_object('_media_id',m.id,'_file_version',f.version,'_share_version',s.version),
 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json),count(*) OVER()
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id
 WHERE f.available AND NOT s.share_cancelled AND f.status='identified' AND m.work_key=$1 AND f.id>$2 ORDER BY f.id LIMIT 100;
PREPARE phase02_after_cold_work(text,integer) AS
SELECT f.id,f.share_id,s.name,m.work_key,s.url,s.password,f.file_name,f.file_id,m.result || jsonb_build_object('_media_id',m.id,'_file_version',f.version,'_share_version',s.version),
 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json),count(*) OVER()
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id
 WHERE f.available AND NOT s.share_cancelled AND f.status='identified' AND m.work_key=$1 AND f.id>$2 ORDER BY f.id LIMIT 100;
PREPARE phase02_after_locked_file(integer) AS
SELECT f.id,f.share_id,s.name,m.work_key,s.url,s.password,f.file_name,f.file_id,m.result || jsonb_build_object('_media_id',m.id,'_file_version',f.version,'_share_version',s.version),
 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number) ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json),count(*) OVER()
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id
 WHERE f.available AND NOT s.share_cancelled AND f.status='identified' AND f.id=$1;
PREPARE phase02_after_work_observation(text) AS
SELECT f.id,f.share_id,COALESCE(f.media_id,0),m.work_key,f.version,s.version,f.available,s.share_cancelled,
 COALESCE((SELECT string_agg(e.season_number::text||':'||e.episode_number::text,',' ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),''),
 CASE WHEN f.available AND NOT s.share_cancelled AND f.status='identified' THEN 'active' ELSE 'stale' END
 FROM t_share_media m JOIN t_share_media_file f ON f.media_id=m.id JOIN t_share_record s ON s.id=f.share_id WHERE m.work_key=$1 ORDER BY f.id;
PREPARE phase02_after_dirty(text[]) AS
SELECT work_key,revision,pending_export_keys FROM t_share_export_dirty_work WHERE revision > acked_revision AND NOT(work_key=ANY($1::text[])) ORDER BY updated_at,work_key LIMIT 100;
PREPARE phase02_before_check_path(text) AS
SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing' UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1;
PREPARE phase02_check_path(text) AS
SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing'
 UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1
 UNION ALL SELECT CASE WHEN strm_config_id=-1 THEN 'share:default' ELSE 'cloud115:'||strm_config_id::text END,''
 FROM t_strm_file WHERE local_strm_path=$1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path=$1);
PREPARE phase02_check_history_path(text) AS
SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing'
 UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1
 UNION ALL SELECT CASE WHEN strm_config_id=-1 THEN 'share:default' ELSE 'cloud115:'||strm_config_id::text END,''
 FROM t_strm_file WHERE local_strm_path=$1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path=$1);
PREPARE phase02_check_legacy_path(text) AS
SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing'
 UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1
 UNION ALL SELECT CASE WHEN strm_config_id=-1 THEN 'share:default' ELSE 'cloud115:'||strm_config_id::text END,''
 FROM t_strm_file WHERE local_strm_path=$1
 AND NOT EXISTS(SELECT 1 FROM t_strm_export_state WHERE output_path=$1);
\echo catalog-indexes
BEGIN;
SET LOCAL application_name='easy-strm-phase02-explain';
\echo before_full
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_full(0);
\set phase02_plan_file :explain_dir '/catalog-indexes-before_full.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_full(0)
\g :phase02_plan_file
\echo before_work_and_conflicts
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0);
\set phase02_plan_file :explain_dir '/catalog-indexes-before_work_and_conflicts.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0)
\g :phase02_plan_file
\echo before_locked_file
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_locked_file(ARRAY[300000001],300000000);
\set phase02_plan_file :explain_dir '/catalog-indexes-before_locked_file.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_locked_file(ARRAY[300000001],300000000)
\g :phase02_plan_file
\echo after_work_and_conflicts
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0);
\set phase02_plan_file :explain_dir '/catalog-indexes-after_work_and_conflicts.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0)
\g :phase02_plan_file
\echo after_cold_work
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_cold_work('["tmdb", "", "movie", "200000003"]',0);
\set phase02_plan_file :explain_dir '/catalog-indexes-after_cold_work.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_cold_work('["tmdb", "", "movie", "200000003"]',0)
\g :phase02_plan_file
\echo after_locked_file
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_locked_file(300000001);
\set phase02_plan_file :explain_dir '/catalog-indexes-after_locked_file.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_locked_file(300000001)
\g :phase02_plan_file
\echo after_work_observation
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_work_observation('["tmdb", "", "movie", "200000001"]');
\set phase02_plan_file :explain_dir '/catalog-indexes-after_work_observation.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_observation('["tmdb", "", "movie", "200000001"]')
\g :phase02_plan_file
\echo after_dirty
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_dirty(ARRAY[]::text[]);
\set phase02_plan_file :explain_dir '/catalog-indexes-after_dirty.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_dirty(ARRAY[]::text[])
\g :phase02_plan_file
\echo before_check_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm');
\set phase02_plan_file :explain_dir '/catalog-indexes-before_check_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm')
\g :phase02_plan_file
\echo check_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_check_path('/phase02-synthetic/owned-1.strm');
\set phase02_plan_file :explain_dir '/catalog-indexes-check_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_check_path('/phase02-synthetic/owned-1.strm')
\g :phase02_plan_file
\echo check_history_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_check_history_path('/phase02-synthetic/history-1.strm');
\set phase02_plan_file :explain_dir '/catalog-indexes-check_history_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_check_history_path('/phase02-synthetic/history-1.strm')
\g :phase02_plan_file
\echo check_legacy_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_check_legacy_path('/phase02-synthetic/legacy-1.strm');
\set phase02_plan_file :explain_dir '/catalog-indexes-check_legacy_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_check_legacy_path('/phase02-synthetic/legacy-1.strm')
\g :phase02_plan_file
SAVEPOINT zero_dirty;
UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]';
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_dirty(ARRAY[]::text[]);
DO $$ BEGIN IF EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE revision>acked_revision) THEN RAISE EXCEPTION 'not zero dirty'; END IF; END $$;
ROLLBACK TO SAVEPOINT zero_dirty;
SAVEPOINT old_path_indexes;
DROP INDEX IF EXISTS idx_strm_export_state_path;
DROP INDEX IF EXISTS idx_strm_export_history_path;
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm');
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/history-1.strm');
ROLLBACK TO SAVEPOINT old_path_indexes;
\echo affected-100-work-new-and-old
SELECT format('EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_and_conflicts(%L,0);',work_key) FROM t_share_media WHERE id BETWEEN 200000001 AND 200000100 ORDER BY id
\gexec
SELECT format('EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_work_and_conflicts(%L,0);',work_key) FROM t_share_media WHERE id BETWEEN 200000001 AND 200000100 ORDER BY id
\gexec
ROLLBACK;
\echo without-status-index
BEGIN;
SET LOCAL application_name='easy-strm-phase02-explain';
DROP INDEX IF EXISTS idx_share_media_status;
\echo before_full
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_full(0);
\set phase02_plan_file :explain_dir '/without-status-index-before_full.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_full(0)
\g :phase02_plan_file
\echo before_work_and_conflicts
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0);
\set phase02_plan_file :explain_dir '/without-status-index-before_work_and_conflicts.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0)
\g :phase02_plan_file
\echo before_locked_file
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_locked_file(ARRAY[300000001],300000000);
\set phase02_plan_file :explain_dir '/without-status-index-before_locked_file.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_locked_file(ARRAY[300000001],300000000)
\g :phase02_plan_file
\echo after_work_and_conflicts
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0);
\set phase02_plan_file :explain_dir '/without-status-index-after_work_and_conflicts.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_and_conflicts('["tmdb", "", "movie", "200000001"]',0)
\g :phase02_plan_file
\echo after_cold_work
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_cold_work('["tmdb", "", "movie", "200000003"]',0);
\set phase02_plan_file :explain_dir '/without-status-index-after_cold_work.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_cold_work('["tmdb", "", "movie", "200000003"]',0)
\g :phase02_plan_file
\echo after_locked_file
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_locked_file(300000001);
\set phase02_plan_file :explain_dir '/without-status-index-after_locked_file.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_locked_file(300000001)
\g :phase02_plan_file
\echo after_work_observation
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_work_observation('["tmdb", "", "movie", "200000001"]');
\set phase02_plan_file :explain_dir '/without-status-index-after_work_observation.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_observation('["tmdb", "", "movie", "200000001"]')
\g :phase02_plan_file
\echo after_dirty
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_after_dirty(ARRAY[]::text[]);
\set phase02_plan_file :explain_dir '/without-status-index-after_dirty.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_dirty(ARRAY[]::text[])
\g :phase02_plan_file
\echo before_check_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm');
\set phase02_plan_file :explain_dir '/without-status-index-before_check_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm')
\g :phase02_plan_file
\echo check_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_check_path('/phase02-synthetic/owned-1.strm');
\set phase02_plan_file :explain_dir '/without-status-index-check_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_check_path('/phase02-synthetic/owned-1.strm')
\g :phase02_plan_file
\echo check_history_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_check_history_path('/phase02-synthetic/history-1.strm');
\set phase02_plan_file :explain_dir '/without-status-index-check_history_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_check_history_path('/phase02-synthetic/history-1.strm')
\g :phase02_plan_file
\echo check_legacy_path
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_check_legacy_path('/phase02-synthetic/legacy-1.strm');
\set phase02_plan_file :explain_dir '/without-status-index-check_legacy_path.json'
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_check_legacy_path('/phase02-synthetic/legacy-1.strm')
\g :phase02_plan_file
SAVEPOINT zero_dirty;
UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]';
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_dirty(ARRAY[]::text[]);
DO $$ BEGIN IF EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE revision>acked_revision) THEN RAISE EXCEPTION 'not zero dirty'; END IF; END $$;
ROLLBACK TO SAVEPOINT zero_dirty;
SAVEPOINT old_path_indexes;
DROP INDEX IF EXISTS idx_strm_export_state_path;
DROP INDEX IF EXISTS idx_strm_export_history_path;
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm');
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/history-1.strm');
ROLLBACK TO SAVEPOINT old_path_indexes;
\echo affected-100-work-new-and-old
SELECT format('EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_and_conflicts(%L,0);',work_key) FROM t_share_media WHERE id BETWEEN 200000001 AND 200000100 ORDER BY id
\gexec
SELECT format('EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_work_and_conflicts(%L,0);',work_key) FROM t_share_media WHERE id BETWEEN 200000001 AND 200000100 ORDER BY id
\gexec
ROLLBACK;
DEALLOCATE phase02_before_full;
DEALLOCATE phase02_before_work_and_conflicts;
DEALLOCATE phase02_before_locked_file;
DEALLOCATE phase02_after_work_and_conflicts;
DEALLOCATE phase02_after_cold_work;
DEALLOCATE phase02_after_locked_file;
DEALLOCATE phase02_after_work_observation;
DEALLOCATE phase02_after_dirty;
DEALLOCATE phase02_before_check_path;
DEALLOCATE phase02_check_path;
DEALLOCATE phase02_check_history_path;
DEALLOCATE phase02_check_legacy_path;
