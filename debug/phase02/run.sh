#!/bin/sh
# 更新日期：2026-10-08；维护者：Codex。默认仅显示计划；不得由开发者执行 --execute。
set -eu
if test "$#" -ne 2 || test "$1" != --synthetic-test-only || test "$2" != --execute; then
 echo 'DBA ONLY: backup -> v42 seed -> forward -> checkpoint fixtures -> forward repeat/cmp -> SQL assertions/EXPLAIN/fanout/concurrency -> rollback twice/cmp -> forward again -> v43 bulk DML.'
 echo 'No connection made. DBA execution requires --synthetic-test-only --execute and external PG*/DBA_PHASE02_* variables.'
 exit 0
fi
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$repo/debug/phase02/common.sh"
phase02_authorize
: "${DBA_PHASE02_OUTPUT:?absolute evidence directory required}" "${TMPDIR:?DBA temporary directory required}"
case "$DBA_PHASE02_OUTPUT" in /*) ;; *) echo 'absolute evidence directory required' >&2; exit 2;; esac
umask 077
mkdir -p "$DBA_PHASE02_OUTPUT"
test ! -e "$DBA_PHASE02_OUTPUT/isolated-before.dump" || { echo 'use a new evidence directory; never overwrite backup' >&2; exit 2; }
cd "$repo"
sha256sum -c debug/phase02/migrations.sha256 > "$DBA_PHASE02_OUTPUT/migration-fingerprints.log" 2>&1
phase02_run preflight phase02_psql -f debug/phase02/preflight.sql
phase02_run backup pg_dump -w --host="$PGHOST" --port="$PGPORT" --username="$PGUSER" --dbname="$PGDATABASE" --format=custom --file="$DBA_PHASE02_OUTPUT/isolated-before.dump"
phase02_run backup-list pg_restore --list "$DBA_PHASE02_OUTPUT/isolated-before.dump"
phase02_run indexes-outside-transaction phase02_psql -f debug/phase02/prebuild-indexes.sql
phase02_run seed-before-v43 phase02_psql -f debug/phase02/seed.sql
phase02_run forward-1 phase02_psql -1 -f easy-strm/migrations/migrate_v43_share_export_checkpoint.sql
phase02_run checkpoint-fixture phase02_psql -f debug/phase02/seed-checkpoints.sql
phase02_run repeat-state-fixture phase02_psql -f debug/phase02/idempotency-state.sql
phase02_run state-before-repeat phase02_psql -q -v include_checkpoints=true -f debug/phase02/capture-state.sql
phase02_run forward-2 phase02_psql -1 -f easy-strm/migrations/migrate_v43_share_export_checkpoint.sql
phase02_run state-after-repeat phase02_psql -q -v include_checkpoints=true -f debug/phase02/capture-state.sql
phase02_run forward-state-preserved cmp "$DBA_PHASE02_OUTPUT/state-before-repeat.log" "$DBA_PHASE02_OUTPUT/state-after-repeat.log"
phase02_run trigger-and-cas phase02_psql -f debug/phase02/assertions.sql
phase02_run rebind-orders phase02_psql -f debug/phase02/rebind-orders.sql
mkdir -p "$DBA_PHASE02_OUTPUT/plans"
phase02_run explain phase02_psql -v explain_dir="$DBA_PHASE02_OUTPUT/plans" -f debug/phase02/dao-explain.sql
phase02_run fanout phase02_psql -f debug/phase02/fanout.sql
phase02_run concurrency python3 debug/phase02/concurrency.py --synthetic-test-only --execute
phase02_run legacy-before-rollback phase02_psql -q -v include_checkpoints=false -f debug/phase02/capture-state.sql
phase02_run rollback-1 phase02_psql -1 -f easy-strm/migrations/migrate_v43_share_export_checkpoint_rollback.sql
phase02_run rollback-2 phase02_psql -1 -f easy-strm/migrations/migrate_v43_share_export_checkpoint_rollback.sql
phase02_run legacy-after-rollback phase02_psql -q -v include_checkpoints=false -f debug/phase02/capture-state.sql
phase02_run rollback-preserves-v42 cmp "$DBA_PHASE02_OUTPUT/legacy-before-rollback.log" "$DBA_PHASE02_OUTPUT/legacy-after-rollback.log"
phase02_run forward-3 phase02_psql -1 -f easy-strm/migrations/migrate_v43_share_export_checkpoint.sql
phase02_run fresh-required phase02_psql -f debug/phase02/fresh-required.sql
phase02_run legacy-after-forward-3 phase02_psql -q -v include_checkpoints=false -f debug/phase02/capture-state.sql
phase02_run reinstallation-preserves-v42 cmp "$DBA_PHASE02_OUTPUT/legacy-before-rollback.log" "$DBA_PHASE02_OUTPUT/legacy-after-forward-3.log"
phase02_run post-v43-dml phase02_psql -f debug/phase02/bulk-dml.sql
echo 'SQL protocol script finished. DBA must review raw plans/lock/WAL budgets; no production approval is implied.'
