-- 更新日期：2026-10-08；维护者：Codex。DBA 专用；全部业务变化在最后 ROLLBACK。
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL application_name='easy-strm-phase02-assertions';
DO $$ BEGIN
 IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION '仅允许 easy_strm_test'; END IF;
 IF (SELECT count(*) FROM t_share_media)<>20000 OR (SELECT count(*) FROM t_share_media_file)<>100000
 THEN RAISE EXCEPTION '先应用 v43 并灌入 seed.sql'; END IF;
 IF (SELECT count(*) FROM pg_trigger WHERE tgname IN ('share_export_insert','share_export_update','share_export_delete','share_export_truncate') AND NOT tgisinternal AND tgenabled='O')<>32
 THEN RAISE EXCEPTION 'producer 数目/启用状态不符'; END IF;
END $$;
CREATE TEMP TABLE phase02_assertion_notes(message TEXT) ON COMMIT DROP;
CREATE FUNCTION pg_temp.revision(work TEXT) RETURNS bigint LANGUAGE sql AS $$
 SELECT revision FROM t_share_export_dirty_work WHERE work_key=work;
$$;
CREATE FUNCTION pg_temp.assert_bumped(work TEXT, previous BIGINT) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF COALESCE(pg_temp.revision(work),0)<=COALESCE(previous,0) THEN RAISE EXCEPTION '作品未投递：%',work; END IF;
END $$;

SAVEPOINT file_case;
DO $file_changes$
DECLARE work TEXT; previous BIGINT; statement TEXT; old_key TEXT; next_work TEXT; next_previous BIGINT; config_before BIGINT;
BEGIN
 SELECT work_key INTO work FROM t_share_media WHERE id=200000001;
 SELECT work_key INTO next_work FROM t_share_media WHERE id=200000002;
 FOREACH statement IN ARRAY ARRAY[
  'version=version+1','available=NOT available','file_name=file_name||''.changed.mkv''','file_id=file_id||''-changed''',
  'status=''failed''','result=COALESCE(result,''{}''::jsonb)||''{"synthetic_change":true}''::jsonb',
  'metadata_source=''auto''','share_id=200000002','file_size=file_size+1','sha1=''synthetic-only''','pick_code=''synthetic-only'''
 ] LOOP
  previous:=pg_temp.revision(work);
  EXECUTE 'UPDATE t_share_media_file SET '||statement||' WHERE id=300000001';
  PERFORM pg_temp.assert_bumped(work,previous);
 END LOOP;
 previous:=pg_temp.revision(work);
 UPDATE t_share_media_file SET updated_at=now(),last_seen_at=now(),last_seen_scan_token='synthetic-scan-only',error='synthetic-diagnostic-only' WHERE id=300000001;
 IF pg_temp.revision(work) IS DISTINCT FROM previous THEN RAISE EXCEPTION '纯扫描/诊断更新时间错误投递'; END IF;
 SELECT export_keys->>0 INTO old_key FROM t_share_export_source_state WHERE source_file_id=300000001;
 previous:=pg_temp.revision(work); next_previous:=pg_temp.revision(next_work);
 UPDATE t_share_media_file SET media_id=200000002 WHERE id=300000001;
 PERFORM pg_temp.assert_bumped(work,previous); PERFORM pg_temp.assert_bumped(next_work,next_previous);
 UPDATE t_share_export_source_state SET work_key=next_work,export_keys='["synthetic-new-key"]' WHERE source_file_id=300000001;
 IF NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE work_key=work AND pending_export_keys @> to_jsonb(old_key)) THEN RAISE EXCEPTION 'A→B 后丢失 A 旧键'; END IF;
 previous:=pg_temp.revision(next_work);
 UPDATE t_share_media_file SET media_id=NULL WHERE id=300000001;
 PERFORM pg_temp.assert_bumped(next_work,previous);
 previous:=pg_temp.revision(next_work);
 DELETE FROM t_share_media_file WHERE id=300000001;
 PERFORM pg_temp.assert_bumped(next_work,previous);
 IF NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE source_file_id=300000001) THEN RAISE EXCEPTION '删除来源级联删除了检查点'; END IF;
 SELECT config_revision INTO config_before FROM t_share_export_consumer;
 INSERT INTO t_share_media_file(id,share_id,file_id,file_name,status,media_id) VALUES(300100001,200000001,'phase02-synthetic-unidentified','phase02-synthetic/new.mkv','pending',NULL);
 IF (SELECT config_revision FROM t_share_export_consumer)<>config_before THEN RAISE EXCEPTION '新未识别候选不应全局失效'; END IF;
 UPDATE t_share_media_file SET media_id=200000002,status='identified' WHERE id=300100001;
 PERFORM pg_temp.assert_bumped(next_work,previous);
