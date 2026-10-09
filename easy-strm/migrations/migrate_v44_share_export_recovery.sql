-- v44 分享库导出恢复（A2）迁移：正向。仅由 DBA 显式执行，应用启动不得调用。
-- 更新日期：2026-10-09；维护者：Codex（草案），DBA 审核落地为仓库迁移 v44。
-- 来源：a2-recovery-migration-proposal-v1.sql；已在隔离测试库 easy_strm_test 验证通过
-- （回滚/幂等/约束与状态机 39 断言/并发 4 场景/EXPLAIN/二期回归），未在生产库执行。
-- 本 DDL 只含结构：writer 协作、统一锁序、fencing 触发器、捕获完整性、终态 CAS 等
-- 语义守卫由应用侧实现；仅执行本文件不修复 A2，也不得据此设置 catalog_ready=true。
-- 当前应用不使用以下表；仅执行 DDL 不会修复 A2，也不能启用任何任务。
-- 前置：v33/v34/v42/v43 已完成（守卫内校验）。
-- 默认安全拒绝；DBA 审核后须在本事务的守卫之前显式设置：
-- SET LOCAL easy_strm.a2_ddl_review = 'A2-V1-ISOLATED-REVIEWED';
-- 不使用 IF NOT EXISTS 掩盖结构漂移；再次执行前必须逐项比对目录定义。
-- 没有 destructive down、旧数据 completed/ready 回填、trigger 安装或协议降级。

BEGIN;
DO $review_guard$
BEGIN
 IF current_setting('easy_strm.a2_ddl_review', true)
    IS DISTINCT FROM 'A2-V1-ISOLATED-REVIEWED' THEN
  RAISE EXCEPTION 'UNAPPROVED：待 DBA 审核后由 DBA 在隔离测试库执行';
 END IF;
 IF to_regclass('t_share_export_consumer') IS NULL
    OR to_regclass('t_share_export_dirty_work') IS NULL
    OR to_regclass('t_share_export_source_state') IS NULL
    OR to_regclass('t_strm_export_state') IS NULL
    OR to_regclass('t_strm_export_history') IS NULL THEN
  RAISE EXCEPTION '缺少经核验的 v33/v34/v42/v43 前置结构';
 END IF;
END $review_guard$;

-- 全局不循环序列；DBA 不得 reset/setval 回退，不以 sequence 值作为提交水位。
CREATE SEQUENCE share_export_recovery_epoch_seq AS BIGINT
 START WITH 1 INCREMENT BY 1 MINVALUE 1 NO MAXVALUE NO CYCLE CACHE 1;
CREATE SEQUENCE share_export_recovery_generation_seq AS BIGINT
 START WITH 1 INCREMENT BY 1 MINVALUE 1 NO MAXVALUE NO CYCLE CACHE 1;

