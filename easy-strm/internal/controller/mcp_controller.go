package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// MCPController 提供面向第三方 AI Agent 的 MCP JSON-RPC 接口。
type MCPController struct {
	api   *service.GlobalAPIService
	tasks *service.TaskService
}

func NewMCPController(api *service.GlobalAPIService, tasks *service.TaskService) *MCPController {
	return &MCPController{api: api, tasks: tasks}
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
	key := ctx.GetHeader("X-API-Key")
	if key == "" {
		key = ctx.GetHeader("Authorization")
		if len(key) > 7 && key[:7] == "ApiKey " {
			key = key[7:]
		}
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
	switch req.Method {
	case "initialize":
		ctx.JSON(http.StatusOK, mcpResult(req.ID, gin.H{"protocolVersion": "2024-11-05", "capabilities": gin.H{"tools": gin.H{}}, "serverInfo": gin.H{"name": "easy-strm", "version": "1.0"}}))
	case "notifications/initialized":
		ctx.Status(http.StatusAccepted)
	case "tools/list":
		ctx.JSON(http.StatusOK, mcpResult(req.ID, gin.H{"tools": []gin.H{
			{"name": "system_status", "description": "获取系统任务概览", "inputSchema": gin.H{"type": "object", "properties": gin.H{}}},
			{"name": "tasks_recent", "description": "获取最近任务列表", "inputSchema": gin.H{"type": "object", "properties": gin.H{}}},
			{"name": "task_get", "description": "获取指定任务详情", "inputSchema": gin.H{"type": "object", "properties": gin.H{"task_id": gin.H{"type": "string"}}, "required": []string{"task_id"}}},
			{"name": "task_cancel", "description": "取消运行中的任务", "inputSchema": gin.H{"type": "object", "properties": gin.H{"task_id": gin.H{"type": "string"}}, "required": []string{"task_id"}}},
		}}))
	case "tools/call":
		c.callTool(ctx, req)
	default:
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32601, "method not found"))
	}
}

func (c *MCPController) callTool(ctx *gin.Context, req mcpRequest) {
	var p struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32602, "invalid params"))
		return
	}
	var data interface{}
	var err error
	switch p.Name {
	case "system_status", "tasks_recent":
		data, err = c.tasks.GetUnified()
	case "task_get":
		id, _ := p.Arguments["task_id"].(string)
		if id == "" {
			err = context.Canceled
		} else {
			data, err = c.tasks.Get(id)
		}
	case "task_cancel":
		id, _ := p.Arguments["task_id"].(string)
		if id == "" {
			err = context.Canceled
		} else {
			err = c.tasks.Cancel(id)
			data = gin.H{"task_id": id, "cancelled": err == nil}
		}
	default:
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32601, "tool not found"))
		return
	}
	if err != nil {
		ctx.JSON(http.StatusOK, mcpError(req.ID, -32000, err.Error()))
		return
	}
	b, _ := json.Marshal(data)
	ctx.JSON(http.StatusOK, mcpResult(req.ID, gin.H{"content": []gin.H{{"type": "text", "text": string(b)}}, "isError": false}))
}
