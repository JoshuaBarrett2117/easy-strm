-- 更新日期：2026-09-12；维护者：Codex。
-- 必须在事务及停写窗口内运行；候选/失败记录留在文件表，只有识别成功的媒体进入实体表。
DO $split$
BEGIN
 IF to_regclass('t_share_media_file') IS NULL THEN
  LOCK TABLE t_share_media IN ACCESS EXCLUSIVE MODE;
  ALTER TABLE t_share_media RENAME TO t_share_media_file;
  DROP TRIGGER IF EXISTS project_share_library_trigger ON t_share_media_file;
  CREATE TABLE t_share_media (LIKE t_share_media_file INCLUDING ALL);
  CREATE SEQUENCE t_share_media_entity_id_seq OWNED BY t_share_media.id;
  ALTER TABLE t_share_media ALTER COLUMN id SET DEFAULT nextval('t_share_media_entity_id_seq');
  INSERT INTO t_share_media
  SELECT * FROM t_share_media_file WHERE status='identified' AND result->>'success'='true';
  PERFORM setval('t_share_media_entity_id_seq',COALESCE((SELECT max(id) FROM t_share_media),1),EXISTS(SELECT 1 FROM t_share_media));
  ALTER TABLE t_share_media ADD CONSTRAINT share_media_record_fk FOREIGN KEY(share_id) REFERENCES t_share_record(id) ON DELETE CASCADE;
  ALTER TABLE t_share_media DROP COLUMN file_name;
  ALTER TABLE t_share_media_file ADD COLUMN media_id INTEGER;
  UPDATE t_share_media_file SET media_id=id WHERE status='identified' AND result->>'success'='true';
 ELSE
  IF NOT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid='t_share_media_file'::regclass AND attname='media_id' AND NOT attisdropped)
     OR EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid='t_share_media'::regclass AND attname='file_name' AND NOT attisdropped) THEN
   RAISE EXCEPTION '检测到未完成的旧版 v32，请回滚旧版迁移后再升级';
  END IF;
  -- 兼容曾将候选字段错误移出文件表的 v32 草案，恢复失败重试所需状态。
  ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS metadata_source VARCHAR(20) NOT NULL DEFAULT 'auto';
  ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'pending';
  ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS result JSONB;
  ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS error TEXT NOT NULL DEFAULT '';
  ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
  UPDATE t_share_media_file f SET metadata_source=m.metadata_source,status=m.status,result=m.result,error=m.error,version=m.version
   FROM t_share_media m WHERE m.id=f.media_id AND f.status='pending' AND f.result IS NULL;
 END IF;

 ALTER TABLE t_share_media_file ALTER COLUMN media_id DROP NOT NULL;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_share_media'::regclass AND conname='share_media_id_share_unique') THEN
  ALTER TABLE t_share_media ADD CONSTRAINT share_media_id_share_unique UNIQUE(id,share_id);
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_share_media_file'::regclass AND conname='share_media_file_owner_fk') THEN
  ALTER TABLE t_share_media_file ADD CONSTRAINT share_media_file_owner_fk FOREIGN KEY(media_id,share_id) REFERENCES t_share_media(id,share_id);
 END IF;
 CREATE INDEX IF NOT EXISTS share_media_file_owner_idx ON t_share_media_file(media_id,id);
 CREATE INDEX IF NOT EXISTS share_media_file_path_idx ON t_share_media_file(share_id,md5(file_name));
 -- 文件表保留识别过程字段，仅移除只属于媒体资源库的查询投影。
 ALTER TABLE t_share_media_file DROP COLUMN IF EXISTS work_key, DROP COLUMN IF EXISTS tmdb_id,
  DROP COLUMN IF EXISTS title, DROP COLUMN IF EXISTS library_media_type, DROP COLUMN IF EXISTS media_year,
  DROP COLUMN IF EXISTS genre_ids, DROP COLUMN IF EXISTS country_codes, DROP COLUMN IF EXISTS rating;

 -- 修复旧草案产生的未匹配实体；失败记录仍完整保留在文件表供展示和重试。
 UPDATE t_share_media_file f SET media_id=NULL FROM t_share_media m
  WHERE f.media_id=m.id AND (f.status IS DISTINCT FROM 'identified' OR f.result->>'success' IS DISTINCT FROM 'true');
 DELETE FROM t_share_media WHERE status IS DISTINCT FROM 'identified' OR result->>'success' IS DISTINCT FROM 'true';