END $file_changes$;
ROLLBACK TO SAVEPOINT file_case;

SAVEPOINT episode_case;
DO $episode_changes$
DECLARE work TEXT; previous BIGINT;
BEGIN
 SELECT work_key INTO work FROM t_share_media WHERE id=200000002;
 previous:=pg_temp.revision(work);
 INSERT INTO t_share_media_file_episode VALUES(300000002,3,99);
 PERFORM pg_temp.assert_bumped(work,previous);
 previous:=pg_temp.revision(work);
 UPDATE t_share_media_file_episode SET season_number=4,episode_number=100 WHERE file_id=300000002 AND season_number=3 AND episode_number=99;
 PERFORM pg_temp.assert_bumped(work,previous);
 previous:=pg_temp.revision(work);
 UPDATE t_share_media_file_episode SET file_id=300020002 WHERE file_id=300000002 AND season_number=4 AND episode_number=100;
 PERFORM pg_temp.assert_bumped(work,previous);
 previous:=pg_temp.revision(work);
 DELETE FROM t_share_media_file_episode WHERE file_id=300020002 AND season_number=4 AND episode_number=100;
 PERFORM pg_temp.assert_bumped(work,previous);
END $episode_changes$;
ROLLBACK TO SAVEPOINT episode_case;

SAVEPOINT share_case;
DO $share_changes$
DECLARE work TEXT; previous BIGINT; statement TEXT;
BEGIN
 SELECT work_key INTO work FROM t_share_media m JOIN t_share_media_file f ON f.media_id=m.id WHERE f.share_id=200000007 LIMIT 1;
 FOREACH statement IN ARRAY ARRAY['version=version+1','share_cancelled=NOT share_cancelled','name=name||''-changed''','url=url||''-changed''','password=''synthetic-placeholder''','media_type=''movie'''] LOOP
  previous:=pg_temp.revision(work);
  EXECUTE 'UPDATE t_share_record SET '||statement||' WHERE id=200000007';
  PERFORM pg_temp.assert_bumped(work,previous);
 END LOOP;
 previous:=pg_temp.revision(work);
 UPDATE t_share_record SET updated_at=now(),note='synthetic-diagnostic' WHERE id=200000007;
 IF pg_temp.revision(work) IS DISTINCT FROM previous THEN RAISE EXCEPTION '分享纯诊断字段错误投递'; END IF;
 previous:=pg_temp.revision(work);
 DELETE FROM t_share_record WHERE id=200000007;
 PERFORM pg_temp.assert_bumped(work,previous);
 IF NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE share_id=200000007) THEN RAISE EXCEPTION '级联删除丢失 share 检查点'; END IF;
END $share_changes$;
ROLLBACK TO SAVEPOINT share_case;

SAVEPOINT media_case;
DO $media_changes$
DECLARE work TEXT; previous BIGINT; statement TEXT; next_work TEXT;
BEGIN
 SELECT work_key INTO work FROM t_share_media WHERE id=200000003;
 FOREACH statement IN ARRAY ARRAY[
  'version=version+1','title=title||''-changed''','original_title=original_title||''-changed''','media_year=media_year+1',
  'rating=rating+0.1','genre_ids=ARRAY[18,35]','country_codes=ARRAY[''ZZ'',''XY'']','poster_path=''/synthetic''',
  'result=result||''{"synthetic_change":true}''::jsonb','metadata_source=''synthetic''','metadata_provider=''synthetic''',
  'external_id=external_id||''-changed''','media_type=''tv''','tmdb_id=tmdb_id+1000000'
 ] LOOP
  previous:=pg_temp.revision(work);
  EXECUTE 'UPDATE t_share_media SET '||statement||' WHERE id=200000003';
  PERFORM pg_temp.assert_bumped(work,previous);
 END LOOP;
 previous:=pg_temp.revision(work);
 UPDATE t_share_media SET updated_at=now(),error='synthetic-diagnostic' WHERE id=200000003;
 IF pg_temp.revision(work) IS DISTINCT FROM previous THEN RAISE EXCEPTION '媒体纯诊断字段错误投递'; END IF;
 previous:=pg_temp.revision(work); next_work:='phase02-synthetic-moved-work';
 UPDATE t_share_media SET work_key=next_work WHERE id=200000003;
 PERFORM pg_temp.assert_bumped(work,previous); PERFORM pg_temp.assert_bumped(next_work,0);
 previous:=pg_temp.revision(next_work);
 DELETE FROM t_share_media WHERE id=200000003;
 PERFORM pg_temp.assert_bumped(next_work,previous);
 IF NOT EXISTS(SELECT 1 FROM t_share_export_source_state WHERE work_key=work) THEN RAISE EXCEPTION 'media 删除丢失 OLD 证据'; END IF;
