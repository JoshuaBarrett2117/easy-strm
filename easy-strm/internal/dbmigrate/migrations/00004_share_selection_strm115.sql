-- 分享来源选择支持 115 目录来源（原 migrations/migrate_v51_share_selection_strm115.sql，原需 DBA 手工执行）。
-- 幂等：列 ADD COLUMN IF NOT EXISTS，约束按名称存在性判断；去掉了原脚本的 SET ROLE easy_strm_app 与属主/授权检查。
-- +goose Up
-- +goose StatementBegin
SET LOCAL lock_timeout = '60s';
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
-- +goose StatementEnd
