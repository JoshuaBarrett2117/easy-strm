-- 更新日期：2026-09-08；执行者：Codex
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS work_key TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS tmdb_id BIGINT;
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS library_media_type TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS media_year INTEGER;
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS genre_ids INTEGER[] NOT NULL DEFAULT '{}';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS country_codes TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS rating NUMERIC;
-- 在原始结果写入的同一事务内维护索引投影，覆盖所有识别入口与状态清理。
CREATE OR REPLACE FUNCTION project_share_library() RETURNS TRIGGER AS $$
BEGIN
 NEW.work_key := ''; NEW.tmdb_id := NULL; NEW.title := ''; NEW.library_media_type := '';
 NEW.media_year := NULL; NEW.genre_ids := '{}'; NEW.country_codes := '{}'; NEW.rating := NULL;
 IF NEW.status='identified' AND NEW.result->>'success'='true' AND NEW.file_name NOT LIKE '%**%' AND NEW.file_name NOT LIKE '%＊＊%' THEN
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
CREATE TRIGGER project_share_library_trigger BEFORE INSERT OR UPDATE OF result,status,file_name ON t_share_media FOR EACH ROW EXECUTE FUNCTION project_share_library();
UPDATE t_share_media SET result=result WHERE work_key='' AND status='identified';
CREATE INDEX IF NOT EXISTS idx_share_library_work ON t_share_media(work_key);
CREATE INDEX IF NOT EXISTS idx_share_library_tmdb ON t_share_media(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_share_library_year ON t_share_media(media_year);
CREATE INDEX IF NOT EXISTS idx_share_library_rating ON t_share_media(rating);
CREATE INDEX IF NOT EXISTS idx_share_library_created ON t_share_media(created_at);
CREATE INDEX IF NOT EXISTS idx_share_library_title ON t_share_media(title);
CREATE INDEX IF NOT EXISTS idx_share_library_genres ON t_share_media USING gin(genre_ids);
CREATE INDEX IF NOT EXISTS idx_share_library_countries ON t_share_media USING gin(country_codes);
