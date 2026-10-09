package dao

import (
	"easy-strm/internal/domain"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func phase02ExplainSQL() string {
	oldWork, _ := buildShareStrmSourcesQuery(domain.ShareLibraryQuery{WorkKey: "synthetic"}, 0)
	oldFile, _ := buildShareStrmSourcesQuery(domain.ShareLibraryQuery{FileIDs: []int{300000001}}, 300000000)
	oldFull, _ := buildShareStrmSourcesQuery(domain.ShareLibraryQuery{}, 0)
	queries := []struct{ name, types, sql, args string }{
		{"before_full", "integer", oldFull, "0"},
		{"before_work_and_conflicts", "text,integer", oldWork, "'[\"tmdb\", \"\", \"movie\", \"200000001\"]',0"},
		{"before_locked_file", "integer[],integer", oldFile, "ARRAY[300000001],300000000"},
		{"after_work_and_conflicts", "text,integer", shareStrmWorkSourcesSQL, "'[\"tmdb\", \"\", \"movie\", \"200000001\"]',0"},
		{"after_cold_work", "text,integer", shareStrmWorkSourcesSQL, "'[\"tmdb\", \"\", \"movie\", \"200000003\"]',0"},
		{"after_locked_file", "integer", shareStrmFileSourceSQL, "300000001"},
		{"after_work_observation", "text", shareExportObservedSQL, "'[\"tmdb\", \"\", \"movie\", \"200000001\"]'"},
		{"after_dirty", "text[]", shareExportDirtySQL, "ARRAY[]::text[]"},
		{"before_check_path", "text", `SELECT owner_key,export_key FROM t_strm_export_state WHERE output_path=$1 AND state<>'missing' UNION ALL SELECT owner_key,'' FROM t_strm_export_history WHERE output_path=$1`, "'/phase02-synthetic/owned-1.strm'"},
		{"check_path", "text", strmExportCheckPathSQL, "'/phase02-synthetic/owned-1.strm'"},
		{"check_history_path", "text", strmExportCheckPathSQL, "'/phase02-synthetic/history-1.strm'"},
		{"check_legacy_path", "text", strmExportCheckPathSQL, "'/phase02-synthetic/legacy-1.strm'"},
	}
	var output strings.Builder
	output.WriteString("-- 更新日期：2026-10-08；维护者：Codex。由 Go 测试从实际 DAO 常量/构建器生成，禁止手工漂移。\n\\set ON_ERROR_STOP on\n\\pset format unaligned\n\\pset tuples_only on\nDO $$ BEGIN IF current_database()<>'easy_strm_test' THEN RAISE EXCEPTION '仅允许 easy_strm_test'; END IF; END $$;\n")
	for _, query := range queries {
		fmt.Fprintf(&output, "PREPARE phase02_%s(%s) AS\n%s;\n", query.name, query.types, query.sql)
	}
	for _, variant := range []string{"catalog-indexes", "without-status-index"} {
		fmt.Fprintf(&output, "\\echo %s\nBEGIN;\nSET LOCAL application_name='easy-strm-phase02-explain';\n", variant)
		if variant == "without-status-index" {
			output.WriteString("DROP INDEX IF EXISTS idx_share_media_status;\n")
		}
		for _, query := range queries {
			fmt.Fprintf(&output, "\\echo %s\nEXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) EXECUTE phase02_%s(%s);\n\\set phase02_plan_file :explain_dir '/%s-%s.json'\nEXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_%s(%s)\n\\g :phase02_plan_file\n", query.name, query.name, query.args, variant, query.name, query.name, query.args)
		}
		output.WriteString("SAVEPOINT zero_dirty;\nUPDATE t_share_export_dirty_work SET acked_revision=revision,pending_export_keys='[]';\nEXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_dirty(ARRAY[]::text[]);\nDO $$ BEGIN IF EXISTS(SELECT 1 FROM t_share_export_dirty_work WHERE revision>acked_revision) THEN RAISE EXCEPTION 'not zero dirty'; END IF; END $$;\nROLLBACK TO SAVEPOINT zero_dirty;\nSAVEPOINT old_path_indexes;\nDROP INDEX IF EXISTS idx_strm_export_state_path;\nDROP INDEX IF EXISTS idx_strm_export_history_path;\nEXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/owned-1.strm');\nEXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_check_path('/phase02-synthetic/history-1.strm');\nROLLBACK TO SAVEPOINT old_path_indexes;\n")
		output.WriteString("\\echo affected-100-work-new-and-old\nSELECT format('EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_after_work_and_conflicts(%L,0);',work_key) FROM t_share_media WHERE id BETWEEN 200000001 AND 200000100 ORDER BY id\n\\gexec\nSELECT format('EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE phase02_before_work_and_conflicts(%L,0);',work_key) FROM t_share_media WHERE id BETWEEN 200000001 AND 200000100 ORDER BY id\n\\gexec\n")
		output.WriteString("ROLLBACK;\n")
	}
	for _, query := range queries {
		fmt.Fprintf(&output, "DEALLOCATE phase02_%s;\n", query.name)
	}
	return output.String()
}

// TestShareExportMediaDiagnosticHarness 离线验证媒体诊断更新真实改变时间戳且不投递，并阻止 v33 已删除字段回流。
func TestShareExportMediaDiagnosticHarness(test *testing.T) {
	data, err := os.ReadFile("../../../debug/phase02/assertions.sql")
	if err != nil {
		test.Fatal(err)
	}
	sql := string(data)
	start := strings.Index(sql, "DO $media_changes$")
	end := strings.Index(sql, "END $media_changes$;")
	if start < 0 || end <= start {
		test.Fatal("缺少完整媒体断言块")
	}
	media := sql[start:end]
	for _, column := range []string{"error", "status", "share_id", "file_name"} {
		if regexp.MustCompile(`(?i)\b` + column + `\s*=`).MatchString(media) {
			test.Errorf("媒体断言引用 v33 已删除字段：%s", column)
		}
	}
	for _, required := range []string{
		"SELECT updated_at INTO STRICT diagnostic_before FROM t_share_media WHERE id=200000003;",
		"diagnostic_before IS NULL OR NOT isfinite(diagnostic_before)",
		"UPDATE t_share_media SET updated_at=diagnostic_before+interval '1 microsecond' WHERE id=200000003 RETURNING updated_at INTO diagnostic_after;",
		"GET DIAGNOSTICS diagnostic_rows=ROW_COUNT;",
		"diagnostic_rows<>1 OR diagnostic_after IS NULL OR diagnostic_after IS NOT DISTINCT FROM diagnostic_before",
		"IF pg_temp.revision(work) IS DISTINCT FROM previous THEN RAISE EXCEPTION '媒体纯诊断字段错误投递'; END IF;",
	} {
		if !strings.Contains(media, required) {
			test.Errorf("媒体诊断断言缺少保护：%s", required)
		}
	}
	update := regexp.MustCompile(`(?i)UPDATE\s+t_share_media\s+SET\s+updated_at=([^;]+);`).FindString(media)
	if update == "" || strings.Contains(update, "now()") {
		test.Error("诊断时间戳必须确定变化，不能依赖事务内稳定的 now()")
	}
	if !strings.Contains(sql, "UPDATE t_share_media_file SET updated_at=now(),last_seen_at=now(),last_seen_scan_token='synthetic-scan-only',error='synthetic-diagnostic-only' WHERE id=300000001;") {
		test.Error("文件表仍有 error，不能移除合法的文件诊断断言")
	}
}

func TestPhase02ExactDAOSQLHarness(t *testing.T) {
	expected := phase02ExplainSQL()
	if output := os.Getenv("ESTRM_PHASE02_SQL_OUTPUT"); output != "" {
		if err := os.WriteFile(output, []byte(expected), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	actual, err := os.ReadFile("../../../debug/phase02/dao-explain.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != expected {
		t.Fatal("DBA EXPLAIN harness 与实际 DAO SQL 不一致；先重新生成再审核")
	}
}
