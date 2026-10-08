-- 更新日期：2026-10-08；维护者：Codex。DBA 专用合成种子；不读取/导入生产数据。
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL application_name='easy-strm-phase02-seed';
DO $$ BEGIN
 IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION '仅允许 easy_strm_test'; END IF;
 IF EXISTS(SELECT 1 FROM t_share_record WHERE id NOT BETWEEN 200000001 AND 200000040 OR name NOT LIKE 'phase02-synthetic-%')
 OR EXISTS(SELECT 1 FROM t_share_media WHERE id NOT BETWEEN 200000001 AND 200020000 OR title NOT LIKE 'phase02-synthetic-%')
 OR EXISTS(SELECT 1 FROM t_share_media_file WHERE id NOT BETWEEN 300000001 AND 300100000 OR file_id NOT LIKE 'phase02-synthetic-%')
 THEN RAISE EXCEPTION '检测到非本 harness 数据；请 DBA 使用独立空测试库，不允许自动清理'; END IF;
END $$;

INSERT INTO t_share_record(id,name,url,password,media_type,share_cancelled)
 SELECT 200000000+ordinal,'phase02-synthetic-'||ordinal,'https://115.com/s/phase02synthetic'||ordinal,'','auto',ordinal%7=0
 FROM generate_series(1,40) ordinal
 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,url=EXCLUDED.url,password=EXCLUDED.password,media_type=EXCLUDED.media_type,share_cancelled=EXCLUDED.share_cancelled;

INSERT INTO t_share_media(id,metadata_source,metadata_provider,external_id,media_type,work_key,tmdb_id,title,original_title,poster_path,media_year,genre_ids,country_codes,rating,result)
 SELECT 200000000+ordinal,'tmdb','',(200000000+ordinal)::text,kind,
 jsonb_build_array('tmdb','',kind,(200000000+ordinal)::text)::text,
 200000000+ordinal,'phase02-synthetic-'||ordinal,'phase02-synthetic-'||ordinal,'',2024,ARRAY[18],ARRAY['ZZ'],7.5,
 jsonb_build_object('success',true,'metadata_source','tmdb','tmdb_id',200000000+ordinal,'media_type',kind,'title','phase02-synthetic-'||ordinal,'year',2024,'genre_ids',jsonb_build_array(18),'countries',jsonb_build_array('ZZ'))
 FROM generate_series(1,20000) ordinal CROSS JOIN LATERAL (SELECT CASE WHEN ordinal%2=0 THEN 'tv' ELSE 'movie' END kind) types
 ON CONFLICT(id) DO UPDATE SET metadata_source=EXCLUDED.metadata_source,metadata_provider=EXCLUDED.metadata_provider,external_id=EXCLUDED.external_id,media_type=EXCLUDED.media_type,work_key=EXCLUDED.work_key,tmdb_id=EXCLUDED.tmdb_id,title=EXCLUDED.title,original_title=EXCLUDED.original_title,poster_path=EXCLUDED.poster_path,media_year=EXCLUDED.media_year,genre_ids=EXCLUDED.genre_ids,country_codes=EXCLUDED.country_codes,rating=EXCLUDED.rating,result=EXCLUDED.result;