-- 所有 producer、配置、output/path 写者与终态事务共享行锁；DDL 本身不强制其使用。
CREATE TABLE t_share_export_recovery_fence (
 owner_key TEXT PRIMARY KEY CHECK (owner_key = 'share:default'),
 ledger_schema_version INTEGER NOT NULL DEFAULT 1 CHECK (ledger_schema_version = 1),
 catalog_ready BOOLEAN NOT NULL DEFAULT FALSE,
 producer_epoch BIGINT NOT NULL DEFAULT 0 CHECK (producer_epoch >= 0),
 bootstrap_id UUID,
 active_reconciliation_id UUID,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 初次旧数据采集只允许在所有相关写者停写后进行；停写跨崩溃仍须保持。
CREATE TABLE t_share_export_recovery_bootstrap (
 bootstrap_id UUID PRIMARY KEY,
 owner_key TEXT NOT NULL REFERENCES t_share_export_recovery_fence(owner_key),
 capture_epoch BIGINT NOT NULL CHECK (capture_epoch > 0),
 phase TEXT NOT NULL DEFAULT 'capturing'
  CHECK (phase IN ('capturing','validating','complete','invalidated')),
 capture_kind TEXT COLLATE "C" NOT NULL DEFAULT '',
 capture_key TEXT COLLATE "C" NOT NULL DEFAULT '',
 captured_count BIGINT NOT NULL DEFAULT 0 CHECK (captured_count >= 0),
 coverage_manifest JSONB NOT NULL DEFAULT '{}'::jsonb
  CHECK (jsonb_typeof(coverage_manifest) = 'object'),
 coverage_hash TEXT,
 completed_at TIMESTAMPTZ,
 invalidation_reason TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (owner_key, bootstrap_id),
 CHECK ((phase = 'complete') = (completed_at IS NOT NULL)),
 CHECK (phase <> 'complete' OR (coverage_hash IS NOT NULL AND coverage_hash <> ''))
);
CREATE UNIQUE INDEX share_export_recovery_bootstrap_one_open
 ON t_share_export_recovery_bootstrap(owner_key)
 WHERE phase IN ('capturing','validating');

-- 不删除墓碑；token 是所有写者协同维护的非 ABA 令牌，不是 last_seen_run_id。
CREATE TABLE t_share_export_recovery_token (
 resource_kind TEXT COLLATE "C" NOT NULL
  CHECK (resource_kind IN ('work','source','dirty','output','path','config')),
 resource_key TEXT COLLATE "C" NOT NULL CHECK (resource_key <> ''),
 version_token BIGINT NOT NULL CHECK (version_token > 0),
 present BOOLEAN NOT NULL DEFAULT FALSE,
 owner_key TEXT,
 export_key TEXT,
 normalized_path TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (resource_kind, resource_key),
 CHECK (resource_kind <> 'output' OR
  (owner_key IS NOT NULL AND owner_key <> '' AND export_key IS NOT NULL AND export_key <> '')),
 CHECK (resource_kind <> 'path' OR (normalized_path IS NOT NULL AND normalized_path <> ''))
);
CREATE INDEX share_export_recovery_token_output
 ON t_share_export_recovery_token(owner_key,export_key) WHERE resource_kind = 'output';
CREATE INDEX share_export_recovery_token_path
 ON t_share_export_recovery_token(normalized_path) WHERE resource_kind = 'path';

-- 历史目录：payload/version/from 不可变；仅允许将 open valid_to 一次关闭。
-- 半开区间 [valid_from_epoch, valid_to_epoch)，删除也写 present=false 墓碑。
CREATE TABLE t_share_export_recovery_catalog (
 owner_key TEXT NOT NULL REFERENCES t_share_export_recovery_fence(owner_key),
 member_kind TEXT COLLATE "C" NOT NULL
  CHECK (member_kind IN ('work','source','dirty','output','history_path','legacy_path','config')),
 member_key TEXT COLLATE "C" NOT NULL CHECK (member_key <> ''),
 valid_from_epoch BIGINT NOT NULL CHECK (valid_from_epoch > 0),
 valid_to_epoch BIGINT,
 present BOOLEAN NOT NULL,
 work_key TEXT,
 source_file_id INTEGER,
 export_key TEXT,
 normalized_path TEXT,
 resource_token BIGINT NOT NULL CHECK (resource_token > 0),
 payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
 payload_hash TEXT NOT NULL CHECK (payload_hash <> ''),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (owner_key,member_kind,member_key,valid_from_epoch),
 CHECK (valid_to_epoch IS NULL OR valid_to_epoch > valid_from_epoch),
 CHECK (member_kind <> 'work' OR (work_key IS NOT NULL AND work_key <> '')),
 CHECK (member_kind <> 'source' OR source_file_id IS NOT NULL),
 CHECK (member_kind <> 'output' OR (export_key IS NOT NULL AND export_key <> '')),
 CHECK (member_kind NOT IN ('history_path','legacy_path') OR
  (normalized_path IS NOT NULL AND normalized_path <> ''))
);
CREATE UNIQUE INDEX share_export_recovery_catalog_one_current
 ON t_share_export_recovery_catalog(owner_key,member_kind,member_key)
 WHERE valid_to_epoch IS NULL;
CREATE INDEX share_export_recovery_catalog_cut_scan
 ON t_share_export_recovery_catalog(owner_key,member_kind,member_key,valid_from_epoch)
 INCLUDE (valid_to_epoch,present);
CREATE INDEX share_export_recovery_catalog_work
 ON t_share_export_recovery_catalog(owner_key,work_key,member_kind,member_key);

CREATE TABLE t_share_export_reconciliation (
 reconciliation_id UUID PRIMARY KEY,
 generation BIGINT NOT NULL DEFAULT nextval('share_export_recovery_generation_seq') UNIQUE,
 owner_key TEXT NOT NULL REFERENCES t_share_export_recovery_fence(owner_key),
 normalized_root TEXT NOT NULL CHECK (normalized_root <> ''),
 config_revision BIGINT NOT NULL CHECK (config_revision > 0),
 config_fingerprint TEXT NOT NULL CHECK (config_fingerprint <> ''),
 protocol_version INTEGER NOT NULL CHECK (protocol_version > 0),
 bootstrap_id UUID NOT NULL,
 capture_epoch BIGINT NOT NULL CHECK (capture_epoch > 0),
 phase TEXT NOT NULL DEFAULT 'constructing'
  CHECK (phase IN ('constructing','consuming','sweeping','finalizing','complete','invalidated')),
 snapshot_complete BOOLEAN NOT NULL DEFAULT FALSE,
 sweep_complete BOOLEAN NOT NULL DEFAULT FALSE,
 scan_kind TEXT COLLATE "C" NOT NULL DEFAULT '',
 scan_key TEXT COLLATE "C" NOT NULL DEFAULT '',
 sweep_kind TEXT COLLATE "C" NOT NULL DEFAULT '',
 sweep_key TEXT COLLATE "C" NOT NULL DEFAULT '',
 member_count BIGINT NOT NULL DEFAULT 0 CHECK (member_count >= 0),
 snapshot_hash TEXT,
 completed_at TIMESTAMPTZ,
 invalidation_reason TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (owner_key,reconciliation_id),
 FOREIGN KEY (owner_key,bootstrap_id)
  REFERENCES t_share_export_recovery_bootstrap(owner_key,bootstrap_id),
 CHECK (generation > 0),
 CHECK (NOT sweep_complete OR snapshot_complete),
 CHECK (phase IN ('constructing','invalidated') OR snapshot_complete),
 CHECK (phase NOT IN ('finalizing','complete') OR sweep_complete),
 CHECK ((phase = 'complete') = (completed_at IS NOT NULL)),
 CHECK (NOT snapshot_complete OR (snapshot_hash IS NOT NULL AND snapshot_hash <> ''))
);
CREATE UNIQUE INDEX share_export_reconciliation_one_open
 ON t_share_export_reconciliation(owner_key)
 WHERE phase IN ('constructing','consuming','sweeping','finalizing');
CREATE INDEX share_export_reconciliation_binding
 ON t_share_export_reconciliation(owner_key,config_revision,config_fingerprint,protocol_version,generation);
ALTER TABLE t_share_export_recovery_fence ADD CONSTRAINT share_export_recovery_fence_bootstrap_fk
 FOREIGN KEY (owner_key,bootstrap_id)
 REFERENCES t_share_export_recovery_bootstrap(owner_key,bootstrap_id);
ALTER TABLE t_share_export_recovery_fence ADD CONSTRAINT share_export_recovery_fence_active_fk
 FOREIGN KEY (owner_key,active_reconciliation_id)
 REFERENCES t_share_export_reconciliation(owner_key,reconciliation_id);

-- 构造期只增不改；sealed 后完全不可变，ack 放独立表。
CREATE TABLE t_share_export_reconciliation_member (
 reconciliation_id UUID NOT NULL REFERENCES t_share_export_reconciliation(reconciliation_id),
 member_kind TEXT COLLATE "C" NOT NULL
  CHECK (member_kind IN ('work','source','dirty','output','history_path','legacy_path','config')),
 member_key TEXT COLLATE "C" NOT NULL CHECK (member_key <> ''),
 work_key TEXT,
 source_file_id INTEGER,
 export_key TEXT,
 output_owner TEXT,
 normalized_path TEXT,
 source_token BIGINT,
 dirty_revision BIGINT,
 start_output_token BIGINT,
 start_path_token BIGINT,
 start_last_seen_run_id TEXT,
 start_output_state TEXT,
 start_content_fingerprint TEXT,
 start_mapping_fingerprint TEXT,
 payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
 payload_hash TEXT NOT NULL CHECK (payload_hash <> ''),
 PRIMARY KEY (reconciliation_id,member_kind,member_key),
 CHECK (source_token IS NULL OR source_token > 0),
 CHECK (dirty_revision IS NULL OR dirty_revision >= 1),
 CHECK (start_output_token IS NULL OR start_output_token > 0),
 CHECK (start_path_token IS NULL OR start_path_token > 0),
 CHECK (member_kind <> 'work' OR (work_key IS NOT NULL AND work_key <> '')),
 CHECK (member_kind <> 'source' OR (source_file_id IS NOT NULL AND source_token IS NOT NULL)),
 CHECK (member_kind <> 'output' OR
  (output_owner IS NOT NULL AND output_owner = 'share:default'
   AND export_key IS NOT NULL AND export_key <> '' AND normalized_path IS NOT NULL AND normalized_path <> ''
   AND start_output_token IS NOT NULL AND start_path_token IS NOT NULL)),
 CHECK (member_kind NOT IN ('history_path','legacy_path') OR
  (normalized_path IS NOT NULL AND normalized_path <> '' AND start_path_token IS NOT NULL))
);
CREATE INDEX share_export_reconciliation_member_work
 ON t_share_export_reconciliation_member(reconciliation_id,work_key,member_kind,member_key);

-- 成功 ack 不可改；deferred 是明确转交持续 dirty，不等价于来源已经成功导出。
CREATE TABLE t_share_export_reconciliation_ack (
 reconciliation_id UUID NOT NULL,
 member_kind TEXT COLLATE "C" NOT NULL,
 member_key TEXT COLLATE "C" NOT NULL,
 ack_phase TEXT NOT NULL CHECK (ack_phase IN ('consume','sweep')),
 outcome TEXT NOT NULL CHECK (outcome IN ('verified','exported','stale_marked','protected','deferred')),
 acknowledged_revision BIGINT,
 result_token BIGINT,
 result_hash TEXT NOT NULL CHECK (result_hash <> ''),
 reason TEXT NOT NULL DEFAULT '',
 completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (reconciliation_id,member_kind,member_key,ack_phase),
 FOREIGN KEY (reconciliation_id,member_kind,member_key)
  REFERENCES t_share_export_reconciliation_member(reconciliation_id,member_kind,member_key),
 CHECK (acknowledged_revision IS NULL OR acknowledged_revision >= 1),
 CHECK (result_token IS NULL OR result_token > 0),
 CHECK (outcome <> 'deferred' OR reason <> '')
);

-- DB/文件系统不能原子；先持久计划/预留，后写盘，最终 CAS 与 ack 在同一事务。
CREATE TABLE t_share_export_reconciliation_action (
 reconciliation_id UUID NOT NULL,
 member_kind TEXT COLLATE "C" NOT NULL,
 member_key TEXT COLLATE "C" NOT NULL,
 action_key TEXT COLLATE "C" NOT NULL CHECK (action_key <> ''),
 action_kind TEXT NOT NULL CHECK (action_kind IN ('write','verify','mark_stale')),
 export_key TEXT,
 normalized_path TEXT NOT NULL CHECK (normalized_path <> ''),
 expected_output_token BIGINT,
 expected_path_token BIGINT NOT NULL CHECK (expected_path_token > 0),
 reserved_output_token BIGINT,
 reserved_path_token BIGINT,
 state TEXT NOT NULL DEFAULT 'planned'
  CHECK (state IN ('planned','reserved','applied','committed','conflict','unknown')),
 desired_content_hash TEXT,
 desired_mapping_hash TEXT,
 evidence JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(evidence) = 'object'),
 error_text TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (reconciliation_id,member_kind,member_key,action_key),
 FOREIGN KEY (reconciliation_id,member_kind,member_key)
  REFERENCES t_share_export_reconciliation_member(reconciliation_id,member_kind,member_key),
 CHECK (expected_output_token IS NULL OR expected_output_token > 0),
 CHECK (reserved_output_token IS NULL OR reserved_output_token > 0),
 CHECK (reserved_path_token IS NULL OR reserved_path_token > 0)
);
CREATE INDEX share_export_reconciliation_action_pending
 ON t_share_export_reconciliation_action(reconciliation_id,state,member_kind,member_key)
 WHERE state <> 'committed';

-- 除建立不可信的空 fence 外不迁移业务数据；禁止从旧 ready/legacy 标志推断新完成状态。
INSERT INTO t_share_export_recovery_fence(owner_key) VALUES ('share:default');

-- BLOCKERS：全 producer/legacy writer 协同、目录历史区间不重叠守卫、sealed/identity
-- 不可变守卫、token 单调触发器及权限、捕获完整性、终态 CAS 都不在本 DDL 中。
-- DBA 必须审核配套实现和真实并发证明；不得只执行本文件后设置 catalog_ready=true。
COMMIT;
