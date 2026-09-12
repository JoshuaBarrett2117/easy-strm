-- 更新日期：2026-09-12；维护者：Codex。
-- v33：分享文件与全局媒体主数据分离；可重复执行。
DROP VIEW IF EXISTS v_share_media_detail;
DROP TRIGGER IF EXISTS project_share_library_trigger ON t_share_media;
DROP FUNCTION IF EXISTS project_share_library();

ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS file_id TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS file_size BIGINT NOT NULL DEFAULT 0;
ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS sha1 TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS pick_code TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS available BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP;
ALTER TABLE t_share_media_file ADD COLUMN IF NOT EXISTS last_seen_scan_token TEXT NOT NULL DEFAULT '';

ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS metadata_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS external_id TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS original_title TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_media ADD COLUMN IF NOT EXISTS poster_path TEXT NOT NULL DEFAULT '';

-- 历史版本把媒体目录当候选写入文件表；v33 只保留真实视频文件语义。
UPDATE t_share_media_file SET status='ignored',error='历史目录或非视频记录',media_id=NULL
 WHERE file_name !~* '\.(mp4|mkv|avi|mov|wmv|flv|webm|m4v|ts|m2ts|rm|rmvb|mpg|mpeg|iso)$';

UPDATE t_share_media SET
 metadata_source=CASE WHEN COALESCE(result->>'metadata_source','')<>'' THEN result->>'metadata_source'
                      WHEN result->>'tmdb_id' ~ '^[1-9][0-9]{0,17}$' THEN 'tmdb' ELSE metadata_source END,
 metadata_provider=COALESCE(result->>'metadata_provider',''),
 external_id=CASE WHEN COALESCE(result->>'metadata_id','')<>'' THEN result->>'metadata_id'
                  ELSE COALESCE(NULLIF(result->>'tmdb_id','0'),'') END,
 media_type=COALESCE(result->>'media_type',library_media_type,''),
 original_title=COALESCE(result->>'original_title',''),
 poster_path=COALESCE(result->>'poster_path',''),
 title=COALESCE(NULLIF(result->>'title',''),title),
 media_year=CASE WHEN result->>'year' ~ '^[1-9][0-9]{0,3}$' THEN (result->>'year')::integer ELSE media_year END;
-- PostgreSQL 的同一条 UPDATE 会从旧行读取右侧表达式，因此身份字段归一化完成后再生成资源库键。
UPDATE t_share_media SET work_key=jsonb_build_array(metadata_source,metadata_provider,media_type,external_id)::text;

ALTER TABLE t_share_media_file DROP CONSTRAINT IF EXISTS share_media_file_owner_fk;
ALTER TABLE t_share_media DROP CONSTRAINT IF EXISTS share_media_id_share_unique;
UPDATE t_share_media_file f SET media_id=NULL,status='failed',error='媒体外部身份无效' FROM t_share_media m
 WHERE f.media_id=m.id AND (m.external_id='' OR m.media_type NOT IN ('movie','tv'));
DELETE FROM t_share_media WHERE external_id='' OR media_type NOT IN ('movie','tv');

WITH identities AS (
 SELECT id,min(id) OVER(PARTITION BY metadata_source,metadata_provider,media_type,external_id) canonical_id
 FROM t_share_media
)
UPDATE t_share_media_file f SET media_id=i.canonical_id FROM identities i
 WHERE f.media_id=i.id AND i.id<>i.canonical_id;
DELETE FROM t_share_media m WHERE EXISTS (
 SELECT 1 FROM t_share_media keep WHERE keep.id<m.id
 AND keep.metadata_source=m.metadata_source AND keep.metadata_provider=m.metadata_provider
 AND keep.media_type=m.media_type AND keep.external_id=m.external_id);

CREATE TABLE IF NOT EXISTS t_share_media_file_episode (
 file_id INTEGER NOT NULL REFERENCES t_share_media_file(id) ON DELETE CASCADE,
 season_number INTEGER NOT NULL CHECK(season_number>=0),
 episode_number INTEGER NOT NULL CHECK(episode_number>0),
 PRIMARY KEY(file_id,season_number,episode_number)
);
INSERT INTO t_share_media_file_episode(file_id,season_number,episode_number)
 SELECT f.id,(f.result->>'season_number')::integer,(f.result->>'episode_number')::integer
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id
 WHERE m.media_type='tv' AND f.result->>'season_number' ~ '^[0-9]{1,2}$'
 AND f.result->>'episode_number' ~ '^[1-9][0-9]{0,3}$'
 ON CONFLICT DO NOTHING;
-- 不访问外部元数据，直接按现有命名规则回填 S01E01E02 等多集文件。
INSERT INTO t_share_media_file_episode(file_id,season_number,episode_number)
 SELECT f.id,season_match.value[1]::integer,episode_match.value[1]::integer
 FROM t_share_media_file f
 JOIN t_share_media m ON m.id=f.media_id
 CROSS JOIN LATERAL regexp_match(f.file_name,'(?i)S([0-9]{1,2})E[0-9]{1,4}') AS season_match(value)
 CROSS JOIN LATERAL regexp_matches(f.file_name,'(?i)E([0-9]{1,4})','g') AS episode_match(value)
 WHERE m.media_type='tv' AND season_match.value[1] IS NOT NULL AND episode_match.value[1]::integer>0
 ON CONFLICT DO NOTHING;
