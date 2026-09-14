ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS recognition_method VARCHAR(20) NOT NULL DEFAULT 'source';
ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS metadata_source VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS metadata_id VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS metadata_provider VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS ai_used BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS ai_scene VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE t_identify_cache ADD COLUMN IF NOT EXISTS failure_reason TEXT NOT NULL DEFAULT '';

UPDATE t_identify_cache SET recognition_method = 'manual' WHERE is_manual = TRUE AND recognition_method <> 'manual';
