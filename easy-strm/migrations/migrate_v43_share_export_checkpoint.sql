-- 更新日期：2026-10-08；维护者：Codex。仅由 DBA 显式执行，应用启动不得调用。
-- 前置：v33/v34/v42 已完成；生产路径索引请先由 DBA 在事务外 CONCURRENTLY 预建。
DO $prerequisites$
DECLARE missing TEXT;
BEGIN
 SELECT string_agg(relation||'.'||column_name,', ' ORDER BY relation,column_name) INTO missing
 FROM (VALUES
  ('t_share_media_file',ARRAY['id','share_id','media_id','version','available','file_name','file_id','status','result','metadata_source','file_size','sha1','pick_code']),
  ('t_share_media',ARRAY['id','work_key','version','result','metadata_source','metadata_provider','external_id','media_type','tmdb_id','title','original_title','poster_path','media_year','genre_ids','country_codes','rating']),
  ('t_share_record',ARRAY['id','version','share_cancelled','name','url','password','media_type']),
  ('t_share_media_file_episode',ARRAY['file_id','season_number','episode_number']),
  ('t_system_config',ARRAY['id','config_key','config_val']),
  ('t_media_category',ARRAY['id','name','media_type','target_path','match_rules','enabled']),
  ('t_strm_export_state',ARRAY['owner_key','export_key','output_path','last_seen_run_id','state']),
  ('t_strm_export_history',ARRAY['owner_key','output_path']),
  ('t_strm_file',ARRAY['strm_config_id','local_strm_path']),
  ('t_share_strm',ARRAY['id','payload']),
  ('t_share_operation_queue',ARRAY['sequence','task_id','operation','status'])
 ) required(relation,columns) CROSS JOIN LATERAL unnest(columns) AS required_column(column_name)
 WHERE NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass(relation) AND attname=column_name AND attnum>0 AND NOT attisdropped);
 IF missing IS NOT NULL THEN RAISE EXCEPTION 'v43 requires reviewed v33/v34/v42 schema; missing %',missing; END IF;