END $media_changes$;
ROLLBACK TO SAVEPOINT media_case;

SAVEPOINT configuration_case;
DO $configuration_changes$
DECLARE previous BIGINT; key TEXT; changed INTEGER;
BEGIN
 FOREACH key IN ARRAY ARRAY['share_strm_settings','movie_naming_template','tv_naming_template'] LOOP
  SELECT config_revision INTO previous FROM t_share_export_consumer;
  INSERT INTO t_system_config(config_key,config_val) VALUES(key,'synthetic-placeholder') ON CONFLICT(config_key) DO UPDATE SET config_val=EXCLUDED.config_val;
  IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '配置未失效：%',key; END IF;
  SELECT config_revision INTO previous FROM t_share_export_consumer;
  UPDATE t_system_config SET config_val=config_val,update_time=now() WHERE config_key=key;
  IF (SELECT config_revision FROM t_share_export_consumer)<>previous THEN RAISE EXCEPTION '同值配置错误失效'; END IF;
  DELETE FROM t_system_config WHERE config_key=key;
  IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '配置删除未失效'; END IF;
 END LOOP;
 SELECT config_revision INTO previous FROM t_share_export_consumer;
 INSERT INTO t_system_config(config_key,config_val) VALUES('phase02_unrelated','synthetic-placeholder') ON CONFLICT(config_key) DO UPDATE SET config_val=EXCLUDED.config_val;
 IF (SELECT config_revision FROM t_share_export_consumer)<>previous THEN RAISE EXCEPTION '非白名单配置错误失效'; END IF;
 INSERT INTO t_media_category(id,name,media_type,target_path,match_rules,enabled) VALUES(200000001,'phase02-synthetic','movie','/synthetic','{}',false);
 IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '分类插入未失效'; END IF;
 SELECT config_revision INTO previous FROM t_share_export_consumer;
 UPDATE t_media_category SET name=name,update_time=now() WHERE id=200000001;
 IF (SELECT config_revision FROM t_share_export_consumer)<>previous THEN RAISE EXCEPTION '分类同值更新时间错误失效'; END IF;
 UPDATE t_media_category SET enabled=true,target_path='/synthetic-changed',match_rules='{"default":true}' WHERE id=200000001;
 IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '分类编辑未失效'; END IF;
 SELECT config_revision INTO previous FROM t_share_export_consumer;
 DELETE FROM t_media_category WHERE id=200000001;
 IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '分类删除未失效'; END IF;
 SELECT config_revision INTO previous FROM t_share_export_consumer;
 INSERT INTO t_strm_export_state(owner_key,export_key,output_path) VALUES('share:default','phase02-new-output','/phase02-synthetic/new.strm');
 UPDATE t_strm_export_state SET last_seen_run_id='synthetic-refresh' WHERE export_key='phase02-new-output';
 IF (SELECT config_revision FROM t_share_export_consumer)<>previous THEN RAISE EXCEPTION '正常输出 Save 递归失效'; END IF;
 UPDATE t_strm_export_state SET state='missing' WHERE export_key='phase02-new-output';
 IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '输出清空未失效'; END IF;
 SELECT config_revision INTO previous FROM t_share_export_consumer;
 DELETE FROM t_strm_export_state WHERE export_key='phase02-new-output';
 IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION '输出删除未失效'; END IF;
END $configuration_changes$;
ROLLBACK TO SAVEPOINT configuration_case;

SAVEPOINT truncate_case;
DO $truncate_changes$
DECLARE relation TEXT; previous BIGINT;
BEGIN
 FOREACH relation IN ARRAY ARRAY['t_share_media_file_episode','t_share_media_file','t_share_media','t_share_record','t_strm_export_state'] LOOP
  SELECT config_revision INTO previous FROM t_share_export_consumer;
  EXECUTE format('TRUNCATE TABLE %I CASCADE',relation);
  IF (SELECT config_revision FROM t_share_export_consumer)<=previous THEN RAISE EXCEPTION 'TRUNCATE 未置 required：%',relation; END IF;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM t_share_export_source_state) OR NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work) THEN RAISE EXCEPTION 'TRUNCATE 级联删除了恢复证据'; END IF;
