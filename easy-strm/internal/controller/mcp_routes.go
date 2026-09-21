package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

type mcpHTTPRequestKey struct{}

// RegisterBusinessRoutes 将新增的业务路由逐一注册为工具；启动装配时调用，保留原鉴权和中间件。
func (c *MCPController) RegisterBusinessRoutes(engine *gin.Engine, before gin.RoutesInfo) {
	excluded := make(map[string]bool, len(before))
	for _, route := range before {
		excluded[route.Method+" "+route.Path] = true
	}
	for _, route := range engine.Routes() {
		if excluded[route.Method+" "+route.Path] || route.Path == "/mcp" {
			continue
		}
		c.registry.Register(buildMCPRouteTool(engine, route))
	}
}

var mcpNameCharacters = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func mcpRouteName(route gin.RouteInfo) string {
	name := "api_" + strings.ToLower(route.Method) + "_" + strings.Trim(mcpNameCharacters.ReplaceAllString(strings.ReplaceAll(route.Path, ":", "by_"), "_"), "_")
	// 摘要区分短横线、下划线及截断后相同的路由，名称满足客户端的 64 字符限制。
	hash := sha256.Sum256([]byte(route.Method + " " + route.Path))
	if len(name) > 51 {
		name = name[:51]
	}
	return fmt.Sprintf("%s_%x", name, hash[:6])
}

func buildMCPRouteTool(engine http.Handler, route gin.RouteInfo) MCPTool {
	pathProperties := map[string]interface{}{}
	var required []string
	for _, part := range strings.Split(route.Path, "/") {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			key := part[1:]
			pathProperties[key] = map[string]interface{}{"type": "string", "minLength": 1}
			required = append(required, key)
		}
	}
	properties := map[string]interface{}{
		"path":    objectSchema(pathProperties, required...),
		"query":   map[string]interface{}{"type": "object", "description": "原 HTTP 查询参数，值为字符串或字符串数组（重复参数）", "additionalProperties": map[string]interface{}{"oneOf": []interface{}{map[string]interface{}{"type": "string"}, map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}}}}},
		"body":    map[string]interface{}{"description": "原接口 JSON 请求体，字段、必填约束和业务校验与 Web API 一致；可为对象或数组"},
		"file":    objectSchema(map[string]interface{}{"name": stringProperty("上传文件名"), "data": stringProperty("文件内容的标准 Base64 编码；以 multipart/form-data 的 file 字段上传")}, "name", "data"),
		"confirm": boolProperty("执行写入或有副作用的操作时传 true", false),
	}
	readOnly := route.Method == http.MethodGet || route.Method == http.MethodHead
	return MCPTool{
		Name: mcpRouteName(route), Description: fmt.Sprintf("%s %s（%s）。完整业务接口，path 填路径变量，query 填查询参数，body 填原 JSON 请求体，file 用于文件上传。返回 status、content_type 和 body；二进制返回 base64。写操作需要 confirm=true。", route.Method, route.Path, route.Handler),
		InputSchema: objectSchema(properties), ReadOnlyHint: readOnly, DestructiveHint: !readOnly,
		Handler: func(ctx context.Context, args map[string]json.RawMessage) (interface{}, error) {
			return callMCPRoute(ctx, engine, route, args)
		},
	}
}

func callMCPRoute(ctx context.Context, engine http.Handler, route gin.RouteInfo, args map[string]json.RawMessage) (interface{}, error) {
	var path map[string]string
	if raw, ok := args["path"]; ok {
		if err := json.Unmarshal(raw, &path); err != nil {
			return nil, fmt.Errorf("invalid arguments: path: %w", err)
		}
	}
	parts := strings.Split(route.Path, "/")
	used := map[string]bool{}
	for i, part := range parts {
		if !strings.HasPrefix(part, ":") && !strings.HasPrefix(part, "*") {
			continue
		}
		key := part[1:]
		value := path[key]
		if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\?#%\r\n") {
			return nil, fmt.Errorf("invalid arguments: path.%s 必须是非空路径段", key)
		}
		used[key] = true
		parts[i] = url.PathEscape(value)
	}
	for key := range path {
		if !used[key] {
			return nil, fmt.Errorf("invalid arguments: unknown path.%s", key)
		}
	}
	query := url.Values{}
	var values map[string]json.RawMessage
	if raw, ok := args["query"]; ok {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("invalid arguments: query: %w", err)
		}
	}
	for key, raw := range values {
		var value string
		if json.Unmarshal(raw, &value) == nil && string(raw) != "null" {
			query.Add(key, value)
			continue
		}
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil || string(raw) == "null" {
			return nil, fmt.Errorf("invalid arguments: query.%s 需要字符串或字符串数组", key)
		}
		for _, value := range list {
			query.Add(key, value)
		}
	}
	body, contentType, err := buildMCPRouteBody(args)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, route.Method, strings.Join(parts, "/")+"?"+query.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if original, ok := ctx.Value(mcpHTTPRequestKey{}).(*http.Request); ok {
		request.Header.Set("X-API-Key", extractMCPAPIKey(original.Header.Get("X-API-Key"), original.Header.Get("Authorization")))
		request.Header.Set("User-Agent", original.UserAgent())
		request.RemoteAddr = original.RemoteAddr
		request.Host = original.Host
	}
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	result := map[string]interface{}{"status": response.Code, "content_type": response.Header().Get("Content-Type")}
	if location := response.Header().Get("Location"); location != "" {
		result["location"] = location
	}
	var data interface{}
	if json.Unmarshal(response.Body.Bytes(), &data) == nil {
		result["body"] = data
	} else if strings.HasPrefix(response.Header().Get("Content-Type"), "text/") {
		result["body"] = response.Body.String()
	} else {
		result["base64"] = base64.StdEncoding.EncodeToString(response.Body.Bytes())
	}
	return mcpRouteResult{Data: result, Failed: response.Code >= 400}, nil
}

type mcpRouteResult struct {
	Data   map[string]interface{}
	Failed bool
}

func buildMCPRouteBody(args map[string]json.RawMessage) ([]byte, string, error) {
	raw, upload := args["file"]
	if !upload {
		return args["body"], "application/json", nil
	}
	if _, exists := args["body"]; exists {
		return nil, "", fmt.Errorf("invalid arguments: body 和 file 不能同时提供")
	}
	var file struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil || file.Name == "" {
		return nil, "", fmt.Errorf("invalid arguments: file 需要 name 和 data")
	}
	data, err := base64.StdEncoding.DecodeString(file.Data)
	if err != nil {
		return nil, "", fmt.Errorf("invalid arguments: file.data 不是 Base64")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return nil, "", err
	}
	if _, err = part.Write(data); err != nil {
		return nil, "", err
	}
	if err = writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}
