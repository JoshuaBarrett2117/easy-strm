package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	pkglogger "easy-strm/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

func TestRequestLoggingAndRecoveryDoNotDumpUntrustedInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	pkglogger.SetOutputs(&output, &output, &output, &output)
	pkglogger.SetLevel(pkglogger.DEBUG)
	t.Cleanup(func() {
		pkglogger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr)
		pkglogger.SetLevel(pkglogger.INFO)
	})
	router := gin.New()
	router.Use(requestLoggingMiddleware(), safeRecoveryMiddleware())
	router.POST("/play/:id", func(ctx *gin.Context) {
		if pkglogger.RequestID(ctx.Request.Context()) == "" {
			t.Error("请求上下文缺少标识")
		}
		panic("unlabelled-synthetic-private-panic")
	})
	request := httptest.NewRequest("POST", "/play/synthetic-private-id?api_key=synthetic-private-key", strings.NewReader("synthetic-private-body"))
	request.Header.Set("Authorization", "Bearer synthetic-private-auth")
	request.Header.Set("Cookie", "synthetic-private-cookie")
	request.Header.Set("User-Agent", "synthetic-private-header")
	request.Header.Set("X-Request-ID", "synthetic-private-client-id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 500 || response.Header().Get("X-Request-ID") == "" {
		t.Fatal("恢复状态或请求标识错误")
	}
	if strings.Contains(output.String(), "synthetic-private") || strings.Count(output.String(), "\n") != 2 {
		t.Fatal("框架日志不得输出路径参数、原始请求或异常值")
	}
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var entry map[string]interface{}
		if json.Unmarshal(line, &entry) != nil || entry["request_id"] != response.Header().Get("X-Request-ID") {
			t.Fatal("框架日志格式或请求关联错误")
		}
	}
	if !strings.Contains(output.String(), `"route":"/play/:id"`) {
		t.Fatal("应记录路由模板而非原始 URL")
	}
	output.Reset()
	if VerifyPassword("synthetic-private-hash", "synthetic-private-input") == nil {
		t.Fatal("密码不匹配必须失败")
	}
	if strings.Contains(output.String(), "synthetic-private") || !strings.Contains(output.String(), `"module":"db_user"`) {
		t.Fatal("旧主程序 API 应通过统一模板且不得输出密码")
	}
}

func TestDirectoryTreeResponsesNeverReachLogs(t *testing.T) {
	var output bytes.Buffer
	pkglogger.SetOutputs(&output, &output, &output, &output)
	pkglogger.SetLevel(pkglogger.DEBUG)
	t.Cleanup(func() {
		pkglogger.SetOutputs(os.Stdout, os.Stdout, os.Stderr, os.Stderr)
		pkglogger.SetLevel(pkglogger.INFO)
	})
	client := &Client{httpClient: &http.Client{Transport: proxyRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("synthetic-private-response-body"))}, nil
	})}}
	if _, err := client.ExportDirectoryTree115("123", "456", "synthetic-private-cookie"); err == nil || strings.Contains(err.Error(), "synthetic-private") {
		t.Fatal("目录树解析失败不得向上返回原始响应体")
	}
	if _, err := client.GetExportDirectoryTreeStatus("123", "synthetic-private-cookie"); err == nil || strings.Contains(err.Error(), "synthetic-private") {
		t.Fatal("目录树状态解析失败不得向上返回原始响应体")
	}
	if strings.Contains(output.String(), "synthetic-private") {
		t.Fatal("目录树日志不得输出 Cookie 或响应体")
	}
}

func TestLegacyCredentialSourcesDoNotPassRawValuesToLogs(t *testing.T) {
	for _, file := range []string{"115client.go", "db_user.go", "db_cloud115.go", "db_system_config.go"} {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			function, ok := call.Fun.(*ast.Ident)
			if !ok || (function.Name != "Debug" && function.Name != "Info" && function.Name != "Warn" && function.Name != "Error") {
				return true
			}
			for _, argument := range call.Args {
				if selector, ok := argument.(*ast.SelectorExpr); ok {
					switch selector.Sel.Name {
					case "UID", "Name", "UserName", "Cookie", "Password", "ConfigVal":
						t.Errorf("%s 向日志传入了敏感字段 %s", file, selector.Sel.Name)
					}
				}
			}
			return true
		})
	}
}
