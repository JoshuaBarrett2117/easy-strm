package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMCPRoutesDispatchAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	c := NewMCPController(fakeMCPAuthenticator{valid: true}, nil)
	router.POST("/mcp", c.HandleMCP)
	calls := 0
	group := router.Group("/items", func(ctx *gin.Context) {
		if ctx.GetHeader("X-API-Key") != "test-key" {
			ctx.AbortWithStatus(401)
			return
		}
		ctx.Next()
	})
	group.PUT("/:id", func(ctx *gin.Context) {
		calls++
		var body map[string]string
		if ctx.ShouldBindJSON(&body) != nil {
			ctx.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		ctx.JSON(200, gin.H{"id": ctx.Param("id"), "tags": ctx.QueryArray("tag"), "body": body})
	})
	group.GET("/:id", func(ctx *gin.Context) { ctx.JSON(404, gin.H{"error": "missing"}) })
	c.RegisterBusinessRoutes(router, nil)
	if len(c.registry.Tools()) != 2 {
		t.Fatalf("unexpected tools: %v", c.registry.Tools())
	}
	name := mcpRouteName(gin.RouteInfo{Method: "PUT", Path: "/items/:id"})
	invoke := func(name string, args string) map[string]interface{} {
		t.Helper()
		request := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
		request.Header.Set("Authorization", "ApiKey test-key")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var result map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	invoke(name, `{"path":{"id":"7"}}`)
	if calls != 0 {
		t.Fatal("write executed without confirmation")
	}
	result := invoke(name, `{"confirm":true,"path":{"id":"7"},"query":{"tag":["a","b"]},"body":{"name":"测试"}}`)
	rpc := result["result"].(map[string]interface{})
	if rpc["isError"] != false || calls != 1 {
		t.Fatalf("dispatch failed: %v", result)
	}
	content := rpc["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(content, `"id":"7"`) || !strings.Contains(content, `"tags":["a","b"]`) {
		t.Fatal(content)
	}
	for _, args := range []string{`{"confirm":true}`, `{"confirm":true,"path":{"id":"../mcp"}}`, `{"confirm":true,"path":{"id":"7"},"query":{"x":3}}`} {
		if invoke(name, args)["error"] == nil {
			t.Fatalf("accepted invalid arguments: %s", args)
		}
	}
	result = invoke(mcpRouteName(gin.RouteInfo{Method: "GET", Path: "/items/:id"}), `{"path":{"id":"7"}}`)
	if result["result"].(map[string]interface{})["isError"] != true {
		t.Fatal(result)
	}
}

func TestMCPRouteUploadBinaryAndContext(t *testing.T) {
	router := gin.New()
	router.POST("/upload", func(c *gin.Context) {
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil || header.Filename != "image.png" || string(data) != "hello" {
			t.Fatalf("bad upload: %v %s", err, data)
		}
		c.Data(200, "image/png", data)
	})
	args := map[string]json.RawMessage{"file": json.RawMessage(`{"name":"image.png","data":"aGVsbG8="}`)}
	result, err := callMCPRoute(context.Background(), router, gin.RouteInfo{Method: "POST", Path: "/upload"}, args)
	if err != nil || result.(mcpRouteResult).Data["base64"] != "aGVsbG8=" {
		t.Fatalf("%v %v", result, err)
	}
	args["body"] = json.RawMessage(`{}`)
	if _, _, err := buildMCPRouteBody(args); err == nil {
		t.Fatal("body/file conflict accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Err() != context.Canceled {
			t.Fatal("cancellation lost")
		}
		w.WriteHeader(204)
	})
	if _, err := callMCPRoute(ctx, handler, gin.RouteInfo{Method: "GET", Path: "/test"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMCPRouteNamesAndExclusions(t *testing.T) {
	router := gin.New()
	router.GET("/excluded", func(c *gin.Context) {})
	before := router.Routes()
	router.GET("/a-b", func(c *gin.Context) {})
	router.GET("/a_b", func(c *gin.Context) {})
	router.POST("/mcp", func(c *gin.Context) {})
	c := NewMCPController(fakeMCPAuthenticator{valid: true}, nil)
	c.RegisterBusinessRoutes(router, before)
	if len(c.registry.Tools()) != 2 {
		t.Fatal(c.registry.Tools())
	}
	names := map[string]bool{}
	for _, route := range router.Routes() {
		name := mcpRouteName(route)
		if names[name] || len(name) > 64 {
			t.Fatal(name)
		}
		names[name] = true
	}
}