END $prerequisites$;
CREATE TABLE IF NOT EXISTS t_share_export_source_state (
 source_file_id INTEGER PRIMARY KEY, share_id INTEGER NOT NULL, media_id INTEGER,
 work_key TEXT NOT NULL DEFAULT '', file_version INTEGER NOT NULL DEFAULT 0,
 share_version INTEGER NOT NULL DEFAULT 0, available BOOLEAN NOT NULL DEFAULT FALSE,
 cancelled BOOLEAN NOT NULL DEFAULT FALSE, episode_sig TEXT NOT NULL DEFAULT '',
 export_keys JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(export_keys)='array'),
 last_seen_run_id TEXT NOT NULL DEFAULT '', state TEXT NOT NULL CHECK(state IN ('active','stale')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS t_share_export_dirty_work (
 work_key TEXT PRIMARY KEY CHECK(work_key<>''), revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>=1),
 acked_revision BIGINT NOT NULL DEFAULT 0 CHECK(acked_revision>=0 AND acked_revision<=revision),
 scope TEXT NOT NULL DEFAULT 'work', pending_export_keys JSONB NOT NULL DEFAULT '[]'
 CHECK(jsonb_typeof(pending_export_keys)='array'), last_seen_run_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS t_share_export_consumer (
 consumer TEXT PRIMARY KEY CHECK(consumer='share:default'), config_revision BIGINT NOT NULL DEFAULT 1,
 prepared_revision BIGINT NOT NULL DEFAULT 0, completed_revision BIGINT NOT NULL DEFAULT 0,
 config_fingerprint TEXT NOT NULL DEFAULT '', protocol_version INTEGER NOT NULL DEFAULT 1,
 legacy_outputs_reconciled BOOLEAN NOT NULL DEFAULT FALSE,
 baseline_state TEXT NOT NULL DEFAULT 'required' CHECK(baseline_state IN ('required','building','ready')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(completed_revision>=0 AND completed_revision<=prepared_revision AND prepared_revision<=config_revision)
);
CREATE INDEX IF NOT EXISTS idx_share_export_src_work_file ON t_share_export_source_state(work_key,source_file_id);
CREATE INDEX IF NOT EXISTS idx_share_export_src_share_file ON t_share_export_source_state(share_id,source_file_id);
CREATE INDEX IF NOT EXISTS idx_share_export_src_keys_gin ON t_share_export_source_state USING gin(export_keys jsonb_path_ops) WHERE state = 'active';
CREATE INDEX IF NOT EXISTS idx_share_export_dirty_todo ON t_share_export_dirty_work(updated_at,work_key) WHERE revision > acked_revision;
CREATE INDEX IF NOT EXISTS idx_share_export_dirty_pending_gin ON t_share_export_dirty_work USING gin(pending_export_keys jsonb_path_ops) WHERE pending_export_keys <> '[]'::jsonb;
CREATE INDEX IF NOT EXISTS idx_strm_export_state_path ON t_strm_export_state(output_path);
CREATE INDEX IF NOT EXISTS idx_strm_export_history_path ON t_strm_export_history(output_path);
INSERT INTO t_share_export_consumer(consumer) VALUES('share:default') ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION share_export_require_baseline() RETURNS void LANGUAGE sql AS $$
 UPDATE t_share_export_consumer SET config_revision=config_revision+1,baseline_state='required',updated_at=now()
 WHERE consumer='share:default';
$$;

CREATE OR REPLACE FUNCTION share_export_enqueue(keys TEXT[], reason TEXT) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO t_share_export_dirty_work AS dirty(work_key,scope)
 SELECT DISTINCT work_key,reason FROM unnest(keys) work_key WHERE COALESCE(work_key,'')<>'' ORDER BY work_key
 ON CONFLICT(work_key) DO UPDATE SET revision=dirty.revision+1,scope=EXCLUDED.scope,updated_at=now();
END $$;

CREATE OR REPLACE FUNCTION share_export_checkpoint_invalidated() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE destructive BOOLEAN;
BEGIN
 IF TG_OP='TRUNCATE' THEN
  destructive := TRUE;
 ELSE
  SELECT EXISTS(SELECT 1 FROM old_rows) INTO destructive;
 END IF;
 IF destructive THEN
  UPDATE t_share_export_consumer SET config_revision=config_revision+1,baseline_state='required',legacy_outputs_reconciled=FALSE,updated_at=now()
  WHERE consumer='share:default';
 END IF;
 RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION share_export_produce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 changed_sql TEXT; compare_sql TEXT; join_sql TEXT; refs_sql TEXT; source_match_sql TEXT; keys TEXT[];
 unresolved BOOLEAN; relevant BOOLEAN;
BEGIN
 IF TG_OP='TRUNCATE' THEN
  PERFORM share_export_require_baseline();
  RETURN NULL;
 END IF;
 IF TG_OP='INSERT' THEN
  changed_sql := 'SELECT to_jsonb(n) value FROM new_rows n';
 ELSIF TG_OP='DELETE' THEN
  changed_sql := 'SELECT to_jsonb(o) value FROM old_rows o';
 ELSE
  join_sql := TG_ARGV[0];
  compare_sql := TG_ARGV[1];
  changed_sql := format('SELECT to_jsonb(o) value FROM old_rows o FULL JOIN new_rows n ON %s WHERE %s UNION ALL SELECT to_jsonb(n) FROM old_rows o FULL JOIN new_rows n ON %s WHERE %s',join_sql,compare_sql,join_sql,compare_sql);
 END IF;
 IF TG_TABLE_NAME='t_system_config' THEN
  EXECUTE 'WITH changed AS ('||changed_sql||') SELECT EXISTS(SELECT 1 FROM changed WHERE value->>''config_key'' IN (''share_strm_settings'',''movie_naming_template'',''tv_naming_template''))' INTO relevant;
  IF relevant THEN PERFORM share_export_require_baseline(); END IF;
  RETURN NULL;
 ELSIF TG_TABLE_NAME='t_media_category' THEN
  EXECUTE 'SELECT EXISTS(SELECT 1 FROM ('||changed_sql||') changed)' INTO relevant;
  IF relevant THEN PERFORM share_export_require_baseline(); END IF;
  RETURN NULL;
 ELSIF TG_TABLE_NAME='t_strm_export_state' THEN
  IF TG_OP='INSERT' THEN RETURN NULL; END IF;
  IF TG_OP='UPDATE' THEN
   SELECT EXISTS(SELECT 1 FROM old_rows o JOIN new_rows n USING(owner_key,export_key)
    WHERE n.owner_key='share:default' AND n.state='missing' AND o.state IS DISTINCT FROM n.state) INTO relevant;
  ELSE
   SELECT EXISTS(SELECT 1 FROM old_rows WHERE owner_key='share:default') INTO relevant;
  END IF;
  IF relevant THEN PERFORM share_export_require_baseline(); END IF;
  RETURN NULL;
 END IF;
 IF TG_TABLE_NAME='t_share_media_file' THEN
  source_match_sql := 'src.source_file_id=(c.value->>''id'')::integer';
  refs_sql := 'SELECT COALESCE(m.work_key,src.work_key) work_key FROM changed c LEFT JOIN t_share_media m ON m.id=(c.value->>''media_id'')::integer LEFT JOIN t_share_export_source_state src ON src.source_file_id=(c.value->>''id'')::integer WHERE c.value->>''media_id'' IS NOT NULL OR src.source_file_id IS NOT NULL UNION ALL SELECT src.work_key FROM changed c JOIN t_share_export_source_state src ON src.source_file_id=(c.value->>''id'')::integer';
 ELSIF TG_TABLE_NAME='t_share_media' THEN
  source_match_sql := 'src.media_id=(c.value->>''id'')::integer';
  refs_sql := 'SELECT value->>''work_key'' work_key FROM changed UNION ALL SELECT src.work_key FROM changed c JOIN t_share_export_source_state src ON src.media_id=(c.value->>''id'')::integer';
 ELSIF TG_TABLE_NAME='t_share_record' THEN
  source_match_sql := 'src.share_id=(c.value->>''id'')::integer';
  refs_sql := 'SELECT m.work_key FROM changed c JOIN t_share_media_file f ON f.share_id=(c.value->>''id'')::integer LEFT JOIN t_share_media m ON m.id=f.media_id UNION ALL SELECT src.work_key FROM changed c JOIN t_share_export_source_state src ON src.share_id=(c.value->>''id'')::integer';
 ELSE
  source_match_sql := 'src.source_file_id=(c.value->>''file_id'')::integer';
  refs_sql := 'SELECT COALESCE(m.work_key,src.work_key) work_key FROM changed c LEFT JOIN t_share_media_file f ON f.id=(c.value->>''file_id'')::integer LEFT JOIN t_share_media m ON m.id=f.media_id LEFT JOIN t_share_export_source_state src ON src.source_file_id=(c.value->>''file_id'')::integer UNION ALL SELECT src.work_key FROM changed c JOIN t_share_export_source_state src ON src.source_file_id=(c.value->>''file_id'')::integer';
 END IF;
 EXECUTE 'WITH changed AS ('||changed_sql||'), refs AS ('||refs_sql||') SELECT array_agg(DISTINCT work_key ORDER BY work_key) FILTER(WHERE COALESCE(work_key,'''')<>''''),COALESCE(bool_or(COALESCE(work_key,'''')=''''),false) FROM refs' INTO keys,unresolved;
 IF unresolved THEN
  PERFORM share_export_require_baseline();
  UPDATE t_share_export_consumer SET legacy_outputs_reconciled=FALSE WHERE consumer='share:default';
 END IF;
 IF keys IS NOT NULL THEN PERFORM share_export_enqueue(keys,TG_TABLE_NAME||':'||TG_OP); END IF;
 EXECUTE 'WITH changed AS ('||changed_sql||'), evidence AS (
  SELECT DISTINCT src.work_key,src.export_keys FROM changed c JOIN t_share_export_source_state src ON '||source_match_sql||'
 ), locked AS (
  SELECT dirty.work_key FROM t_share_export_dirty_work dirty WHERE dirty.work_key=ANY($1) AND EXISTS(SELECT 1 FROM evidence WHERE evidence.work_key=dirty.work_key)
  ORDER BY dirty.work_key FOR UPDATE OF dirty
 ) UPDATE t_share_export_dirty_work dirty SET pending_export_keys=(
  SELECT COALESCE(jsonb_agg(key ORDER BY key),''[]''::jsonb) FROM (
   SELECT jsonb_array_elements_text(dirty.pending_export_keys) key
   UNION SELECT jsonb_array_elements_text(evidence.export_keys) FROM evidence WHERE evidence.work_key=dirty.work_key
  ) known
 ) FROM locked WHERE dirty.work_key=locked.work_key' USING keys;
 RETURN NULL;
END $$;

DO $install$
DECLARE relation TEXT; join_sql TEXT; compare_sql TEXT; operation TEXT; transition_sql TEXT;
BEGIN
 FOR relation,join_sql,compare_sql IN SELECT * FROM (VALUES
  ('t_share_media_file','o.id=n.id','o.id IS NULL OR n.id IS NULL OR ROW(o.version,o.available,o.media_id,o.file_name,o.file_id,o.status,o.result,o.metadata_source,o.share_id,o.file_size,o.sha1,o.pick_code) IS DISTINCT FROM ROW(n.version,n.available,n.media_id,n.file_name,n.file_id,n.status,n.result,n.metadata_source,n.share_id,n.file_size,n.sha1,n.pick_code)'),
  ('t_share_media','o.id=n.id','o.id IS NULL OR n.id IS NULL OR ROW(o.work_key,o.version,o.result,o.metadata_source,o.metadata_provider,o.external_id,o.media_type,o.tmdb_id,o.title,o.original_title,o.poster_path,o.media_year,o.genre_ids,o.country_codes,o.rating) IS DISTINCT FROM ROW(n.work_key,n.version,n.result,n.metadata_source,n.metadata_provider,n.external_id,n.media_type,n.tmdb_id,n.title,n.original_title,n.poster_path,n.media_year,n.genre_ids,n.country_codes,n.rating)'),
  ('t_share_record','o.id=n.id','o.id IS NULL OR n.id IS NULL OR ROW(o.version,o.share_cancelled,o.name,o.url,o.password,o.media_type) IS DISTINCT FROM ROW(n.version,n.share_cancelled,n.name,n.url,n.password,n.media_type)'),
  ('t_share_media_file_episode','o.file_id=n.file_id AND o.season_number=n.season_number AND o.episode_number=n.episode_number','o.file_id IS NULL OR n.file_id IS NULL'),
  ('t_system_config','o.id=n.id','o.id IS NULL OR n.id IS NULL OR ROW(o.config_key,o.config_val) IS DISTINCT FROM ROW(n.config_key,n.config_val)'),
  ('t_media_category','o.id=n.id','o.id IS NULL OR n.id IS NULL OR ROW(o.name,o.media_type,o.target_path,o.match_rules,o.enabled) IS DISTINCT FROM ROW(n.name,n.media_type,n.target_path,n.match_rules,n.enabled)'),
  ('t_strm_export_state','o.owner_key=n.owner_key AND o.export_key=n.export_key','o.owner_key IS NULL OR n.owner_key IS NULL OR o.state IS DISTINCT FROM n.state')
 ) spec LOOP
  FOREACH operation IN ARRAY ARRAY['INSERT','UPDATE','DELETE','TRUNCATE'] LOOP
   transition_sql := CASE operation WHEN 'INSERT' THEN 'REFERENCING NEW TABLE AS new_rows' WHEN 'DELETE' THEN 'REFERENCING OLD TABLE AS old_rows' WHEN 'UPDATE' THEN 'REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows' ELSE '' END;
   EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I','share_export_'||lower(operation),relation);
   EXECUTE format('CREATE TRIGGER %I AFTER %s ON %I %s FOR EACH STATEMENT EXECUTE FUNCTION share_export_produce(%L,%L)','share_export_'||lower(operation),operation,relation,transition_sql,join_sql,compare_sql);
  END LOOP;
 END LOOP;
 FOREACH relation IN ARRAY ARRAY['t_share_export_source_state','t_share_export_dirty_work'] LOOP
  FOREACH operation IN ARRAY ARRAY['DELETE','TRUNCATE'] LOOP
   transition_sql := CASE operation WHEN 'DELETE' THEN 'REFERENCING OLD TABLE AS old_rows' ELSE '' END;
   EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I','share_export_'||lower(operation),relation);
   EXECUTE format('CREATE TRIGGER %I AFTER %s ON %I %s FOR EACH STATEMENT EXECUTE FUNCTION share_export_checkpoint_invalidated()','share_export_'||lower(operation),operation,relation,transition_sql);
  END LOOP;
 END LOOP;
END $install$;
