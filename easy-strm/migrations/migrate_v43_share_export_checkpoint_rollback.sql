-- 更新日期：2026-10-08；维护者：Codex。DBA 停止新消费者后手动执行；不删除任何磁盘文件。
DO $remove$
DECLARE relation TEXT; operation TEXT;
BEGIN
 FOREACH relation IN ARRAY ARRAY['t_share_media_file','t_share_media','t_share_record','t_share_media_file_episode','t_system_config','t_media_category','t_strm_export_state','t_share_export_source_state','t_share_export_dirty_work'] LOOP
  IF to_regclass(relation) IS NULL THEN CONTINUE; END IF;
  FOREACH operation IN ARRAY ARRAY['insert','update','delete','truncate'] LOOP
   EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I','share_export_'||operation,relation);
  END LOOP;
 END LOOP;
END $remove$;
DROP FUNCTION IF EXISTS share_export_produce();
DROP FUNCTION IF EXISTS share_export_checkpoint_invalidated();
DROP FUNCTION IF EXISTS share_export_enqueue(TEXT[],TEXT);
DROP FUNCTION IF EXISTS share_export_require_baseline();
DROP TABLE IF EXISTS t_share_export_source_state;
DROP TABLE IF EXISTS t_share_export_dirty_work;
DROP TABLE IF EXISTS t_share_export_consumer;
-- 保留两个共享路径索引，避免删除迁移前已由 DBA 创建的对象。