UPDATE t_share_media_file f SET status='failed',error='缺少季集信息',media_id=NULL
 WHERE f.status='identified' AND EXISTS(SELECT 1 FROM t_share_media m WHERE m.id=f.media_id AND m.media_type='tv')
 AND NOT EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id);
DELETE FROM t_share_media m WHERE NOT EXISTS(SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id);

-- 唯一索引建立前优先保留已识别的最佳历史记录。
WITH ranked AS (
 SELECT id,row_number() OVER(PARTITION BY share_id,file_id ORDER BY (status='identified') DESC,id) ordinal
 FROM t_share_media_file WHERE file_id<>''
) DELETE FROM t_share_media_file f USING ranked r WHERE f.id=r.id AND r.ordinal>1;
DELETE FROM t_share_media m WHERE NOT EXISTS(SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id);
WITH ranked AS (
 SELECT id,row_number() OVER(PARTITION BY share_id,file_name ORDER BY (status='identified') DESC,id) ordinal
 FROM t_share_media_file WHERE file_id=''
) DELETE FROM t_share_media_file f USING ranked r WHERE f.id=r.id AND r.ordinal>1;
DELETE FROM t_share_media m WHERE NOT EXISTS(SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id);

ALTER TABLE t_share_media DROP COLUMN IF EXISTS share_id;
ALTER TABLE t_share_media DROP COLUMN IF EXISTS status;
ALTER TABLE t_share_media DROP COLUMN IF EXISTS error;
ALTER TABLE t_share_media ALTER COLUMN external_id SET NOT NULL;
ALTER TABLE t_share_media ALTER COLUMN media_type SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS share_media_identity_unique
 ON t_share_media(metadata_source,metadata_provider,media_type,external_id);
CREATE UNIQUE INDEX IF NOT EXISTS share_media_file_remote_unique
 ON t_share_media_file(share_id,file_id) WHERE file_id<>'';
CREATE UNIQUE INDEX IF NOT EXISTS share_media_file_path_unique
 ON t_share_media_file(share_id,file_name) WHERE file_id='';
CREATE INDEX IF NOT EXISTS share_media_file_available_status_idx
 ON t_share_media_file(share_id,available,status,id);
DO $constraint$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_share_media_file'::regclass AND conname='share_media_file_media_fk') THEN
  ALTER TABLE t_share_media_file ADD CONSTRAINT share_media_file_media_fk
   FOREIGN KEY(media_id) REFERENCES t_share_media(id) ON DELETE SET NULL;
 END IF;
END $constraint$;

CREATE VIEW v_share_media_detail AS
 SELECT f.id,f.id AS local_file_id,f.file_id,f.media_id,f.share_id,f.file_name,f.file_size,f.sha1,f.pick_code,
 f.metadata_source,f.status,f.result,f.error,f.version,f.available,f.last_seen_at,f.last_seen_scan_token,
 f.created_at,f.updated_at,m.work_key,m.tmdb_id,m.title,m.original_title,m.poster_path,
 m.media_type AS library_media_type,m.media_year,m.genre_ids,m.country_codes,m.rating,
 COALESCE((SELECT json_agg(json_build_object('season_number',e.season_number,'episode_number',e.episode_number)
 ORDER BY e.season_number,e.episode_number) FROM t_share_media_file_episode e WHERE e.file_id=f.id),'[]'::json) episodes
 FROM t_share_media_file f LEFT JOIN t_share_media m ON m.id=f.media_id;

DO $validate$
BEGIN
 IF EXISTS(SELECT 1 FROM t_share_media WHERE metadata_source='' OR external_id='' OR media_type NOT IN ('movie','tv')) THEN
  RAISE EXCEPTION 'v33校验失败：媒体主数据存在无效外部身份';
 END IF;
 IF EXISTS(SELECT 1 FROM t_share_media_file WHERE status='identified' AND media_id IS NULL) THEN
  RAISE EXCEPTION 'v33校验失败：已识别文件缺少媒体关联';
 END IF;
 IF EXISTS(
  SELECT 1 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id
  WHERE f.status='identified' AND m.media_type='tv'
  AND NOT EXISTS(SELECT 1 FROM t_share_media_file_episode e WHERE e.file_id=f.id)
 ) THEN
  RAISE EXCEPTION 'v33校验失败：已识别电视剧文件缺少季集映射';
 END IF;
 IF EXISTS(SELECT 1 FROM t_share_media m WHERE NOT EXISTS(SELECT 1 FROM t_share_media_file f WHERE f.media_id=m.id)) THEN
  RAISE EXCEPTION 'v33校验失败：存在无文件关联的媒体主数据';
 END IF;
END $validate$;