END $truncate_changes$;
ROLLBACK TO SAVEPOINT truncate_case;

DO $cas$
DECLARE work TEXT; previous BIGINT; changed INTEGER;
BEGIN
 SELECT work_key INTO work FROM t_share_media WHERE id=200000004;
 PERFORM share_export_enqueue(ARRAY[work],'synthetic-CAS');
 previous:=pg_temp.revision(work);
 UPDATE t_share_export_dirty_work SET pending_export_keys=(SELECT jsonb_agg(key ORDER BY key) FROM (SELECT jsonb_array_elements_text(pending_export_keys) key UNION SELECT jsonb_array_elements_text('["synthetic-plan-before-failure"]'::jsonb)) keys) WHERE work_key=work AND revision=previous;
 PERFORM share_export_enqueue(ARRAY[work],'synthetic-concurrent-bump');
 UPDATE t_share_export_dirty_work SET acked_revision=revision,last_seen_run_id='synthetic-old-consumer',pending_export_keys='[]',updated_at=now() WHERE work_key=work AND revision=previous AND acked_revision<>revision;
 GET DIAGNOSTICS changed=ROW_COUNT;
 IF changed<>0 THEN RAISE EXCEPTION '旧 revision 误 ack'; END IF;
 IF NOT EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE work_key=work AND revision > acked_revision AND pending_export_keys @> '"synthetic-plan-before-failure"'::jsonb) THEN RAISE EXCEPTION 'bump 丢失 pending/待办'; END IF;
 UPDATE t_share_export_dirty_work SET acked_revision=revision,last_seen_run_id='synthetic-current-consumer',pending_export_keys='[]',updated_at=now() WHERE work_key=work AND revision=previous+1 AND acked_revision<>revision;
 GET DIAGNOSTICS changed=ROW_COUNT;
 IF changed<>1 THEN RAISE EXCEPTION '最新 revision 无法 ack'; END IF;
 PERFORM share_export_enqueue(ARRAY[work],'synthetic-replay');
 IF pg_temp.revision(work)<>previous+2 THEN RAISE EXCEPTION 'revision 被重置（ABA）'; END IF;
END $cas$;
SAVEPOINT checkpoint_guard;
DO $destruction$
DECLARE relation TEXT; operation TEXT; previous BIGINT;
BEGIN
 FOREACH relation IN ARRAY ARRAY['t_share_export_source_state','t_share_export_dirty_work'] LOOP
  FOREACH operation IN ARRAY ARRAY['DELETE','TRUNCATE'] LOOP
   UPDATE t_share_export_consumer SET legacy_outputs_reconciled=TRUE;
   SELECT config_revision INTO previous FROM t_share_export_consumer;
   IF operation='DELETE' THEN
    EXECUTE format('DELETE FROM %I',relation);
   ELSE
    EXECUTE format('TRUNCATE %I',relation);
   END IF;
   IF (SELECT config_revision FROM t_share_export_consumer)<=previous
    OR EXISTS(SELECT 1 FROM t_share_export_consumer WHERE legacy_outputs_reconciled OR baseline_state<>'required')
   THEN RAISE EXCEPTION 'checkpoint destruction guard failed: % %',relation,operation; END IF;
  END LOOP;
 END LOOP;
END $destruction$;
ROLLBACK TO SAVEPOINT checkpoint_guard;

SAVEPOINT narrow_pending;
DO $pending_scope$
DECLARE work TEXT; sibling INTEGER;
BEGIN
 SELECT work_key INTO work FROM t_share_export_source_state WHERE source_file_id=300000001;
 SELECT source_file_id INTO sibling FROM t_share_export_source_state WHERE work_key=work AND source_file_id<>300000001 LIMIT 1;
 IF sibling IS NULL THEN RAISE EXCEPTION 'seed lacks sibling'; END IF;
 UPDATE t_share_export_source_state SET export_keys='["phase02-sibling-unrelated-key"]' WHERE source_file_id=sibling;
 UPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]' WHERE work_key=work;
 UPDATE t_share_media_file SET media_id=200000002 WHERE id=300000001;
 IF EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE work_key=work AND pending_export_keys @> '"phase02-sibling-unrelated-key"'::jsonb)
 THEN RAISE EXCEPTION 'R1: unrelated source keys were unioned'; END IF;
END $pending_scope$;
ROLLBACK TO SAVEPOINT narrow_pending;
\echo phase02 transactional producer / pending / CAS assertions passed; rolling back all data changes
ROLLBACK;
