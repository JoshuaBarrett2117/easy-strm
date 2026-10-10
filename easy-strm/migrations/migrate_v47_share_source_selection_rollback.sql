-- 更新日期：2026-10-10；维护者：Codex。只回滚 v47 对象，不触碰输出目录。
BEGIN;
SET LOCAL lock_timeout='30s';
SET LOCAL statement_timeout='5min';
DROP TABLE IF EXISTS t_share_media_selection;
DROP TABLE IF EXISTS t_share_export_candidate_item;
DROP TABLE IF EXISTS t_share_export_candidate;
DROP FUNCTION IF EXISTS share_export_candidate_guard();
DROP SEQUENCE IF EXISTS share_export_candidate_discovery_seq;
DROP SEQUENCE IF EXISTS share_export_candidate_seq;
COMMIT;
