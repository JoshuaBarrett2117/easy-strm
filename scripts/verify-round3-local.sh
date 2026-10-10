#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output=${1:-/opt/data/output/estrm-round3/proof}
case "$output" in
  /opt/data/output/estrm-round3/*) ;;
  *) echo "证据目录必须位于 /opt/data/output/estrm-round3/，禁止写入仓库或其他工作树" >&2; exit 2 ;;
esac
output=$(realpath -m "$output")
case "$output" in
  /opt/data/output/estrm-round3/*) ;;
  *) echo "证据路径越界" >&2; exit 2 ;;
esac
mkdir -p "$output"
export PATH=/opt/data/tools/go/bin:/usr/local/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export EASY_STRM_REAL_115_COPY_TEST=0 EASY_STRM_REAL_115_TEST=0
export EASY_STRM_LIVE_SHARE_CODE= EASY_STRM_LIVE_SHARE_PASSWORD= EMBY_REPAIR_USER_ID=
export ESTRM_UUID_CHILD= ESTRM_CLEANUP_CLI_CHILD=
export ESTRM_PHASE02_SQL_OUTPUT= ESTRM_PHASE01_RESULTS= ESTRM_PHASE01_BENCH_RESULTS=
config_directory=$(mktemp -d "$output/npm-config.XXXXXX")
: > "$config_directory/user"
: > "$config_directory/global"
export NPM_CONFIG_USERCONFIG="$config_directory/user" NPM_CONFIG_GLOBALCONFIG="$config_directory/global"
export E2E_OUTPUT_DIR="$output/browser"

cd "$root/easy-strm"
echo "运行后端 build/test（只使用本地 mocks）"
(go build ./... && go test ./... -count=1) > "$output/backend-gates.log" 2>&1
go test . -run 'TestHTTPConcurrentRequestIDsAndThreeGoroutines|TestHTTPUpstreamRequestAndTraceparentReuse|TestRequestIDMultiProcessUniqueness|TestActualServerLifecycleLogsUseAbsentRequestMarker|TestV47ManualMigrationConstraints' -count=1 -v > "$output/http-lifecycle-proof.log" 2>&1
go test ./internal/dao ./cmd/share-cleanup -run 'TestCleanup|TestSelection|TestMockSelection|TestWatermark|TestShareExportCheckpoint' -count=1 -v > "$output/dao-proof.log" 2>&1
go test ./internal/pkg/logger -count=1 -v > "$output/logger-proof.log" 2>&1
go test ./internal/service ./internal/controller -run 'TestActualAsyncExportHTTPAndScheduledTaskTimeline|TestActualCronHTTPAndScheduledTaskTimeline|Test.*Retain.*Context|Test.*Retains.*Context|TestLogViewerHumanAndJSONCompatibility|TestSelectionConcurrentHTTPCASOneWinner|TestSelectedExport24MultiSourceTitlesStableCleanPathsAndLegacyUntouched|TestShareStrmIncremental' -count=1 -v > "$output/service-controller-proof.log" 2>&1
go test -race . ./internal/dao ./internal/pkg/logger ./internal/service -run 'TestHTTPConcurrentRequestIDsAndThreeGoroutines|TestRequestIDMultiProcessUniqueness|TestMockSelectionSyncSerializesReceiptAndObservesCommittedRevision|TestActualAsyncExportHTTPAndScheduledTaskTimeline|TestActualCronHTTPAndScheduledTaskTimeline|TestConcurrentConfigurationAndLogging' -count=1 > "$output/race-proof.log" 2>&1

cd "$root/easy-strm-front"
echo "运行前端 build/单测/真实浏览器（API 全部为本地夹具）"
npm run build > "$output/frontend-build.log" 2>&1
npm run test:share-strm-summary > "$output/frontend-summary.log" 2>&1
npm run e2e:share-selection > "$output/frontend-browser.log" 2>&1
printf '本地门禁和六行证明全部完成，证据目录：%s\n' "$output"
