package controller

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"easy-strm/internal/domain"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// EmbyManagementController 提供多实例 Emby 管理接口。
type EmbyManagementController struct {
	service *service.EmbyManagementService
}

// NewEmbyManagementController 创建 Emby 管理控制器。
func NewEmbyManagementController(managementService *service.EmbyManagementService) *EmbyManagementController {
	return &EmbyManagementController{service: managementService}
}

func (c *EmbyManagementController) serverID(ctx *gin.Context) (int, bool) {
	id, err := strconv.Atoi(ctx.Param("server_id"))
	if err != nil || id <= 0 {
		ErrorResp(ctx, http.StatusBadRequest, "无效的 Emby 实例 ID")
		return 0, false
	}
	return id, true
}

// ListServers 查询实例列表。
func (c *EmbyManagementController) ListServers(ctx *gin.Context) {
	servers, err := c.service.ListServers()
	if err != nil {
		ErrorResp(ctx, 500, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": servers, "total": len(servers)})
}

// CreateServer 新增实例。
func (c *EmbyManagementController) CreateServer(ctx *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		APIKey    string `json:"api_key"`
		Enabled   bool   `json:"enabled"`
		IsDefault bool   `json:"is_default"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	server, taskID, err := c.service.CreateServerTask(req.Name, req.BaseURL, req.APIKey, req.Enabled, req.IsDefault)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"server": server, "task_id": taskID})
}

// UpdateServer 修改实例。
func (c *EmbyManagementController) UpdateServer(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		APIKey    string `json:"api_key"`
		Enabled   bool   `json:"enabled"`
		IsDefault bool   `json:"is_default"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	server, taskID, err := c.service.UpdateServerTask(id, req.Name, req.BaseURL, req.APIKey, req.Enabled, req.IsDefault)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"server": server, "task_id": taskID})
}

// DeleteServer 删除实例连接配置。
func (c *EmbyManagementController) DeleteServer(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	taskID, err := c.service.DeleteServerTask(id)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"message": "Emby 实例已删除", "task_id": taskID})
}

// CheckServer 检查实例连接。
func (c *EmbyManagementController) CheckServer(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	info, err := c.service.CheckServerConnection(id)
	if err != nil {
		SuccessResp(ctx, gin.H{"connected": false, "error": err.Error()})
		return
	}
	SuccessResp(ctx, gin.H{"connected": true, "info": info})
}

// ListUsers 查询 Emby 用户。
func (c *EmbyManagementController) ListUsers(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	users, err := c.service.ListUsers(id)
	if err != nil {
		ErrorResp(ctx, 502, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": users, "total": len(users)})
}

// CreateUser 新增 Emby 用户。
func (c *EmbyManagementController) CreateUser(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	taskID, user, err := c.service.CreateUser(id, req.Name, req.Password)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID, "user": user})
}

