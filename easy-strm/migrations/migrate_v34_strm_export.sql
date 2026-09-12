-- 2026-09-12 Codex：导出归属、历史路径和可恢复清理阶段。
CREATE TABLE IF NOT EXISTS t_strm_export_state (
 owner_key TEXT NOT NULL, export_key TEXT NOT NULL, output_path TEXT NOT NULL,
 content_fingerprint TEXT NOT NULL DEFAULT '', mapping_fingerprint TEXT NOT NULL DEFAULT '',
 share_strm_id TEXT, state TEXT NOT NULL DEFAULT 'active', last_seen_run_id TEXT NOT NULL DEFAULT '',
 exported_at TIMESTAMPTZ, PRIMARY KEY(owner_key,export_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS strm_export_active_path ON t_strm_export_state(output_path) WHERE state='active';
CREATE TABLE IF NOT EXISTS t_strm_export_history (
 owner_key TEXT NOT NULL, output_path TEXT NOT NULL, reason TEXT NOT NULL,
 PRIMARY KEY(owner_key,output_path)
);
CREATE TABLE IF NOT EXISTS t_strm_clear_run (task_id TEXT PRIMARY KEY, output_path TEXT NOT NULL, completed BOOLEAN NOT NULL DEFAULT FALSE);