INSERT INTO t_share_media_file(id,share_id,media_id,file_id,file_name,file_size,sha1,pick_code,metadata_source,status,result,available)
 SELECT 300000000+ordinal,200000001+(((ordinal-1)%40)+((ordinal-1)/20000)*3)%40,
 CASE WHEN ordinal%101=0 THEN NULL WHEN ordinal BETWEEN 20001 AND 20500 THEN 200000001
 WHEN ordinal BETWEEN 20501 AND 20550 THEN 200000002 ELSE 200000001+(ordinal-1)%20000 END,
 'phase02-synthetic-'||ordinal,'phase02-synthetic/Video-'||ordinal||'.mkv',1024::bigint*ordinal,'','','tmdb',
 CASE WHEN ordinal%101=0 THEN 'pending' WHEN ordinal%103=0 THEN 'failed' WHEN ordinal%107=0 THEN 'ignored' ELSE 'identified' END,'{}'::jsonb,ordinal%11<>0
 FROM generate_series(1,100000) ordinal
 ON CONFLICT(id) DO UPDATE SET share_id=EXCLUDED.share_id,media_id=EXCLUDED.media_id,file_id=EXCLUDED.file_id,file_name=EXCLUDED.file_name,file_size=EXCLUDED.file_size,sha1=EXCLUDED.sha1,pick_code=EXCLUDED.pick_code,metadata_source=EXCLUDED.metadata_source,status=EXCLUDED.status,result=EXCLUDED.result,available=EXCLUDED.available;

INSERT INTO t_share_media_file_episode(file_id,season_number,episode_number)
 SELECT f.id,1+((f.id-300000001)/20000)%2,1+(f.id-300000001)%10
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id
 WHERE m.media_type='tv' AND f.id BETWEEN 300000001 AND 300100000 ON CONFLICT DO NOTHING;
INSERT INTO t_share_media_file_episode(file_id,season_number,episode_number)
 SELECT f.id,1+((f.id-300000001)/20000)%2,21+(f.id-300000001)%10
 FROM t_share_media_file f JOIN t_share_media m ON m.id=f.media_id
 WHERE m.media_type='tv' AND f.id BETWEEN 300000001 AND 300100000 AND f.id%13=0 ON CONFLICT DO NOTHING;

INSERT INTO t_strm_export_state(owner_key,export_key,output_path,last_seen_run_id)
 SELECT 'share:default','phase02-owned-'||ordinal,'/phase02-synthetic/owned-'||ordinal||'.strm','synthetic-seed'
 FROM generate_series(1,100000) ordinal ON CONFLICT DO NOTHING;
INSERT INTO t_strm_export_history(owner_key,output_path,reason)
 SELECT 'share:default','/phase02-synthetic/history-'||ordinal||'.strm','synthetic-only'
 FROM generate_series(1,20000) ordinal ON CONFLICT DO NOTHING;
INSERT INTO t_strm_file(strm_config_id,file_name,file_path,local_strm_path)
 SELECT -1,'synthetic-'||ordinal||'.mkv','/phase02-synthetic/legacy-'||ordinal||'.strm','/phase02-synthetic/legacy-'||ordinal||'.strm'
 FROM generate_series(1,20000) ordinal ON CONFLICT DO NOTHING;
INSERT INTO t_share_strm(id,payload) VALUES('00000000-0000-0000-0000-000000000043','{"phase02_synthetic":true,"file_path":"synthetic.mkv"}') ON CONFLICT DO NOTHING;

INSERT INTO t_system_config(config_key,config_val) VALUES('share_strm_settings','{"output_path":"/phase02-synthetic","base_url":"https://phase02.invalid","cloud115_id":1,"transfer_path":"/phase02"}')
 ON CONFLICT(config_key) DO UPDATE SET config_val=EXCLUDED.config_val;

DO $$ BEGIN
 IF (SELECT count(*) FROM t_share_media)<>20000 OR (SELECT count(*) FROM t_share_media_file)<>100000
 THEN RAISE EXCEPTION '合成数据规模不符'; END IF;
 IF NOT EXISTS(SELECT 1 FROM t_share_media_file WHERE NOT available) OR NOT EXISTS(SELECT 1 FROM t_share_record WHERE share_cancelled)
 OR NOT EXISTS(SELECT 1 FROM t_share_media_file WHERE media_id IS NULL) THEN RAISE EXCEPTION '缺少合成失效场景'; END IF;
END $$;
COMMIT;
ANALYZE t_share_media;
ANALYZE t_share_media_file;
ANALYZE t_share_record;
ANALYZE t_share_media_file_episode;
ANALYZE t_strm_export_state;
ANALYZE t_strm_export_history;