// UpdateUser 修改用户名称和常用权限。
func (c *EmbyManagementController) UpdateUser(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		Name   string                 `json:"name"`
		Policy *domain.EmbyUserPolicy `json:"policy"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	taskID, err := c.service.UpdateUser(id, ctx.Param("user_id"), req.Name, req.Policy)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// SetUserPassword 修改或重置密码。
func (c *EmbyManagementController) SetUserPassword(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
		Reset    bool   `json:"reset"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	taskID, err := c.service.SetUserPassword(id, ctx.Param("user_id"), req.Password, req.Reset)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// DeleteUser 删除用户。
func (c *EmbyManagementController) DeleteUser(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	taskID, err := c.service.DeleteUser(id, ctx.Param("user_id"))
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// UploadUserAvatar 上传用户头像。
func (c *EmbyManagementController) UploadUserAvatar(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	data, contentType, err := readUploadedImage(ctx)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	taskID, err := c.service.UploadUserAvatar(id, ctx.Param("user_id"), contentType, data)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// GetUserAvatar 代理读取用户头像，不向浏览器暴露 Emby API Key。
func (c *EmbyManagementController) GetUserAvatar(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	data, contentType, err := c.service.GetUserAvatar(id, ctx.Param("user_id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadGateway, err.Error())
		return
	}
	ctx.Data(http.StatusOK, contentType, data)
}

// ListUserLibraries 返回权限 Guid 和媒体库名称，供用户访问范围选择。
func (c *EmbyManagementController) ListUserLibraries(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	libraries, err := c.service.ListUserLibraries(id)
	if err != nil {
		ErrorResp(ctx, 502, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": libraries, "total": len(libraries)})
}

// ListLibraries 查询媒体库。
func (c *EmbyManagementController) ListLibraries(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	libraries, err := c.service.ListLibrarySummaries(id)
	if err != nil {
		ErrorResp(ctx, 502, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": libraries, "total": len(libraries)})
}

// GetLibraryCover 代理读取媒体库封面，不向浏览器暴露 Emby API Key。
func (c *EmbyManagementController) GetLibraryCover(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	data, contentType, err := c.service.GetLibraryCover(id, ctx.Param("library_id"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadGateway, err.Error())
		return
	}
	ctx.Data(http.StatusOK, contentType, data)
}

// CreateLibrary 新增媒体库。
func (c *EmbyManagementController) CreateLibrary(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req domain.EmbyLibraryInput
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	taskID, err := c.service.CreateLibrary(id, req)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// UpdateLibrary 修改媒体库。
func (c *EmbyManagementController) UpdateLibrary(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		OriginalName string `json:"original_name"`
		domain.EmbyLibraryInput
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	taskID, err := c.service.UpdateLibrary(id, req.OriginalName, req.EmbyLibraryInput)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// DeleteLibrary 删除媒体库配置。
func (c *EmbyManagementController) DeleteLibrary(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	name := strings.TrimSpace(ctx.Query("name"))
	taskID, err := c.service.DeleteLibrary(id, name)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// RefreshLibrary 提交异步刷新任务。
func (c *EmbyManagementController) RefreshLibrary(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	taskID, err := c.service.StartRefresh(id, ctx.Param("library_id"))
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	InfoResp(ctx, http.StatusAccepted, "刷新任务已提交", gin.H{"task_id": taskID})
}

// RefreshAllLibraries 提交全部媒体库刷新任务。
func (c *EmbyManagementController) RefreshAllLibraries(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	taskID, err := c.service.StartRefresh(id, "")
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	InfoResp(ctx, http.StatusAccepted, "刷新任务已提交", gin.H{"task_id": taskID})
}

// UploadLibraryCover 上传媒体库封面。
func (c *EmbyManagementController) UploadLibraryCover(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	data, contentType, err := readUploadedImage(ctx)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	taskID, err := c.service.UploadLibraryCover(id, ctx.Param("library_id"), contentType, data)
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID})
}

// GenerateLibraryCover 生成拼图或 AI 封面预览。
func (c *EmbyManagementController) GenerateLibraryCover(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		Mode        string `json:"mode"`
		LibraryName string `json:"library_name"`
		Description string `json:"description"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	var taskID string
	var err error
	if req.Mode == "ai" {
		taskID, err = c.service.GenerateAICover(id, ctx.Param("library_id"), req.LibraryName, req.Description)
	} else {
		taskID, err = c.service.GenerateCollageCover(id, ctx.Param("library_id"), req.LibraryName)
	}
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	InfoResp(ctx, http.StatusAccepted, "封面生成任务已提交", gin.H{"task_id": taskID})
}

// ApplyLibraryCover 应用生成任务的预览。
func (c *EmbyManagementController) ApplyLibraryCover(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	if err := c.service.ApplyGeneratedCover(id, ctx.Param("library_id"), req.TaskID); err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"task_id": req.TaskID})
}

// GetCoverPreview 返回任务生成的预览图片。
func (c *EmbyManagementController) GetCoverPreview(ctx *gin.Context) {
	data, contentType, err := c.service.GetCoverPreview(ctx.Param("task_id"))
	if err != nil {
		ErrorResp(ctx, 404, err.Error())
		return
	}
	ctx.Data(http.StatusOK, contentType, data)
}

// GetPluginStatus 检测神医助手。
func (c *EmbyManagementController) GetPluginStatus(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	status, err := c.service.GetStrmAssistantStatus(id)
	if err != nil {
		ErrorResp(ctx, 502, err.Error())
		return
	}
	SuccessResp(ctx, status)
}

// RunPluginTask 触发神医助手任务。
func (c *EmbyManagementController) RunPluginTask(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req struct {
		Action        string `json:"action"`
		LibraryID     string `json:"library_id"`
		AutoConfigure bool   `json:"auto_configure"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	if req.Action == "strm_scan_capture" && !req.AutoConfigure {
		ErrorResp(ctx, 400, "请确认自动启用 Image Capture 并更新神医助手 Library Scope")
		return
	}
	taskID, err := c.service.StartStrmAssistantTaskWithOptions(id, req.Action, req.LibraryID, req.AutoConfigure)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	InfoResp(ctx, http.StatusAccepted, "神医助手任务已提交", gin.H{"task_id": taskID})
}

// GetAIConfig 获取脱敏配置。
func (c *EmbyManagementController) GetAIConfig(ctx *gin.Context) {
	SuccessResp(ctx, c.service.GetAIConfig())
}

// SaveAIConfig 保存 AI 封面配置。
func (c *EmbyManagementController) SaveAIConfig(ctx *gin.Context) {
	var req struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	if err := c.service.SaveAIConfig(req.Provider, req.BaseURL, req.APIKey, req.Model); err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, c.service.GetAIConfig())
}

// BindMediaSource 绑定媒体源到当前 Emby 实例和媒体库。
func (c *EmbyManagementController) BindMediaSource(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	sourceID, err := strconv.Atoi(ctx.Param("source_id"))
	if err != nil || sourceID <= 0 {
		ErrorResp(ctx, 400, "无效的媒体源 ID")
		return
	}
	var req struct {
		LibraryID string `json:"library_id"`
	}
	if err = ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "请求参数错误")
		return
	}
	taskID, err := c.service.BindMediaSource(id, sourceID, req.LibraryID)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"message": "媒体源绑定已更新", "task_id": taskID})
}

func readUploadedImage(ctx *gin.Context) ([]byte, string, error) {
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	if header.Size > 10<<20 {
		return nil, "", service.ErrImageTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		return nil, "", err
	}
	contentType := http.DetectContentType(data)
	return data, contentType, nil
}

func respondTaskError(ctx *gin.Context, taskID string, err error) {
	status := http.StatusBadRequest
	if taskID != "" {
		status = http.StatusBadGateway
	}
	ctx.JSON(status, gin.H{"error": err.Error(), "task_id": taskID})
}
