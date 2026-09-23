package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestCronStartupWiring 防止路由重构遗漏后台调度启动或创建孤立调度实例。
func TestCronStartupWiring(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[string]token.Pos{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, ok := call.Fun.(*ast.Ident); ok {
			positions[name.Name] = call.Pos()
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
			positions["serve"] = call.Pos()
		}
		return true
	})
	if positions["LoadCronTasksFromDB"] <= positions["SetupAuthProtectedRoutes"] || positions["LoadCronTasksFromDB"] >= positions["serve"] {
		t.Error("必须在业务处理器注册完成后、HTTP 服务启动前装载并启动定时任务")
	}
	f, err = parser.ParseFile(fset, "auth.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewCronService" {
			t.Error("业务入口必须复用已注册处理器的 scheduler，不能另建调度实例")
		}
		return true
	})
}

func TestCronBuiltinHandlersRegistered(t *testing.T) {
	previous := scheduler
	t.Cleanup(func() { scheduler.Stop(); scheduler = previous })
	if err := InitCronScheduler(); err != nil {
		t.Fatal(err)
	}
	handlers := map[string]bool{}
	for _, h := range scheduler.Handlers() {
		handlers[h.Key] = h.Execute != nil
	}
	for _, key := range []string{"full_generate", "incremental_sync", "log_cleanup", "identify_cache_cleanup", "cooling_recovery"} {
		if !handlers[key] {
			t.Errorf("处理器 %s 未注册", key)
		}
	}
}
