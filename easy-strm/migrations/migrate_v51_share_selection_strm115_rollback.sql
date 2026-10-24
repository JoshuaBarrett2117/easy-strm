-- 更新：2026-10-19；维护者：Codex。仅供 DBA 手工执行，必须先备份并确认维护窗口。
-- 幂等回滚；任何目录选择或空候选均拒绝，必须先显式重新选择分享来源，绝不删除或自动候补。
-- 先锁表再检查数据，避免检查与移除列之间出现新的目录选择；超时整笔回滚。
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';
SET ROLE easy_strm_app;
DO $rollback$
DECLARE
 selection_table regclass := to_regclass('t_share_media_selection');
 source_column boolean;
 external_column boolean;
 unsafe_selection boolean;
BEGIN
 IF selection_table IS NULL THEN
  RAISE EXCEPTION 'v51 回滚无法解析 v47 选择表；请 DBA 检查 schema/search_path';
 END IF;
 LOCK TABLE t_share_media_selection IN ACCESS EXCLUSIVE MODE;
 IF NOT EXISTS(SELECT 1 FROM pg_class WHERE oid=selection_table AND pg_has_role(current_user,relowner,'USAGE')) THEN
  RAISE EXCEPTION 'easy_strm_app 缺少表属主权限；不自动改属主';
 END IF;
 SELECT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=selection_table AND attname='source_kind' AND NOT attisdropped),
        EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=selection_table AND attname='external_ref' AND NOT attisdropped)
 INTO source_column,external_column;
 IF source_column THEN
  EXECUTE 'SELECT EXISTS(SELECT 1 FROM t_share_media_selection WHERE source_kind IS DISTINCT FROM ''share'')' INTO unsafe_selection;
  IF unsafe_selection THEN
   RAISE EXCEPTION '请先显式重新选择所有115目录来源；禁止删除选择或自动回退';
  END IF;
 ELSIF external_column THEN
  RAISE EXCEPTION 'v51 列不完整，无法证明没有目录选择；请 DBA 人工核对';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=selection_table AND attname='selected_candidate_id' AND NOT attisdropped AND atttypid='bigint'::regtype) THEN
  RAISE EXCEPTION 'selected_candidate_id 缺失或类型漂移，拒绝回滚';
 END IF;
 EXECUTE 'SELECT EXISTS(SELECT 1 FROM t_share_media_selection WHERE selected_candidate_id IS NULL)' INTO unsafe_selection;
 IF unsafe_selection THEN
  RAISE EXCEPTION '存在空候选，请显式重新选择有效分享来源后重试；禁止删除或候补';
 END IF;
 EXECUTE 'ALTER TABLE t_share_media_selection ALTER COLUMN selected_candidate_id SET NOT NULL';
 EXECUTE 'ALTER TABLE t_share_media_selection DROP CONSTRAINT IF EXISTS share_selection_source_ref_check';
 EXECUTE 'ALTER TABLE t_share_media_selection DROP CONSTRAINT IF EXISTS share_selection_source_kind_check';
 EXECUTE 'ALTER TABLE t_share_media_selection DROP COLUMN IF EXISTS external_ref';
 EXECUTE 'ALTER TABLE t_share_media_selection DROP COLUMN IF EXISTS source_kind';
 IF EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=selection_table AND NOT attisdropped AND attname IN ('source_kind','external_ref')) THEN
  RAISE EXCEPTION 'v51 回滚结构回读失败';
 END IF;
END $rollback$;
COMMIT;
RESET ROLE;
