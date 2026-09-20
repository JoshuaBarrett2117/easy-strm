package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMCPRegistryToolsAreStableAndAnnotated(t *testing.T) {
	registry := NewMCPRegistry(MCPTool{Name: "z_tool", Description: "z", InputSchema: objectSchema(map[string]interface{}{}), ReadOnlyHint: true, IdempotentHint: true, Handler: func(context.Context, map[string]json.RawMessage) (interface{}, error) {
		return map[string]bool{"ok": true}, nil
	}}, MCPTool{Name: "a_tool", Description: "a", InputSchema: objectSchema(map[string]interface{}{"confirm": boolProperty("confirm", false)}), DestructiveHint: true, Handler: func(context.Context, map[string]json.RawMessage) (interface{}, error) { return "done", nil }})
	tools := registry.Tools()
	if len(tools) != 2 || tools[0]["name"] != "a_tool" || tools[1]["name"] != "z_tool" {
		t.Fatalf("unexpected stable tool order: %#v", tools)
	}
	annotations := tools[0]["annotations"].(map[string]bool)
	if !annotations["destructiveHint"] || annotations["readOnlyHint"] {
		t.Fatalf("unexpected annotations: %#v", annotations)
	}
	if _, ok := tools[0]["inputSchema"].(map[string]interface{}); !ok {
		t.Fatal("input schema missing")
	}
}

func TestMCPRegistryRequiresConfirmationAndDispatchesTypedArguments(t *testing.T) {
	called := false
	registry := NewMCPRegistry(MCPTool{Name: "delete_item", DestructiveHint: true, InputSchema: objectSchema(map[string]interface{}{"item_id": intProperty("id"), "confirm": boolProperty("confirm", false)}, "item_id", "confirm"), Handler: func(_ context.Context, args map[string]json.RawMessage) (interface{}, error) {
		called = true
		var id int
		if err := decodeArg(args, "item_id", &id, true); err != nil {
			return nil, err
		}
		return map[string]int{"item_id": id}, nil
	}})
	result, err := registry.Call(context.Background(), "delete_item", json.RawMessage(`{"item_id":7}`))
	if err != nil || called {
		t.Fatalf("confirmation should stop dispatch: result=%#v err=%v called=%v", result, err, called)
	}
	confirmation := result.(map[string]interface{})
	if confirmation["confirmation_required"] != true {
		t.Fatalf("unexpected confirmation result: %#v", result)
	}
	result, err = registry.Call(context.Background(), "delete_item", json.RawMessage(`{"item_id":7,"confirm":true}`))
	if err != nil || !called || result.(map[string]int)["item_id"] != 7 {
		t.Fatalf("typed dispatch failed: result=%#v err=%v called=%v", result, err, called)
	}
}

func TestExtractMCPAPIKey(t *testing.T) {
	if got := extractMCPAPIKey(" direct ", "ApiKey fallback"); got != "direct" {
		t.Fatalf("X-API-Key precedence failed: %q", got)
	}
	if got := extractMCPAPIKey("", "ApiKey secret"); got != "secret" {
		t.Fatalf("Authorization compatibility failed: %q", got)
	}
	if got := extractMCPAPIKey("", "Bearer secret"); got != "" {
		t.Fatalf("unexpected bearer key: %q", got)
	}
}

type fakeMCPAuthenticator struct{ valid bool }

func (f fakeMCPAuthenticator) ValidateAPIKey(string) (bool, error) { return f.valid, nil }

func TestMCPControllerAuthAndProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewMCPController(fakeMCPAuthenticator{valid: true}, nil)
	router.POST("/mcp", controller.HandleMCP)
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	request.Header.Set("X-API-Key", "test-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["result"] == nil {
		t.Fatalf("missing result: %s", response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"unknown","arguments":{}}}`))
	request.Header.Set("Authorization", "ApiKey test-key")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "-32601") {
		t.Fatalf("unexpected unknown tool response: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`))
	response = httptest.NewRecorder()
	unauthorizedRouter := gin.New()
	unauthorizedController := NewMCPController(fakeMCPAuthenticator{valid: false}, nil)
	unauthorizedRouter.POST("/mcp", unauthorizedController.HandleMCP)
	unauthorizedRouter.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
}
