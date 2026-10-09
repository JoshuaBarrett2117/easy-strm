-- v45 分享库导出恢复（A2）语义守卫与最小权限边界：正向。仅由 DBA 显式执行，应用启动不得调用。
-- 更新日期：2026-10-09；维护者：DBA。基础：a2-v45-semantic-guard-request.md；依赖 v44 已建账本。
-- 本文件只做两件事：
--   (1) 为 v44 恢复账本安装最小语义守卫（只增不改不删、token 单调、catalog 区间不重叠）；
--   (2) 定义最小权限运行角色 share_export_recovery_app 并收敛账本对象的 PUBLIC 与角色 ACL。
-- 不含业务列、不重建表、不改 v43/v44 文件。
-- 生产执行前必须单独取得 owner 批准（见 P3）；应用身份切换/旧身份收权不在本 DDL 内。
-- 默认安全拒绝；DBA 审核后须在本事务守卫之前显式设置：
-- SET LOCAL easy_strm.v45_ddl_review = 'V45-ISOLATED-REVIEWED';

\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '30s';

DO $review_guard$
BEGIN
 IF current_setting('easy_strm.v45_ddl_review', true)
    IS DISTINCT FROM 'V45-ISOLATED-REVIEWED' THEN
  RAISE EXCEPTION 'UNAPPROVED：待 DBA 审核后由 DBA 在隔离测试库执行';
 END IF;
 IF to_regclass('t_share_export_recovery_fence') IS NULL
    OR to_regclass('t_share_export_recovery_bootstrap') IS NULL
    OR to_regclass('t_share_export_recovery_token') IS NULL
    OR to_regclass('t_share_export_recovery_catalog') IS NULL
    OR to_regclass('t_share_export_reconciliation') IS NULL
    OR to_regclass('t_share_export_reconciliation_member') IS NULL
    OR to_regclass('t_share_export_reconciliation_ack') IS NULL
    OR to_regclass('t_share_export_reconciliation_action') IS NULL THEN
  RAISE EXCEPTION '缺少 v44 恢复账本前置结构';
 END IF;
END $review_guard$;

-- ---------------------------------------------------------------------------
-- (1) 最小语义守卫函数
-- ---------------------------------------------------------------------------
-- 证据只增：DELETE 一律拒绝（业务删除用 present=false 墓碑表达）。
CREATE FUNCTION share_export_recovery_ledger_no_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'A2_LEDGER_APPEND_ONLY: 禁止删除恢复账本证据 (table=%, op=%)',
    TG_TABLE_NAME, TG_OP;
END $$;

-- 证据只增：TRUNCATE 一律拒绝。
CREATE FUNCTION share_export_recovery_ledger_no_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'A2_LEDGER_APPEND_ONLY: 禁止 TRUNCATE 恢复账本 (table=%)', TG_TABLE_NAME;
END $$;

-- token：身份不可改；version_token 必须严格递增。
CREATE FUNCTION share_export_recovery_token_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.resource_kind IS DISTINCT FROM OLD.resource_kind
     OR NEW.resource_key IS DISTINCT FROM OLD.resource_key THEN
    RAISE EXCEPTION 'A2_TOKEN_IDENTITY_IMMUTABLE: token 主键身份不可改';
  END IF;
  IF NEW.version_token <= OLD.version_token THEN
    RAISE EXCEPTION 'A2_TOKEN_MONOTONIC: token 必须严格递增 (% -> %)',
      OLD.version_token, NEW.version_token;
  END IF;
  RETURN NEW;
END $$;

-- catalog：payload/hash/from/identity 不可改；valid_to 只能 NULL 一次闭合；
-- 同一身份的任意历史半开区间不得重叠（含 INSERT 与 UPDATE）。
CREATE FUNCTION share_export_recovery_catalog_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE overlap INTEGER;
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.owner_key IS DISTINCT FROM OLD.owner_key
       OR NEW.member_kind IS DISTINCT FROM OLD.member_kind
       OR NEW.member_key IS DISTINCT FROM OLD.member_key
       OR NEW.valid_from_epoch IS DISTINCT FROM OLD.valid_from_epoch
       OR NEW.resource_token IS DISTINCT FROM OLD.resource_token
       OR NEW.payload IS DISTINCT FROM OLD.payload
       OR NEW.payload_hash IS DISTINCT FROM OLD.payload_hash THEN
      RAISE EXCEPTION 'A2_CATALOG_IMMUTABLE: catalog 身份/payload/from/version 不可改';
    END IF;
    IF OLD.valid_to_epoch IS NOT NULL
       AND NEW.valid_to_epoch IS DISTINCT FROM OLD.valid_to_epoch THEN
      RAISE EXCEPTION 'A2_CATALOG_CLOSE_ONCE: 已闭合区间不可再次修改 valid_to';
    END IF;
  END IF;
  IF NEW.valid_to_epoch IS NOT NULL THEN
    IF NEW.valid_to_epoch <= NEW.valid_from_epoch THEN
      RAISE EXCEPTION 'A2_CATALOG_INTERVAL: valid_to 必须大于 valid_from';
    END IF;
    SELECT count(*) INTO overlap
      FROM t_share_export_recovery_catalog c
     WHERE c.owner_key = NEW.owner_key
       AND c.member_kind = NEW.member_kind
       AND c.member_key = NEW.member_key
       AND NOT (c.valid_from_epoch = NEW.valid_from_epoch)
       AND int8range(c.valid_from_epoch, c.valid_to_epoch, '[)')
           && int8range(NEW.valid_from_epoch, NEW.valid_to_epoch, '[)');
    IF overlap > 0 THEN
      RAISE EXCEPTION 'A2_CATALOG_OVERLAP: 同一身份历史半开区间重叠';
    END IF;
  END IF;
  RETURN NEW;
