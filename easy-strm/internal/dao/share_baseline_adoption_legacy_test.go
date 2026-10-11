package dao

const legacyShareBaselineCountsSQL = legacyShareBaselineCountsPrefixSQL + legacyShareBaselineMediaIDMatchSQL + legacyShareBaselineCountsSuffixSQL
const legacyShareBaselineBenignCountsSQL = legacyShareBaselineCountsPrefixSQL + legacyShareBaselineCountsSuffixSQL
const legacyShareBaselineMediaIDMatchSQL = ` AND (mapping.payload->>'media_id')::integer=live.id`
const legacyShareBaselineCountsPrefixSQL = `SELECT
 (SELECT count(*) FROM t_share_media_selection),
 (SELECT count(*) FROM t_strm_export_state WHERE owner_key='share:default' AND state='active'),
 (SELECT count(*) FROM t_share_export_dirty_work WHERE revision>acked_revision),
 (SELECT COALESCE(sum(revision),0) FROM t_share_export_dirty_work),
 (SELECT COALESCE(sum(selection_revision),0) FROM t_share_media_selection),
 (SELECT count(*) FROM t_share_media_selection s LEFT JOIN t_strm_export_state output
 ON output.owner_key='share:default' AND output.export_key=s.media_item_key AND output.state='active'
 LEFT JOIN t_share_strm mapping ON mapping.id::text=output.share_strm_id
 WHERE s.stable_relative_path='' OR s.stable_relative_path LIKE '/%' OR s.stable_relative_path ~ '(^|/)\.\.(/|$)'
 OR output.export_key IS NULL OR output.output_path<>rtrim($1,'/')||'/'||s.stable_relative_path
 OR COALESCE(mapping.payload->>'file_name','')='' OR COALESCE(mapping.payload->>'media_id','')='' OR COALESCE(mapping.payload->>'share_code','')=''
 OR (COALESCE(mapping.payload->>'file_id','')='' AND COALESCE(mapping.payload->>'file_path','')='')
 OR NOT EXISTS(SELECT 1 FROM t_share_media WHERE id=(mapping.payload->>'media_id')::integer)
 OR NOT EXISTS(SELECT 1` + shareSelectionLiveJoin + ` WHERE i.candidate_id=s.selected_candidate_id
 AND i.work_key=s.work_key AND i.season_number=s.season_number AND i.episode_number=s.episode_number
`
const legacyShareBaselineCountsSuffixSQL = ` AND ` + shareSelectionLivePredicate + `))
 + (SELECT count(*) FROM t_share_export_dirty_work WHERE revision<acked_revision OR work_key='' OR (revision>acked_revision AND scope<>'baseline'))
 + (SELECT count(*) FROM t_strm_export_state WHERE owner_key='share:default' AND state='active' AND (output_path='' OR COALESCE(share_strm_id,'')='' OR content_fingerprint='' OR mapping_fingerprint=''))
 + (SELECT count(*) FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id
 WHERE f.available AND NOT r.share_cancelled AND f.status='identified' AND
 (m.media_type IS NULL OR m.media_type NOT IN ('movie','tv')
 OR (m.media_type='movie' AND NOT EXISTS(SELECT 1 FROM t_share_media_selection s WHERE s.work_key=m.work_key AND s.season_number=0 AND s.episode_number=0))
 OR (m.media_type='tv' AND (NOT EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id)
 OR EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id AND NOT EXISTS(
 SELECT 1 FROM t_share_media_selection s WHERE s.work_key=m.work_key AND s.season_number=e.season_number AND s.episode_number=e.episode_number))))))`

const legacyShareBaselineMappingsSQL = `SELECT selected.id,selected.file_name,selected.file_id,selected.media_id,selected.url,selected.password,mapping.id::text,mapping.payload
 FROM t_share_media_selection s
 JOIN t_strm_export_state output ON output.owner_key='share:default' AND output.export_key=s.media_item_key AND output.state='active'
 JOIN t_share_strm mapping ON mapping.id::text=output.share_strm_id
 JOIN LATERAL (SELECT f.id,f.file_name,f.file_id,f.media_id,r.url,r.password` + shareSelectionLiveJoin + `
 WHERE i.candidate_id=s.selected_candidate_id AND i.work_key=s.work_key
 AND i.season_number=s.season_number AND i.episode_number=s.episode_number
 AND ` + shareSelectionLivePredicate + `) selected ON true`

const legacyShareBaselineSourcesSQL = `INSERT INTO t_share_export_source_state AS source
 (source_file_id,share_id,media_id,work_key,file_version,share_version,available,cancelled,episode_sig,export_keys,last_seen_run_id,state)
 SELECT f.id,f.share_id,f.media_id,m.work_key,f.version,r.version,f.available,r.share_cancelled,
 COALESCE((SELECT string_agg(e.season_number::text||':'||e.episode_number::text,',' ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),''),
 COALESCE(keys.export_keys,'[]'::jsonb),$1,CASE WHEN f.available AND NOT r.share_cancelled AND f.status='identified' THEN 'active' ELSE 'stale' END
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record r ON r.id=f.share_id
 LEFT JOIN LATERAL (SELECT jsonb_agg(s.media_item_key ORDER BY s.media_item_key) export_keys
 FROM t_share_media_selection s WHERE f.available AND NOT r.share_cancelled AND f.status='identified' AND s.work_key=m.work_key
 AND ((m.media_type='movie' AND s.season_number=0 AND s.episode_number=0)
 OR (m.media_type='tv' AND EXISTS(SELECT 1 FROM t_share_media_file_episode e
 WHERE e.file_id=f.id AND e.season_number=s.season_number AND e.episode_number=s.episode_number)))) keys ON true
 ORDER BY f.id` + shareExportSourcesConflictSQL
