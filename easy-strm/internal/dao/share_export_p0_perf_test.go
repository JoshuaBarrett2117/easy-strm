package dao

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

func TestShareExportP0StaticPaginationGuards(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\boffset\b|\bcount\s*\(\s*\*\s*\)`)
	for name, query := range map[string]string{
		"dirty": shareExportDirtySQL, "work-sources": shareStrmWorkSourcesSQL,
		"selection-sync":    strings.Join(shareSelectionSyncSQL, "\n"),
		"selection-resolve": strings.Join(shareSelectionPrepareSQL, "\n"),
	} {
		if forbidden.MatchString(query) {
			t.Errorf("导出路径 %s 不能新增 OFFSET 或逐页 COUNT(*)", name)
		}
	}
	for path, names := range map[string][]string{
		"share_export_checkpoint.go":           {"DirtyBatch"},
		"share_strm.go":                        {"StrmSources", "StrmWorkSources", "scanStrmSources", "buildShareStrmSourcesQuery"},
		"../service/share_strm_incremental.go": {"runPrepared", "prepareDirtyWork", "consumeWork"},
		"../service/share_strm_export.go":      {"export"},
	} {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			found := false
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Name.Name != name {
					continue
				}
				found = true
				var body bytes.Buffer
				if err := printer.Fprint(&body, fileSet, function.Body); err != nil {
					t.Fatal(err)
				}
				if forbidden.Match(body.Bytes()) {
					t.Errorf("导出分页 %s:%s 不能新增 OFFSET 或逐页 COUNT(*)", path, name)
				}
				if name == "runPrepared" || name == "prepareDirtyWork" {
					ast.Inspect(function.Body, func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						selector, ok := call.Fun.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						if selector.Sel.Name == "prepareSelectionWork" || selector.Sel.Name == "SyncSelections" || selector.Sel.Name == "ResolveSelections" {
							argument := call.Args[len(call.Args)-1]
							work, scoped := argument.(*ast.SelectorExpr)
							if !scoped || work.Sel.Name != "WorkKey" {
								t.Errorf("增量准备 %s:%s 必须明确限制目标 WorkKey，禁止全局同步/解析", path, name)
							}
						}
						return true
					})
				}
			}
			if !found {
				t.Errorf("静态守卫目标丢失：%s:%s", path, name)
			}
		}
	}
	if !strings.Contains(shareExportDirtySQL, "work_key>$1 ORDER BY work_key LIMIT 100") {
		t.Fatal("待办游标及进度探测上限必须保持 100，与 Service 的精确/下界口径一致")
	}
}