END $$;

-- ---------------------------------------------------------------------------
-- (2) 安装触发器
-- ---------------------------------------------------------------------------
DO $install$
DECLARE t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
      't_share_export_recovery_fence','t_share_export_recovery_bootstrap',
      't_share_export_recovery_token','t_share_export_recovery_catalog',
      't_share_export_reconciliation','t_share_export_reconciliation_member',
      't_share_export_reconciliation_ack','t_share_export_reconciliation_action']
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t||'_a2_no_delete', t);
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', t||'_a2_no_truncate', t);
    EXECUTE format(
      'CREATE TRIGGER %I BEFORE DELETE ON %I FOR EACH ROW '
      'EXECUTE FUNCTION share_export_recovery_ledger_no_delete()', t||'_a2_no_delete', t);
    EXECUTE format(
      'CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT '
      'EXECUTE FUNCTION share_export_recovery_ledger_no_truncate()', t||'_a2_no_truncate', t);
  END LOOP;
END $install$;

DROP TRIGGER IF EXISTS t_share_export_recovery_token_a2_monotonic ON t_share_export_recovery_token;
CREATE TRIGGER t_share_export_recovery_token_a2_monotonic
 BEFORE UPDATE ON t_share_export_recovery_token
 FOR EACH ROW EXECUTE FUNCTION share_export_recovery_token_guard();

DROP TRIGGER IF EXISTS t_share_export_recovery_catalog_a2_interval ON t_share_export_recovery_catalog;
CREATE TRIGGER t_share_export_recovery_catalog_a2_interval
 BEFORE INSERT OR UPDATE ON t_share_export_recovery_catalog
 FOR EACH ROW EXECUTE FUNCTION share_export_recovery_catalog_guard();

-- ---------------------------------------------------------------------------
-- (3) 最小权限运行角色与 ACL 收敛
-- ---------------------------------------------------------------------------
DO $roles$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'share_export_recovery_app') THEN
    CREATE ROLE share_export_recovery_app NOLOGIN NOINHERIT;
  END IF;
END $roles$;

REVOKE ALL ON
  t_share_export_recovery_fence,
  t_share_export_recovery_bootstrap,
  t_share_export_recovery_token,
  t_share_export_recovery_catalog,
  t_share_export_reconciliation,
  t_share_export_reconciliation_member,
  t_share_export_reconciliation_ack,
  t_share_export_reconciliation_action
FROM PUBLIC;

REVOKE ALL ON SEQUENCE
  share_export_recovery_epoch_seq,
  share_export_recovery_generation_seq
FROM PUBLIC;

-- 最小访问面：无 TRUNCATE、账本无 DELETE（证据只增不改删），序列仅 USAGE。
GRANT SELECT, INSERT, UPDATE ON
  t_share_export_recovery_fence,
  t_share_export_recovery_bootstrap,
  t_share_export_recovery_token,
  t_share_export_recovery_catalog,
  t_share_export_reconciliation,
  t_share_export_reconciliation_action
TO share_export_recovery_app;
GRANT SELECT, INSERT ON
  t_share_export_reconciliation_member,
  t_share_export_reconciliation_ack
TO share_export_recovery_app;
GRANT USAGE ON SEQUENCE
  share_export_recovery_epoch_seq,
  share_export_recovery_generation_seq
TO share_export_recovery_app;

-- 收敛 v43 生产者函数的 PUBLIC EXECUTE（若存在），仅授权给最小运行角色。
DO $funcs$
BEGIN
  IF to_regprocedure('share_export_enqueue(text[],text)') IS NOT NULL THEN
    REVOKE EXECUTE ON FUNCTION share_export_enqueue(text[],text) FROM PUBLIC;
    GRANT EXECUTE ON FUNCTION share_export_enqueue(text[],text) TO share_export_recovery_app;
  END IF;
  IF to_regprocedure('share_export_produce()') IS NOT NULL THEN
    REVOKE EXECUTE ON FUNCTION share_export_produce() FROM PUBLIC;
    GRANT EXECUTE ON FUNCTION share_export_produce() TO share_export_recovery_app;
  END IF;
  IF to_regprocedure('share_export_require_baseline()') IS NOT NULL THEN
    REVOKE EXECUTE ON FUNCTION share_export_require_baseline() FROM PUBLIC;
    GRANT EXECUTE ON FUNCTION share_export_require_baseline() TO share_export_recovery_app;
  END IF;
  IF to_regprocedure('share_export_checkpoint_invalidated()') IS NOT NULL THEN
    REVOKE EXECUTE ON FUNCTION share_export_checkpoint_invalidated() FROM PUBLIC;
    GRANT EXECUTE ON FUNCTION share_export_checkpoint_invalidated() TO share_export_recovery_app;
  END IF;
END $funcs$;

-- 说明：本 DDL 定义权限边界载体，但不改变任何既有登录身份（含生产运行身份）。
-- 旧写者的持久拒绝需要另有 owner 批准的“运行身份切换到最小权限角色 + 旧身份收权”，
-- 该操作严禁在本文件或生产环境自动执行（见 P3 清单）。
COMMIT;
