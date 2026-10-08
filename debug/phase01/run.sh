#!/bin/sh
set -eu
phase=${1:?usage: sh debug/phase01/run.sh baseline|after output-directory}
output=${2:?output-directory is required}
case "$phase" in baseline|after) ;; *) exit 2 ;; esac
: "${TMPDIR:?TMPDIR must be supplied}"
export GOTOOLCHAIN=local GOPROXY=https://goproxy.cn,direct
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
mkdir -p "$output"
cd "$root/easy-strm"
run() {
    label=$1
    shift
    set +e
    "$@" > "$output/$phase-$label.log" 2>&1
    status=$?
    set -e
    printf '%s\n' "$status" > "$output/$phase-$label.exit"
    cat "$output/$phase-$label.log"
    return "$status"
}
run test /opt/data/tools/go/bin/go test ./...
run scenarios env ESTRM_PHASE01_RESULTS="$output/$phase.json" /opt/data/tools/go/bin/go test ./internal/service -run TestShareStrmPhase01 -count=1
if [ -e "$output/$phase-benchmark.jsonl" ]; then
    printf 'benchmark JSONL already exists; use a fresh output directory or explicitly archive the old file\n' >&2
    exit 2
fi
run benchmark env ESTRM_PHASE01_BENCH_RESULTS="$output/$phase-benchmark.jsonl" /opt/data/tools/go/bin/go test ./internal/service -run '^$' -bench '^BenchmarkShareStrmPhase01$' -benchtime=1x -count=3 -benchmem
