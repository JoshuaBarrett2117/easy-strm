-- 更新日期：2026-09-06；执行者：Codex
-- 分享主表与媒体子表；整个迁移由调用方事务执行。
CREATE TABLE IF NOT EXISTS t_share_record (
 id SERIAL PRIMARY KEY, name TEXT NOT NULL DEFAULT '', url TEXT NOT NULL,
 password TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1,
 created_at TIMESTAMP NOT NULL DEFAULT now(), updated_at TIMESTAMP NOT NULL DEFAULT now()
);
ALTER TABLE t_share_record ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_record ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT '';
ALTER TABLE t_share_record ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS t_share_media (
 id SERIAL PRIMARY KEY,
 share_id INTEGER NOT NULL REFERENCES t_share_record(id) ON DELETE CASCADE,
 file_name TEXT NOT NULL,
 metadata_source VARCHAR(20) NOT NULL DEFAULT 'auto',
 status VARCHAR(20) NOT NULL DEFAULT 'pending',
 result JSONB, error TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1,
 created_at TIMESTAMP NOT NULL DEFAULT now(), updated_at TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_share_media_share_id ON t_share_media(share_id);
CREATE INDEX IF NOT EXISTS idx_share_media_status ON t_share_media(status);
DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='t_share_record' AND column_name='file_name') THEN
  UPDATE t_share_record SET name=file_name WHERE name='';
  INSERT INTO t_share_media(share_id,file_name,metadata_source,status,result,error)
   SELECT id,file_name,metadata_source,status,result,error FROM t_share_record;
  ALTER TABLE t_share_record DROP COLUMN file_name, DROP COLUMN metadata_source,
   DROP COLUMN status, DROP COLUMN result, DROP COLUMN error;
 END IF;
END $$;
