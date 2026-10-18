-- migrate_v50_behavior_sync_config.sql
-- 115 操作事件增量同步：按 STRM 配置维度启用，checkpoint 从全局账号游标改为 (strm_config_id, cloud115_id)。
-- 更新日期：2026-10-18；执行者：Codex
-- 说明：
--   1) t_strm_config 的 behavior_sync_enabled / behavior_sync_interval 两列同时由应用启动迁移幂等补齐（db.go），
--      此处重复添加仅作 DBA 独立应用时的兜底，均使用 ADD COLUMN IF NOT EXISTS，重复执行无副作用。
--   2) t_behavior_checkpoint 旧表主键为 cloud115_id（每账号一个全局游标）。本迁移保留旧游标进度，
--      为每个 STRM 配置生成独立 checkpoint（继承所属账号的旧游标），并重建为复合主键 (strm_config_id, cloud115_id)。
--   3) 本脚本需在 v49（t_behavior_checkpoint / t_strm_file_index 已建）之后执行，建议 DBA 在低峰期以单事务运行。
BEGIN;

-- 1) STRM 配置新增增量开关与周期（可选字段，默认关闭、周期 600 秒）
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS behavior_sync_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE t_strm_config ADD COLUMN IF NOT EXISTS behavior_sync_interval integer NOT NULL DEFAULT 600;

-- 2) checkpoint 迁移到配置维度
-- 2.1 新增配置维度列（先可空，避免与旧主键冲突）
ALTER TABLE t_behavior_checkpoint ADD COLUMN IF NOT EXISTS strm_config_id integer REFERENCES t_strm_config(id);

-- 2.2 解除旧主键，允许同账号多配置各自持有一行
ALTER TABLE t_behavior_checkpoint DROP CONSTRAINT IF EXISTS t_behavior_checkpoint_pkey;

-- 2.3 为每个 STRM 配置生成独立 checkpoint，继承该账号旧全局游标（仅对尚未生成的配置补充，保证可重入）
INSERT INTO t_behavior_checkpoint(strm_config_id, cloud115_id, last_event_id, last_run_at, update_time)
SELECT sc.id,
       sc.cloud115_id,
       COALESCE(legacy.last_event_id, 0),
       legacy.last_run_at,
       legacy.update_time
FROM t_strm_config sc
LEFT JOIN t_behavior_checkpoint legacy
       ON legacy.cloud115_id = sc.cloud115_id
      AND legacy.strm_config_id IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM t_behavior_checkpoint cp
    WHERE cp.strm_config_id = sc.id AND cp.cloud115_id = sc.cloud115_id
);

-- 2.4 删除旧的全局游标行（strm_config_id 为空）
DELETE FROM t_behavior_checkpoint WHERE strm_config_id IS NULL;

-- 2.5 收紧非空约束并重建复合主键
ALTER TABLE t_behavior_checkpoint ALTER COLUMN strm_config_id SET NOT NULL;
ALTER TABLE t_behavior_checkpoint ADD PRIMARY KEY (strm_config_id, cloud115_id);

COMMIT;
