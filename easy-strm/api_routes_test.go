package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestFrontendAPIRoutes 从真实注册源码构建路由表，避免控制器测试自行注册路由掩盖装配遗漏。
// 只执行空处理器，不连接数据库、不触发转存或删除等业务操作。
func TestFrontendAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	prefixes := map[string]string{"r": ""}
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true, "HEAD": true}
	for _, name := range []string{"auth_routes_public.go", "auth.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if assign, ok := node.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
				if call, ok := assign.Rhs[0].(*ast.CallExpr); ok {
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Group" {
						parent, parentOK := selector.X.(*ast.Ident)
						child, childOK := assign.Lhs[0].(*ast.Ident)
						if parentOK && childOK {
							prefix, exists := prefixes[parent.Name]
							if !exists {
								t.Fatalf("未知路由组 %s", parent.Name)
							}
							prefixes[child.Name] = strings.TrimRight(prefix+routeString(t, call.Args[0]), "/")
						}
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !methods[selector.Sel.Name] {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			prefix, exists := prefixes[receiver.Name]
			if !exists {
				t.Fatalf("未知路由接收者 %s", receiver.Name)
			}
			router.Handle(selector.Sel.Name, prefix+routeString(t, call.Args[0]), func(c *gin.Context) { c.Status(204) })
			return true
		})
	}

	// 覆盖 API 模块及页面中的调用；模板参数使用合法的占位路径段。
	calls := regexp.MustCompile("(?:api\\.(get|post|put|delete|patch)|\\b(request))\\(\\s*['\"`]([^'\"`]+)['\"`]")
	methodOption := regexp.MustCompile(`method:\s*['"]([A-Z]+)['"]`)
	parameter := regexp.MustCompile(`\$\{[^}]+\}`)
	count := 0
	err := filepath.WalkDir("../easy-strm-front/src", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (filepath.Ext(path) != ".js" && filepath.Ext(path) != ".vue") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		for _, match := range calls.FindAllStringSubmatchIndex(source, -1) {
			method := "GET"
			if match[2] >= 0 {
				method = strings.ToUpper(source[match[2]:match[3]])
			}
			if match[4] >= 0 {
				tail := strings.SplitN(source[match[1]:], ")", 2)[0]
				if option := methodOption.FindStringSubmatch(tail); option != nil {
					method = option[1]
				}
			}
			url := parameter.ReplaceAllString(source[match[6]:match[7]], "1")
			count++
			t.Run(method+" "+url, func(t *testing.T) {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(method, url, nil))
				if response.Code != 204 {
					t.Errorf("%s: %s %s 未匹配已注册路由，状态 %d", path, method, url, response.Code)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("未提取到前端 API 调用")
	}
	t.Logf("核对 %d 个前端调用，%d 个后端路由", count, len(router.Routes()))
}

func routeString(t *testing.T, expression ast.Expr) string {
	t.Helper()
	literal, ok := expression.(*ast.BasicLit)
	if !ok {
		t.Fatal("路由路径必须是字符串常量")
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