END $split$;

CREATE OR REPLACE FUNCTION project_share_library() RETURNS TRIGGER AS $$
BEGIN
 NEW.work_key := ''; NEW.tmdb_id := NULL; NEW.title := ''; NEW.library_media_type := '';
 NEW.media_year := NULL; NEW.genre_ids := '{}'; NEW.country_codes := '{}'; NEW.rating := NULL;
 IF NEW.status='identified' AND NEW.result->>'success'='true' THEN
  NEW.library_media_type := COALESCE(NEW.result->>'media_type','');
  NEW.title := COALESCE(NEW.result->>'title','');
  -- MetaTube 使用正数合成ID，不应被当成真正TMDB身份或参与TMDB ID搜索。
  IF COALESCE(NEW.result->>'metadata_source','')<>'metatube' AND COALESCE(NEW.result->>'metadata_provider','')='' AND NEW.result->>'tmdb_id' ~ '^[0-9]{1,18}$' THEN NEW.tmdb_id := NULLIF((NEW.result->>'tmdb_id')::bigint,0); END IF;
  IF NEW.result->>'year' ~ '^[0-9]{1,4}$' THEN NEW.media_year := NULLIF((NEW.result->>'year')::integer,0); END IF;
  IF jsonb_typeof(NEW.result->'vote_average')='number' AND (NEW.result->>'vote_average')::numeric BETWEEN 0 AND 10 THEN NEW.rating := (NEW.result->>'vote_average')::numeric; END IF;
  IF jsonb_typeof(NEW.result->'genre_ids')='array' THEN
   NEW.genre_ids := ARRAY(SELECT DISTINCT v::integer FROM jsonb_array_elements_text(NEW.result->'genre_ids') v WHERE v ~ '^[0-9]{1,8}$');
  END IF;
  IF jsonb_typeof(NEW.result->'countries')='array' THEN
   NEW.country_codes := ARRAY(SELECT DISTINCT upper(v) FROM jsonb_array_elements_text(NEW.result->'countries') v WHERE v <> '');
  END IF;
  NEW.work_key := CASE WHEN NEW.tmdb_id > 0 THEN 'tmdb:'||NEW.library_media_type||':'||NEW.tmdb_id
   WHEN COALESCE(NEW.result->>'metadata_id','')<>'' AND COALESCE(NEW.result->>'metadata_source','')<>'' THEN
    jsonb_build_array(NEW.result->>'metadata_source',COALESCE(NEW.result->>'metadata_provider',''),NEW.library_media_type,NEW.result->>'metadata_id')::text
   ELSE 'row:'||NEW.id END;
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS project_share_library_trigger ON t_share_media;
CREATE TRIGGER project_share_library_trigger BEFORE INSERT OR UPDATE OF result,status ON t_share_media FOR EACH ROW EXECUTE FUNCTION project_share_library();

-- 业务列表以文件候选ID为操作主键，媒体实体字段仅在识别成功后存在。
DROP VIEW IF EXISTS v_share_media_detail;
CREATE VIEW v_share_media_detail AS
 SELECT f.id,f.id AS file_id,f.media_id,f.share_id,f.file_name,f.metadata_source,f.status,f.result,f.error,f.version,
  f.created_at,f.updated_at,m.work_key,m.tmdb_id,m.title,m.library_media_type,m.media_year,
  m.genre_ids,m.country_codes,m.rating
 FROM t_share_media_file f LEFT JOIN t_share_media m ON m.id=f.media_id;
