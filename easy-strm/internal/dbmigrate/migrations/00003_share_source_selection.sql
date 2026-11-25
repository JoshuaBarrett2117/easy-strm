-- 分享来源选择（原 migrations/migrate_v47_share_source_selection.sql，原需 DBA 手工执行）。
-- 幂等：结构全部 IF NOT EXISTS；历史回填、入队与“需要重建基线”只在本次新建 t_share_media_selection 时执行，
-- 因此已手工安装过 v47 的库不会被再次回填，也不会被强制重建基线。
-- 与原脚本的差异：work_key 为空的媒体不参与回填（原脚本直接报错中止），重复 work_key 只告警不中止，
-- 避免启动被阻塞；DISTINCT ON 保证每个媒体项只选一个候选。
-- +goose Up
-- +goose StatementBegin
SET LOCAL lock_timeout = '60s';
SELECT set_config('easy_strm.share_selection_fresh', (to_regclass('t_share_media_selection') IS NULL)::text, true);
LOCK TABLE t_share_media_file, t_share_media_file_episode, t_share_media, t_share_record IN SHARE MODE;
CREATE SEQUENCE IF NOT EXISTS share_export_candidate_seq AS BIGINT MINVALUE 1 CACHE 1;
CREATE SEQUENCE IF NOT EXISTS share_export_candidate_discovery_seq AS BIGINT MINVALUE 1 CACHE 1;
CREATE TABLE IF NOT EXISTS t_share_export_candidate (
 candidate_id BIGINT PRIMARY KEY DEFAULT nextval('share_export_candidate_seq'),
 share_id INTEGER NOT NULL CHECK(share_id>0), remote_file_id TEXT NOT NULL DEFAULT '',
 source_file_id INTEGER, name_hash TEXT NOT NULL DEFAULT '',
 first_seen_seq BIGINT NOT NULL DEFAULT nextval('share_export_candidate_discovery_seq') CHECK(first_seen_seq>0),
 file_version INTEGER NOT NULL DEFAULT 0, share_version INTEGER NOT NULL DEFAULT 0,
 file_size BIGINT NOT NULL DEFAULT 0, available BOOLEAN NOT NULL DEFAULT TRUE,
 state TEXT NOT NULL DEFAULT 'active' CHECK(state IN ('active','stale','removed')),
 episode_sig TEXT NOT NULL DEFAULT '', last_seen_run_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(remote_file_id<>'' OR source_file_id IS NOT NULL), CHECK(remote_file_id<>'' OR name_hash<>'')
);
CREATE UNIQUE INDEX IF NOT EXISTS share_export_candidate_remote_uq ON t_share_export_candidate(share_id,remote_file_id) WHERE remote_file_id<>'';
CREATE UNIQUE INDEX IF NOT EXISTS share_export_candidate_named_uq ON t_share_export_candidate(share_id,name_hash) WHERE remote_file_id='';
CREATE UNIQUE INDEX IF NOT EXISTS share_export_candidate_source_uq ON t_share_export_candidate(source_file_id) WHERE source_file_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS share_export_candidate_share_order ON t_share_export_candidate(share_id,first_seen_seq,candidate_id);
CREATE TABLE IF NOT EXISTS t_share_export_candidate_item (
 candidate_id BIGINT NOT NULL REFERENCES t_share_export_candidate(candidate_id) ON DELETE CASCADE,
 work_key TEXT NOT NULL CHECK(work_key<>''), season_number INTEGER NOT NULL CHECK(season_number>=0),
 episode_number INTEGER NOT NULL CHECK(episode_number>=0), revoked BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(candidate_id,work_key,season_number,episode_number)
);
CREATE INDEX IF NOT EXISTS share_export_candidate_item_lookup ON t_share_export_candidate_item(work_key,season_number,episode_number,candidate_id);
CREATE TABLE IF NOT EXISTS t_share_media_selection (
 work_key TEXT NOT NULL CHECK(work_key<>''), season_number INTEGER NOT NULL CHECK(season_number>=0),
 episode_number INTEGER NOT NULL CHECK(episode_number>=0),
 media_item_key TEXT GENERATED ALWAYS AS (work_key||':'||season_number||':'||episode_number) STORED PRIMARY KEY,
 selected_candidate_id BIGINT NOT NULL, selection_mode TEXT NOT NULL DEFAULT 'auto' CHECK(selection_mode IN ('auto','manual')),
 selection_revision BIGINT NOT NULL DEFAULT 1 CHECK(selection_revision>=1),
 exported_revision BIGINT NOT NULL DEFAULT 0 CHECK(exported_revision>=0 AND exported_revision<=selection_revision),
 stable_relative_path TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CONSTRAINT share_media_selection_member_fk FOREIGN KEY(selected_candidate_id,work_key,season_number,episode_number)
 REFERENCES t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS share_media_selection_pending ON t_share_media_selection(updated_at,work_key,season_number,episode_number) WHERE selection_revision>exported_revision;
CREATE INDEX IF NOT EXISTS share_media_selection_work ON t_share_media_selection(work_key,season_number,episode_number);
CREATE OR REPLACE FUNCTION share_export_candidate_guard() RETURNS trigger LANGUAGE plpgsql AS $guard$
BEGIN
 IF NEW.candidate_id IS DISTINCT FROM OLD.candidate_id OR NEW.share_id IS DISTINCT FROM OLD.share_id
 OR NEW.remote_file_id IS DISTINCT FROM OLD.remote_file_id OR NEW.name_hash IS DISTINCT FROM OLD.name_hash
 OR NEW.first_seen_seq IS DISTINCT FROM OLD.first_seen_seq THEN
  RAISE EXCEPTION 'SHARE_CANDIDATE_IMMUTABLE';
 END IF;
 RETURN NEW;
END $guard$;
DROP TRIGGER IF EXISTS candidate_identity_immutable ON t_share_export_candidate;
CREATE TRIGGER candidate_identity_immutable BEFORE UPDATE ON t_share_export_candidate FOR EACH ROW EXECUTE FUNCTION share_export_candidate_guard();
DO $backfill$
BEGIN
 IF current_setting('easy_strm.share_selection_fresh', true) IS DISTINCT FROM 'true' THEN
  RETURN;
 END IF;
 IF EXISTS(SELECT 1 FROM t_share_media WHERE COALESCE(work_key,'')='') THEN
  RAISE WARNING 'share selection backfill: 存在 work_key 为空的媒体，已跳过这些媒体的回填';
 END IF;
 IF EXISTS(SELECT work_key FROM t_share_media WHERE COALESCE(work_key,'')<>'' GROUP BY work_key HAVING count(*)>1) THEN
  RAISE WARNING 'share selection backfill: 存在重复 work_key，每个媒体项按最早发现的候选自动选择';
 END IF;
 PERFORM setval('share_export_candidate_discovery_seq', GREATEST(
  COALESCE((SELECT max(id) FROM t_share_media_file),0)+1,
  COALESCE((SELECT max(first_seen_seq) FROM t_share_export_candidate),0)+1,
  (SELECT last_value+CASE WHEN is_called THEN 1 ELSE 0 END FROM share_export_candidate_discovery_seq)),false);
 INSERT INTO t_share_export_candidate(share_id,remote_file_id,source_file_id,name_hash,first_seen_seq,file_version,share_version,file_size,available,state,last_seen_run_id)
 SELECT f.share_id,COALESCE(f.file_id,''),f.id,CASE WHEN COALESCE(f.file_id,'')='' THEN md5(f.file_name) ELSE '' END,
  f.id,f.version,s.version,COALESCE(f.file_size,0),f.available AND NOT s.share_cancelled,
  CASE WHEN f.available AND NOT s.share_cancelled AND f.status='identified' THEN 'active' ELSE 'stale' END,COALESCE(f.last_seen_scan_token,'')
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id JOIN t_share_record s ON s.id=f.share_id
 WHERE COALESCE(m.work_key,'')<>'' ORDER BY f.id ON CONFLICT DO NOTHING;
 INSERT INTO t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number)
 SELECT c.candidate_id,m.work_key,e.season_number,e.episode_number FROM t_share_export_candidate c
 JOIN t_share_media_file f ON f.id=c.source_file_id JOIN t_share_media m ON m.id=f.media_id
 JOIN t_share_media_file_episode e ON e.file_id=f.id WHERE m.media_type='tv' ON CONFLICT DO NOTHING;
 INSERT INTO t_share_export_candidate_item(candidate_id,work_key,season_number,episode_number)
 SELECT c.candidate_id,m.work_key,0,0 FROM t_share_export_candidate c JOIN t_share_media_file f ON f.id=c.source_file_id
 JOIN t_share_media m ON m.id=f.media_id WHERE m.media_type='movie' ON CONFLICT DO NOTHING;
 PERFORM setval('share_export_candidate_seq', GREATEST(COALESCE((SELECT max(candidate_id) FROM t_share_export_candidate),0)+1,
  (SELECT last_value+CASE WHEN is_called THEN 1 ELSE 0 END FROM share_export_candidate_seq)),false);
 INSERT INTO t_share_media_selection(work_key,season_number,episode_number,selected_candidate_id)
 SELECT DISTINCT ON(i.work_key,i.season_number,i.episode_number) i.work_key,i.season_number,i.episode_number,c.candidate_id
 FROM t_share_export_candidate_item i JOIN t_share_export_candidate c USING(candidate_id)
 WHERE c.available AND c.state='active' AND NOT i.revoked
 ORDER BY i.work_key,i.season_number,i.episode_number,c.first_seen_seq,c.candidate_id ON CONFLICT DO NOTHING;
 PERFORM share_export_enqueue(ARRAY(SELECT DISTINCT work_key FROM t_share_media_selection),'selection-v47');
 PERFORM share_export_require_baseline();
END $backfill$;
-- +goose StatementEnd
