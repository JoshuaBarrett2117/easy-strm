package dao

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"easy-strm/internal/domain"
)

var performanceUIOffsetAllowlist = map[string]string{
	"internal/dao/cron_registry.go:Runs":                       "UI 随机页任务历史",
	"internal/dao/offline_download_task_dao.go:List":           "UI 随机页离线任务",
	"internal/dao/share_library.go:Library":                    "UI 海报墙随机页及总数",
	"internal/dao/share_library.go:LibrarySources":             "UI 作品来源随机页及总数",
	"internal/dao/share_media_page.go:buildShareRecordParents": "UI 分享随机页；Cursor 内部分支禁止 OFFSET",
	"internal/dao/share_media_page.go:ListMedia":               "UI 媒体卡片随机页及全分享重复数",
	"internal/dao/share_media_page.go:ListFiles":               "UI 文件随机页及总数",
	"internal/dao/share_media_page.go:ListReviewItems":         "UI 手动核对随机页及总数",
	"internal/dao/share_record_dao.go:List":                    "UI 分享随机页及总数",
	"internal/dao/share_selection.go:ListSelections":           "UI 默认随机页及总数；内部 Cursor/SkipCount 分支不使用 OFFSET/count",
}

var performanceAggregateAllowlist = map[string]string{
	"internal/dao/share_cleanup.go:BuildShareCleanupSQL":          "仅生成 DBA 清理脚本；固定表清单，各表一次汇总，不是翻页",
	"internal/dao/share_cleanup.go:ReadShareCleanupCounts":        "DBA 清理工具固定表清单，各表一次汇总，不是翻页",
	"internal/dao/share_export_checkpoint.go:CheckSchema":         "一次性 pg_trigger 目录验证，固定 28+4 个触发器",
	"internal/dao/share_export_batch.go:shareExportStaleBatchSQL": "每作品一次 CAS 更新数/目标数汇总，不是翻页总数",
}

var performanceOffsetPattern = regexp.MustCompile(`(?i)\boffset\s+(?:\$|[0-9]|%|:)`)
var performanceWindowPattern = regexp.MustCompile(`(?i)count\s*\(\s*\*\s*\)\s*over\s*\(`)
var performanceCountPattern = regexp.MustCompile(`(?i)count\s*\(\s*\*\s*\)`)

type performanceVisitor struct {
	file, symbol string
	loopDepth    int
	positions    *token.FileSet
	violations   *[]string
}

func (visitor performanceVisitor) reject(node ast.Node, reason string) {
	*visitor.violations = append(*visitor.violations, fmt.Sprintf("%s:%d [%s] %s", visitor.file, visitor.positions.Position(node.Pos()).Line, visitor.symbol, reason))
}

