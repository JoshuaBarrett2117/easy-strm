package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// MCPController 提供面向第三方 AI Agent 的 MCP JSON-RPC 接口。
type MCPController struct {
	api      MCPAuthenticator
	registry *MCPRegistry
}

// NewMCPController 保留旧构造函数，便于已有装配代码和测试兼容。
func NewMCPController(api MCPAuthenticator, tasks *service.TaskService) *MCPController {
	return NewMCPControllerWithDependencies(MCPDependencies{API: api, Tasks: tasks})
}

// NewMCPControllerWithDependencies 创建带核心业务工具的 MCP Controller。
func NewMCPControllerWithDependencies(deps MCPDependencies) *MCPController {
	return &MCPController{api: deps.API, registry: NewCoreMCPRegistry(deps)}
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func mcpResult(id interface{}, result interface{}) gin.H {
	return gin.H{"jsonrpc": "2.0", "id": id, "result": result}
}
func mcpError(id interface{}, code int, msg string) gin.H {
	return gin.H{"jsonrpc": "2.0", "id": id, "error": gin.H{"code": code, "message": msg}}
}

// HandleMCP 处理 MCP Streamable HTTP 的 JSON 请求（单请求/单响应模式）。
func (c *MCPController) HandleMCP(ctx *gin.Context) {
	key := extractMCPAPIKey(ctx.GetHeader("X-API-Key"), ctx.GetHeader("Authorization"))
	if c.api == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp authentication is not configured"})
		return
	}
	ok, err := c.api.ValidateAPIKey(key)
	if err != nil || !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return
	}
	var req mcpRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, mcpError(nil, -32700, "invalid json"))
		return
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32600, "invalid request"))
		return
	}
	switch req.Method {
	case "initialize":
		ctx.JSON(http.StatusOK, mcpResult(req.ID, gin.H{"protocolVersion": "2024-11-05", "capabilities": gin.H{"tools": gin.H{}}, "serverInfo": gin.H{"name": "easy-strm", "version": "1.0"}}))
	case "notifications/initialized":
		ctx.Status(http.StatusAccepted)
	case "tools/list":
		ctx.JSON(http.StatusOK, mcpResult(req.ID, gin.H{"tools": c.registry.Tools()}))
	case "tools/call":
		c.callTool(ctx, req)
	default:
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32601, "method not found"))
	}
}

// extractMCPAPIKey 支持 X-API-Key 和兼容性的 Authorization: ApiKey 头。
func extractMCPAPIKey(xAPIKey, authorization string) string {
	if strings.TrimSpace(xAPIKey) != "" {
		return strings.TrimSpace(xAPIKey)
	}
	const prefix = "ApiKey "
	if strings.HasPrefix(authorization, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	}
	return ""
}

func (c *MCPController) callTool(ctx *gin.Context, req mcpRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32602, "invalid params"))
		return
	}
	requestContext := context.WithValue(ctx.Request.Context(), mcpHTTPRequestKey{}, ctx.Request)
	data, err := c.registry.Call(requestContext, params.Name, params.Arguments)
	if err != nil {
		code := -32000
		if strings.HasPrefix(err.Error(), "tool not found") {
			code = -32601
		}
		if strings.HasPrefix(err.Error(), "invalid arguments") || strings.HasPrefix(err.Error(), "missing required argument") {
			code = -32602
		}
		ctx.JSON(http.StatusOK, mcpError(req.ID, code, err.Error()))
		return
	}
	isError := false
	if routeResult, ok := data.(mcpRouteResult); ok {
		data = routeResult.Data
		isError = routeResult.Failed
	}
	b, err := json.Marshal(data)
	if err != nil {
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32000, "serialize tool result failed"))
		return
	}
	ctx.JSON(http.StatusOK, mcpResult(req.ID, gin.H{"content": []gin.H{{"type": "text", "text": string(b)}}, "isError": isError}))
}
