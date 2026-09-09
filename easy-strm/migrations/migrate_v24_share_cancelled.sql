-- 更新日期：2026-09-08；执行者：Codex。保存网盘分享取消状态，历史记录默认未确认取消。
ALTER TABLE t_share_record ADD COLUMN IF NOT EXISTS share_cancelled BOOLEAN NOT NULL DEFAULT FALSE;