func (visitor performanceVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	switch value := node.(type) {
	case *ast.FuncDecl:
		visitor.symbol = value.Name.Name
	case *ast.ValueSpec:
		if len(value.Names) > 0 {
			visitor.symbol = value.Names[0].Name
		}
	case *ast.ForStmt, *ast.RangeStmt:
		visitor.loopDepth++
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			break
		}
		query, err := strconv.Unquote(value.Value)
		if err != nil {
			break
		}
		if performanceWindowPattern.MatchString(query) {
			visitor.reject(node, "禁止 count(*) OVER 全集窗口统计")
		}
		if performanceOffsetPattern.MatchString(query) && performanceUIOffsetAllowlist[visitor.file+":"+visitor.symbol] == "" {
			visitor.reject(node, "非 UI 白名单 SQL 禁止 OFFSET")
		}
		allowedAggregate := performanceAggregateAllowlist[visitor.file+":"+visitor.symbol] != ""
		if visitor.loopDepth > 0 && performanceCountPattern.MatchString(query) && !allowedAggregate {
			visitor.reject(node, "循环内禁止每页 count(*)")
		}
		bulk := strings.Contains(visitor.file, "export") || strings.Contains(visitor.file, "selection_batch")
		if bulk && performanceCountPattern.MatchString(query) && !allowedAggregate && performanceUIOffsetAllowlist[visitor.file+":"+visitor.symbol] == "" {
			visitor.reject(node, "导出批处理禁止 count(*)，仅允许完成检查点的一次 CAS 汇总")
		}
	case *ast.CallExpr:
		selector, ok := value.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}
		name := selector.Sel.Name
		if visitor.loopDepth > 0 && (name == "ListSelections" || name == "ListMedia" || name == "ListFiles" || name == "Library" || name == "LibrarySources") {
			visitor.reject(node, "禁止批量循环调用带 UI 分页/总数的列表")
		}
		if name == "ResolveSelection" && visitor.file+":"+visitor.symbol != "internal/service/share_selection_export.go:exportLocalStrm" {
			visitor.reject(node, "准备阶段禁止逐条 ResolveSelection 事务；exportLocalStrm 的逐文件版本校验除外")
		}
		if visitor.loopDepth > 0 && (name == "BeginTx" || name == "Begin") {
			visitor.reject(node, "循环内禁止逐行开启事务")
		}
		localCount := false
		if receiver, ok := selector.X.(*ast.Ident); ok {
			localCount = receiver.Name == "strings" || receiver.Name == "bytes"
		}
		if visitor.loopDepth > 0 && strings.HasPrefix(name, "Count") && !localCount {
			visitor.reject(node, "循环内禁止调用 COUNT 总数 API")
		}
	}
	return visitor
}

func scanProductionPerformance(root string) ([]string, int, error) {
	positions := token.NewFileSet()
	violations := []string{}
	files := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(positions, path, nil, 0)
		if err != nil {
			return err
		}
		files++
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		ast.Walk(performanceVisitor{file: filepath.ToSlash(relative), positions: positions, violations: &violations}, parsed)
		return nil
	})
	return violations, files, err
}

func TestSQLPerformanceGuardProduction(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	violations, files, err := scanProductionPerformance(filepath.Join(filepath.Dir(filename), "../.."))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AST performance guard scanned %d production Go files; UI allowlist is symbol-scoped, not whole-file", files)
	for _, violation := range violations {
		t.Error(violation)
	}
	for name, query := range map[string]string{"full": func() string { value, _ := buildShareStrmSourcesQuery(domain.ShareLibraryQuery{}, 0); return value }(), "work": shareStrmWorkSourcesSQL, "dirty": shareExportDirtySQL} {
		if performanceOffsetPattern.MatchString(query) || performanceCountPattern.MatchString(query) || performanceWindowPattern.MatchString(query) {
			t.Errorf("%s: bulk cursor contains pagination aggregate", name)
		}
	}
}

func TestSQLPerformanceGuardRejectsIntroducedPatterns(t *testing.T) {
	for _, source := range []string{
		"package fixture; const query = `SELECT id FROM files LIMIT $1 OFFSET $2`",
		"package fixture; const query = `SELECT id FROM files LIMIT 100 OFFSET 10000`",
		"package fixture; const query = `SELECT count(*) OVER(PARTITION BY id) FROM files`",
		"package fixture; func prepare(s any) { for { s.Query(`SELECT count(*) FROM files`) } }",
		"package fixture; func export(s any) { for { s.ListSelections() } }",
		"package fixture; func prepareSelectionWork(s any) { s.ResolveSelection() }",
		"package fixture; func bulk(s any) { for { s.BeginTx() } }",
		"package fixture; const countSQL = `SELECT count(*) FROM files`",
		"package fixture; func export(s any) { for { s.CountSelections() } }",
	} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "new_export.go"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		violations, _, err := scanProductionPerformance(root)
		if err != nil || len(violations) == 0 {
			t.Fatalf("source=%s violations=%v err=%v", source, violations, err)
		}
	}
}
