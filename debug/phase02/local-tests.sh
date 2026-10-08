#!/bin/sh
# 更新日期：2026-10-08；维护者：Codex。只运行 mock/临时文件系统测试，绝不执行 DBA 脚本。
set -eu
output=${1:?请指定绝对日志目录}
repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
case "$output" in /*) ;; *) echo '日志目录必须是绝对路径' >&2; exit 2;; esac
: "${TMPDIR:?必须保留当前 TMPDIR}"
mkdir -p "$output"
unset EASY_STRM_REAL_115_COPY_TEST EASY_STRM_REAL_115_TEST EASY_STRM_STRM_CONFIG_ID EASY_STRM_LIVE_SHARE_CODE EASY_STRM_LIVE_SHARE_PASSWORD EMBY_REPAIR_USER_ID GOFLAGS ESTRM_PHASE02_SQL_OUTPUT ESTRM_PHASE01_RESULTS ESTRM_PHASE01_BENCH_RESULTS
export GOTOOLCHAIN=local
cd "$repo/easy-strm"
{
 command -v go
 go version
 printf 'TMPDIR=%s\n' "$TMPDIR"
 for name in EASY_STRM_REAL_115_COPY_TEST EASY_STRM_REAL_115_TEST EASY_STRM_STRM_CONFIG_ID EASY_STRM_LIVE_SHARE_CODE EASY_STRM_LIVE_SHARE_PASSWORD EMBY_REPAIR_USER_ID GOFLAGS; do
  eval 'test "${'"$name"'+x}" != x'
  printf '%s=UNSET\n' "$name"
 done
 test -z "$(go env GOFLAGS)"
 printf 'effective_GOFLAGS=EMPTY; no live build tags\n'
} > "$output/resume-test-safety.log" 2>&1
failed=0
run() {
 label=$1; shift
 printf '%s\n' "$*" > "$output/$label.command"
 set +e
 "$@" > "$output/$label.log" 2>&1
 code=$?
 set -e
 printf '%s\n' "$code" > "$output/$label.exit"
 printf '%s: exit=%s\n' "$label" "$code"
 if test "$code" -ne 0; then failed=1; fi
}
run resume-full-test go test ./... -count=1
run resume-race go test -race ./internal/service ./internal/dao -run 'Test(ShareStrm|StrmOutput|StrmExport|ShareExport)' -count=1
run resume-phase01 env ESTRM_PHASE01_RESULTS="$output/resume-phase01.json" go test ./internal/service -run '^TestShareStrmPhase01(Scenarios|FallbackAndLockedRevalidation)$' -count=1 -v
exit "$failed"
