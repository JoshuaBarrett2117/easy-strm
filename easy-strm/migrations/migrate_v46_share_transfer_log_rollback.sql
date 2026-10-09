-- !!! 不可逆日志删除 / IRREVERSIBLE LOG LOSS：全部日志与索引将随表删除 !!!
-- 更新日期：2026-10-09；维护者：Codex。
-- 来源：/opt/data/output/easy-strm-phase03/D1-DBA-review.md 与 D1-share-transfer-log.down.draft.sql。
-- 仅供本次变更新建的 t_share_transfer_log 使用；变更前已存在的表禁止采用本回滚。
-- 实际回滚必须另行取得独立 DBA 批准，明确目标库并确认日志无保留价值；应用修复批准不等于执行批准。
-- 执行前必须按顺序完成：
-- 1. 停用全部 HTTP、Telegram 及其它提交/重试入口，停止所有实例、多副本及独立写入进程。
-- 2. 排空或终止全部在途 executeTransfer goroutine 与其它相关工作，确认无 BatchInsert/UpdateStatus 并发写入。
-- 3. DBA 通过 pg_stat_activity 或等价只读观测，确认全部写入会话及在途任务已停止。
-- 4. 停写后备份完整表结构与数据，回读验证内容完整性，并完成恢复验证（backup AND restore validation）。
--    仅存在备份文件不够；备份和恢复验证均通过后才允许继续。
-- 5. 记录当前行数与回滚审批证据，保持全部写者停止，防止应用启动重新建表或继续写入。
-- 不使用 CASCADE；索引随表删除，无需单独 DROP INDEX。本文件从未执行 SQL。

DROP TABLE IF EXISTS t_share_transfer_log;

-- 审批后由 DBA 验证表已不存在，全部应用实例仍停写；无法满足任一前置条件时禁止回滚。
