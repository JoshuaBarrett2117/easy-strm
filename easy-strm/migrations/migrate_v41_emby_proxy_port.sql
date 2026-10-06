-- 更新日期：2026-10-06；维护者：Codex。0 表示关闭，非零端口按实例独占。
ALTER TABLE t_emby_server ADD COLUMN IF NOT EXISTS proxy_port INTEGER NOT NULL DEFAULT 0;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='t_emby_server'::regclass AND conname='ck_emby_server_proxy_port') THEN
        ALTER TABLE t_emby_server ADD CONSTRAINT ck_emby_server_proxy_port CHECK (proxy_port BETWEEN 0 AND 65535);
    END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS uk_emby_server_proxy_port ON t_emby_server(proxy_port) WHERE proxy_port <> 0;
