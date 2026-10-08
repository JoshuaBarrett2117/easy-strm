#!/bin/sh
# 更新日期：2026-10-08；维护者：Codex。只供 DBA 的显式隔离测试，不读取任何 env/凭据文件。
phase02_authorize() {
 : "${PGHOST:?DBA must supply PGHOST}" "${PGPORT:?DBA must supply PGPORT}" "${PGUSER:?DBA must supply PGUSER}" "${PGDATABASE:?DBA must supply PGDATABASE}"
 : "${DBA_PHASE02_TARGET_CONFIRM:?DBA must confirm exact isolated target}" "${DBA_PHASE02_CONFIRM:?DBA must authorize synthetic changes}"
 test "$PGDATABASE" = easy_strm_test || { echo 'Refuse non-approved database' >&2; exit 2; }
 test "$DBA_PHASE02_TARGET_CONFIRM" = "$PGHOST:$PGPORT/$PGDATABASE" || { echo 'Target confirmation mismatch' >&2; exit 2; }
 test "$DBA_PHASE02_CONFIRM" = 'isolated-synthetic-phase02-only' || { echo 'Explicit authorization missing' >&2; exit 2; }
 unset PGSERVICE PGSERVICEFILE
 export PGOPTIONS='-c search_path=public -c statement_timeout=120000 -c lock_timeout=30000'
}
phase02_psql() {
 psql -X -w --host="$PGHOST" --port="$PGPORT" --username="$PGUSER" --dbname="$PGDATABASE" -v ON_ERROR_STOP=1 "$@"
}
phase02_run() {
 label=$1; shift
 set +e
 "$@" > "$DBA_PHASE02_OUTPUT/$label.log" 2>&1
 code=$?
 set -e
 printf '%s\n' "$code" > "$DBA_PHASE02_OUTPUT/$label.exit"
 printf '%s: exit=%s\n' "$label" "$code"
 test "$code" -eq 0 || exit "$code"
}
