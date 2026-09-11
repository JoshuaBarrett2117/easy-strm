-- 更新日期：2026-09-10；执行者：Codex
-- 分享导出使用-1；普通配置仍保留外键及级联删除，无须伪造115账号或定时任务配置。
ALTER TABLE t_strm_file ADD COLUMN IF NOT EXISTS strm_config_ref_id INTEGER
    GENERATED ALWAYS AS (NULLIF(strm_config_id, -1)) STORED;

DO $migration$
DECLARE
    constraint_name text;
BEGIN
    FOR constraint_name IN
        SELECT c.conname FROM pg_constraint c
        JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=ANY(c.conkey)
        WHERE c.conrelid='t_strm_file'::regclass AND c.contype='f'
          AND c.confrelid='t_strm_config'::regclass AND a.attname='strm_config_id'
    LOOP
        EXECUTE format('ALTER TABLE t_strm_file DROP CONSTRAINT %I', constraint_name);
    END LOOP;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_strm_file'::regclass
        AND conname='t_strm_file_config_ref_fkey') THEN
        ALTER TABLE t_strm_file ADD CONSTRAINT t_strm_file_config_ref_fkey
            FOREIGN KEY(strm_config_ref_id) REFERENCES t_strm_config(id) ON DELETE CASCADE;
    END IF;
END $migration$;
