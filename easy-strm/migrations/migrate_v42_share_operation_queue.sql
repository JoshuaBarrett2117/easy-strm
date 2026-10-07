-- 更新日期：2026-10-07；维护者：Codex。分享清理操作持久化，不依赖分享行的外键以便删除后恢复结果。
CREATE TABLE IF NOT EXISTS t_share_operation_queue (
    sequence BIGSERIAL PRIMARY KEY,
    task_id TEXT NOT NULL UNIQUE,
    operation TEXT NOT NULL CHECK (operation IN ('share_delete','share_clear','share_media_delete')),
    request_key TEXT NOT NULL,
    share_ids INTEGER[] NOT NULL,
    file_id INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','cancelled')),
    phase TEXT NOT NULL DEFAULT '等待目标资源',
    result JSONB NOT NULL DEFAULT '{}',
    error TEXT NOT NULL DEFAULT '',
    published BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_share_operation_active ON t_share_operation_queue(request_key) WHERE status IN ('pending','running');
CREATE INDEX IF NOT EXISTS idx_share_operation_recovery ON t_share_operation_queue(status,sequence);
