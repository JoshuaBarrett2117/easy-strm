-- 更新：2026-10-19；维护者：Codex。仅供 DBA 离线复审后手工执行，禁止启动时嵌入或自动应用。
-- 执行前备份并选择维护窗口；5 秒锁超时或 30 秒语句超时均应整笔回滚，不自动重试。
-- 不改属主、不改既有复合 FK、不增加索引。半迁移只补齐定义正确的列；漂移立即拒绝。
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';
SET ROLE easy_strm_app;
DO $preflight$
DECLARE
 selection_table regclass := to_regclass('t_share_media_selection');
 item_table regclass := to_regclass('t_share_export_candidate_item');
 column_row record;
 expected_default text;
BEGIN
 IF selection_table IS NULL OR item_table IS NULL THEN
  RAISE EXCEPTION 'v51 需要 v47 表；请 DBA 检查 schema/search_path，禁止自动创建';
 END IF;
 LOCK TABLE t_share_media_selection IN ACCESS EXCLUSIVE MODE;
 IF NOT EXISTS(SELECT 1 FROM pg_class WHERE oid=selection_table AND relkind='r' AND pg_has_role(current_user,relowner,'USAGE')) THEN
  RAISE EXCEPTION 'easy_strm_app 缺少表属主权限；请 DBA 人工处理，脚本不改属主';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=selection_table AND attname='selected_candidate_id' AND NOT attisdropped AND atttypid='bigint'::regtype) THEN
  RAISE EXCEPTION 'selected_candidate_id 缺失或类型漂移，拒绝 v51';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint constraint_row WHERE conrelid=selection_table AND confrelid=item_table
  AND conname='share_media_selection_member_fk' AND contype='f' AND confmatchtype='s' AND convalidated
  AND (SELECT array_agg(attribute.attname::text ORDER BY keys.position) FROM unnest(constraint_row.conkey) WITH ORDINALITY keys(number,position) JOIN pg_attribute attribute ON attribute.attrelid=selection_table AND attribute.attnum=keys.number)=ARRAY['selected_candidate_id','work_key','season_number','episode_number']
  AND (SELECT array_agg(attribute.attname::text ORDER BY keys.position) FROM unnest(constraint_row.confkey) WITH ORDINALITY keys(number,position) JOIN pg_attribute attribute ON attribute.attrelid=item_table AND attribute.attnum=keys.number)=ARRAY['candidate_id','work_key','season_number','episode_number']) THEN
  RAISE EXCEPTION 'v47 复合 FK 缺失、不匹配或非有效 MATCH SIMPLE；请 DBA 处理，禁止重建 FK';
 END IF;
 FOR column_row IN SELECT attribute.*,pg_get_expr(defaults.adbin,defaults.adrelid) default_expr
  FROM pg_attribute attribute LEFT JOIN pg_attrdef defaults ON defaults.adrelid=attribute.attrelid AND defaults.adnum=attribute.attnum
  WHERE attribute.attrelid=selection_table AND NOT attribute.attisdropped AND attribute.attname IN ('source_kind','external_ref')
 LOOP
  expected_default := CASE WHEN column_row.attname='source_kind' THEN '''share''::text' ELSE '''''::text' END;
  IF column_row.atttypid<>'text'::regtype OR NOT column_row.attnotnull OR column_row.default_expr IS DISTINCT FROM expected_default THEN
   RAISE EXCEPTION 'v51 列 % 定义漂移，拒绝自动修复',column_row.attname;
  END IF;
 END LOOP;
 IF (SELECT count(*) FROM pg_attribute WHERE attrelid=selection_table AND NOT attisdropped AND attname IN ('source_kind','external_ref'))=1 THEN
  RAISE NOTICE '检测到部分 v51 列；已校验既有列定义，将补齐缺失列';
 END IF;
END $preflight$;
ALTER TABLE t_share_media_selection
 ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'share',
 ADD COLUMN IF NOT EXISTS external_ref text NOT NULL DEFAULT '';
ALTER TABLE t_share_media_selection ALTER COLUMN selected_candidate_id DROP NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_share_media_selection'::regclass AND conname='share_selection_source_kind_check') THEN
  ALTER TABLE t_share_media_selection ADD CONSTRAINT share_selection_source_kind_check CHECK (source_kind IN ('share','strm115'));
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_share_media_selection'::regclass AND conname='share_selection_source_ref_check') THEN
  ALTER TABLE t_share_media_selection ADD CONSTRAINT share_selection_source_ref_check CHECK ((source_kind='share' AND selected_candidate_id IS NOT NULL) OR (source_kind='strm115' AND external_ref<>''));
 END IF;
END $$;
DO $verify$
DECLARE
 definition text;
BEGIN
 SELECT regexp_replace(pg_get_constraintdef(oid),'\s+','','g') INTO definition FROM pg_constraint
 WHERE conrelid='t_share_media_selection'::regclass AND conname='share_selection_source_kind_check' AND contype='c' AND convalidated;
 IF definition IS DISTINCT FROM 'CHECK((source_kind=ANY(ARRAY[''share''::text,''strm115''::text])))' THEN
  RAISE EXCEPTION 'source_kind CHECK 定义漂移或 deparse 不兼容，需 DBA 离线复审：%',definition;
 END IF;
 SELECT regexp_replace(pg_get_constraintdef(oid),'\s+','','g') INTO definition FROM pg_constraint
 WHERE conrelid='t_share_media_selection'::regclass AND conname='share_selection_source_ref_check' AND contype='c' AND convalidated;
 IF definition IS DISTINCT FROM 'CHECK((((source_kind=''share''::text)AND(selected_candidate_idISNOTNULL))OR((source_kind=''strm115''::text)AND(external_ref<>''''::text))))' THEN
  RAISE EXCEPTION '来源引用 CHECK 定义漂移或 deparse 不兼容，需 DBA 离线复审：%',definition;
 END IF;
 IF (SELECT count(*) FROM pg_attribute attribute JOIN pg_attrdef defaults ON defaults.adrelid=attribute.attrelid AND defaults.adnum=attribute.attnum
  WHERE attribute.attrelid='t_share_media_selection'::regclass AND NOT attribute.attisdropped AND attribute.atttypid='text'::regtype AND attribute.attnotnull
  AND ((attribute.attname='source_kind' AND pg_get_expr(defaults.adbin,defaults.adrelid)='''share''::text') OR (attribute.attname='external_ref' AND pg_get_expr(defaults.adbin,defaults.adrelid)='''''::text')))<>2
 OR NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='t_share_media_selection'::regclass AND attname='selected_candidate_id' AND NOT attisdropped AND NOT attnotnull AND atttypid='bigint'::regtype) THEN
  RAISE EXCEPTION 'v51 结构回读不一致，整笔回滚';
 END IF;
 IF NOT has_column_privilege(current_user,'t_share_media_selection','source_kind','SELECT')
 OR NOT has_column_privilege(current_user,'t_share_media_selection','source_kind','UPDATE')
 OR NOT has_column_privilege(current_user,'t_share_media_selection','external_ref','SELECT')
 OR NOT has_column_privilege(current_user,'t_share_media_selection','external_ref','UPDATE') THEN
  RAISE EXCEPTION 'easy_strm_app 缺少新增列权限，请 DBA 人工处理；脚本不授权';
 END IF;
END $verify$;
COMMIT;
RESET ROLE;
