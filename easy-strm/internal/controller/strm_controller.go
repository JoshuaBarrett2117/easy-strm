package controller

import (
	"fmt"
	"net/http"
	"time"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StrmConfigDetail STRM配置详情（用于回调注入时传递配置数据）
type StrmConfigDetail struct {
	ID               int
	Cloud115Id       int
	NetDiskPath      string
	LocalPath        string
	Cron             string
	Extension        string
	SyncMode         string
	SourceAccount    int
	TargetAccount    int
	TargetDirectory  string
	AutoCleanup      bool
	CleanupThreshold int
	CleanupPolicy    string
	MaxConcurrency   int
	CreateTime       time.Time
	UpdateTime       time.Time
}

// Cloud115AccountBrief 115账号简要信息（用于回调注入时传递账号数据）
type Cloud115AccountBrief struct {
	ID                int
	Name              string
	Cookie            string
	RefreshToken      string
	AccessToken       string
	ExpiresIn         int
	TransferAccountID int
	TransferDirectory string
	AccountType       string
	Priority          int
	Status            string
	TransferMethod    string
	AlistUrl          string
	AlistToken        string
	CreateTime        time.Time
	UpdateTime        time.Time
}

type StrmController struct {
	strmService *service.StrmService

	// --- 回调依赖：main 包全局函数通过依赖注入解耦 ---
	// NOTE: 以下函数属于 main 包，无法在 controller 层直接引用，
	// 因此通过回调模式注入，与 CronController/LogController 保持一致

	// getAllStrmConfig: 获取所有 STRM 配置（main.GetAllStrmConfig）
	getAllStrmConfig func(sortField, sortOrder string) ([]*StrmConfigDetail, error)
	// getStrmConfigByID: 根据 ID 获取 STRM 配置（main.GetStrmConfigByID）
	getStrmConfigByID func(id int) (*StrmConfigDetail, error)
	// createStrmConfig: 创建 STRM 配置（main.CreateStrmConfig）
	createStrmConfig func(cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfigDetail, error)
	// updateStrmConfig: 更新 STRM 配置（main.UpdateStrmConfig）
	updateStrmConfig func(id, cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfigDetail, error)
	// deleteStrmConfig: 删除 STRM 配置（main.DeleteStrmConfig）
	deleteStrmConfig func(id int) error
	// getCloud115ByID: 根据 ID 获取 115 账号（main.GetCloud115ByID）
	getCloud115ByID func(id int) (*Cloud115AccountBrief, error)
	// createTask: 创建异步任务（main.CreateTask）
	createTask func(taskID, taskType, taskName string) error
	// updateTaskStatus: 更新任务状态（main.UpdateTaskStatus）
	updateTaskStatus func(taskID, status string) error
	// setTaskError: 设置任务错误（main.SetTaskError）
	setTaskError func(taskID, errMsg string) error
	// runFullStrmGenerate: 执行全量 STRM 生成（main.RunFullStrmGenerate）
	// 参数使用 interface{} 以避免 main 包类型依赖
	runFullStrmGenerate func(strmConfig interface{}, cloud115 interface{}, taskID string) (interface{}, error)
	// getTaskByID: 根据 ID 获取任务（main.GetTask）
	getTaskByID func(taskID string) (interface{}, error)
	// serverURL: 服务器 URL（来自 config.ServerURL）
	serverURL string
}

func NewStrmController(strmService *service.StrmService) *StrmController {
	return &StrmController{
		strmService: strmService,
	}
}

func (c *StrmController) GetService() *service.StrmService {
	return c.strmService
}

// --- 回调注入方法 ---

func (c *StrmController) SetGetAllStrmConfig(fn func(sortField, sortOrder string) ([]*StrmConfigDetail, error)) {
	c.getAllStrmConfig = fn
}

func (c *StrmController) SetGetStrmConfigByID(fn func(id int) (*StrmConfigDetail, error)) {
	c.getStrmConfigByID = fn
}

func (c *StrmController) SetCreateStrmConfig(fn func(cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfigDetail, error)) {
	c.createStrmConfig = fn
}

func (c *StrmController) SetUpdateStrmConfig(fn func(id, cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*StrmConfigDetail, error)) {
	c.updateStrmConfig = fn
}

func (c *StrmController) SetDeleteStrmConfig(fn func(id int) error) {
	c.deleteStrmConfig = fn
}

func (c *StrmController) SetGetCloud115ByID(fn func(id int) (*Cloud115AccountBrief, error)) {
	c.getCloud115ByID = fn
}

func (c *StrmController) SetCreateTask(fn func(taskID, taskType, taskName string) error) {
	c.createTask = fn
}

func (c *StrmController) SetUpdateTaskStatus(fn func(taskID, status string) error) {
	c.updateTaskStatus = fn
}

func (c *StrmController) SetSetTaskError(fn func(taskID, errMsg string) error) {
	c.setTaskError = fn
}

func (c *StrmController) SetRunFullStrmGenerate(fn func(strmConfig interface{}, cloud115 interface{}, taskID string) (interface{}, error)) {
	c.runFullStrmGenerate = fn
}

func (c *StrmController) SetGetTaskByID(fn func(taskID string) (interface{}, error)) {
	c.getTaskByID = fn
}

func (c *StrmController) SetServerURL(url string) {
	c.serverURL = url
}

// --- 路由处理器方法 ---

// formatStrmConfig 格式化 STRM 配置为 API 响应格式
// 保持与原 auth.go 内联处理器一致的响应字段
func formatStrmConfig(cfg *StrmConfigDetail, full bool) map[string]interface{} {
	result := map[string]interface{}{
		"id":            cfg.ID,
		"cloud115_id":   cfg.Cloud115Id,
		"net_disk_path": cfg.NetDiskPath,
		"local_path":    cfg.LocalPath,
		"cron":          cfg.Cron,
		"extension":     cfg.Extension,
		"create_time":   cfg.CreateTime.Format("2006-01-02 15:04:05"),
		"update_time":   cfg.UpdateTime.Format("2006-01-02 15:04:05"),
	}
	// full=true 时返回所有字段（用于详情/创建/更新响应）
	if full {
		result["sync_mode"] = cfg.SyncMode
		result["source_account"] = cfg.SourceAccount
		result["target_account"] = cfg.TargetAccount
		result["target_directory"] = cfg.TargetDirectory
		result["auto_cleanup"] = cfg.AutoCleanup
		result["cleanup_threshold"] = cfg.CleanupThreshold
		result["cleanup_policy"] = cfg.CleanupPolicy
		result["max_concurrency"] = cfg.MaxConcurrency
	}
	return result
}

// GetConfigList 获取所有STRM配置
// Route: GET /strm/config
func (c *StrmController) GetConfigList(ctx *gin.Context) {
	logger.Debug("StrmController[GetConfigList] 获取STRM配置列表")

	sortField := ctx.DefaultQuery("sort_field", "id")
	sortOrder := ctx.DefaultQuery("sort_order", "asc")

	if c.getAllStrmConfig == nil {
		logger.Error("StrmController[GetConfigList] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	list, err := c.getAllStrmConfig(sortField, sortOrder)
	if err != nil {
		logger.Errorf("StrmController[GetConfigList] 获取配置列表失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 格式化返回数据的时间字段，保持与原 auth.go 内联处理器一致的响应格式
	formattedList := make([]map[string]interface{}, len(list))
	for i, cfg := range list {
		formattedList[i] = formatStrmConfig(cfg, false)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": formattedList,
	})
}

// GetConfigByID 根据ID获取STRM配置
// Route: GET /strm/config/:id
func (c *StrmController) GetConfigByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("StrmController[GetConfigByID] 获取配置详情, ID: %d", id)

	if c.getStrmConfigByID == nil {
		logger.Error("StrmController[GetConfigByID] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	cfg, err := c.getStrmConfigByID(id)
	if err != nil || cfg == nil {
		logger.Errorf("StrmController[GetConfigByID] 获取配置详情失败: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "STRM config not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": formatStrmConfig(cfg, true),
	})
}

// CreateConfig 创建STRM配置
// Route: POST /strm/config
func (c *StrmController) CreateConfig(ctx *gin.Context) {
	logger.Debug("StrmController[CreateConfig] 创建STRM配置")

	var cfgData struct {
		Cloud115Id       int    `json:"cloud115_id" binding:"required"`
		NetDiskPath      string `json:"net_disk_path" binding:"required"`
		LocalPath        string `json:"local_path" binding:"required"`
		Cron             string `json:"cron"`
		Extension        string `json:"extension"`
		SyncMode         string `json:"sync_mode"`
		SourceAccount    int    `json:"source_account"`
		TargetAccount    int    `json:"target_account"`
		TargetDirectory  string `json:"target_directory"`
		AutoCleanup      bool   `json:"auto_cleanup"`
		CleanupThreshold int    `json:"cleanup_threshold"`
		CleanupPolicy    string `json:"cleanup_policy"`
		MaxConcurrency   int    `json:"max_concurrency"`
	}

	if err := ctx.ShouldBindJSON(&cfgData); err != nil {
		logger.Warnf("StrmController[CreateConfig] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if c.createStrmConfig == nil {
		logger.Error("StrmController[CreateConfig] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	cfg, err := c.createStrmConfig(
		cfgData.Cloud115Id, cfgData.NetDiskPath, cfgData.LocalPath,
		cfgData.Cron, cfgData.Extension, cfgData.SyncMode,
		cfgData.SourceAccount, cfgData.TargetAccount, cfgData.TargetDirectory,
		cfgData.AutoCleanup, cfgData.CleanupThreshold, cfgData.CleanupPolicy,
		cfgData.MaxConcurrency,
	)
	if err != nil {
		logger.Errorf("StrmController[CreateConfig] 创建配置失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message": "STRM config created successfully",
		"data":    formatStrmConfig(cfg, true),
	})
}

// UpdateConfig 更新STRM配置
// Route: PUT /strm/config/:id
func (c *StrmController) UpdateConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("StrmController[UpdateConfig] 更新STRM配置, ID: %d", id)

	var cfgData struct {
		Cloud115Id       int    `json:"cloud115_id" binding:"required"`
		NetDiskPath      string `json:"net_disk_path" binding:"required"`
		LocalPath        string `json:"local_path" binding:"required"`
		Cron             string `json:"cron"`
		Extension        string `json:"extension"`
		SyncMode         string `json:"sync_mode"`
		SourceAccount    int    `json:"source_account"`
		TargetAccount    int    `json:"target_account"`
		TargetDirectory  string `json:"target_directory"`
		AutoCleanup      bool   `json:"auto_cleanup"`
		CleanupThreshold int    `json:"cleanup_threshold"`
		CleanupPolicy    string `json:"cleanup_policy"`
		MaxConcurrency   int    `json:"max_concurrency"`
	}

	if err := ctx.ShouldBindJSON(&cfgData); err != nil {
		logger.Warnf("StrmController[UpdateConfig] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if c.updateStrmConfig == nil {
		logger.Error("StrmController[UpdateConfig] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	cfg, err := c.updateStrmConfig(
		id, cfgData.Cloud115Id, cfgData.NetDiskPath, cfgData.LocalPath,
		cfgData.Cron, cfgData.Extension, cfgData.SyncMode,
		cfgData.SourceAccount, cfgData.TargetAccount, cfgData.TargetDirectory,
		cfgData.AutoCleanup, cfgData.CleanupThreshold, cfgData.CleanupPolicy,
		cfgData.MaxConcurrency,
	)
	if err != nil {
		logger.Errorf("StrmController[UpdateConfig] 更新配置失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "STRM config updated successfully",
		"data":    formatStrmConfig(cfg, false),
	})
}

// DeleteConfig 删除STRM配置
// Route: DELETE /strm/config/:id
func (c *StrmController) DeleteConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("StrmController[DeleteConfig] 删除STRM配置, ID: %d", id)

	if c.deleteStrmConfig == nil {
		logger.Error("StrmController[DeleteConfig] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if err := c.deleteStrmConfig(id); err != nil {
		logger.Errorf("StrmController[DeleteConfig] 删除配置失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "STRM config deleted successfully",
	})
}

// GenerateFull 全量生成STRM文件（异步）
// Route: POST /strm/config/:id/generate/full
func (c *StrmController) GenerateFull(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("StrmController[GenerateFull] 全量生成STRM, ID: %d", id)

	if c.getStrmConfigByID == nil || c.getCloud115ByID == nil ||
		c.createTask == nil || c.updateTaskStatus == nil || c.setTaskError == nil {
		logger.Error("StrmController[GenerateFull] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// 获取STRM配置
	strmConfig, err := c.getStrmConfigByID(id)
	if err != nil {
		logger.Errorf("StrmController[GenerateFull] 获取配置失败 ID %d: %v", id, err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "STRM config not found"})
		return
	}
	logger.Infof("StrmController[GenerateFull] 找到配置 ID %d: cloud115_id=%d, net_disk_path=%s, local_path=%s",
		strmConfig.ID, strmConfig.Cloud115Id, strmConfig.NetDiskPath, strmConfig.LocalPath)

	// 获取115云账号
	cloud115, err := c.getCloud115ByID(strmConfig.Cloud115Id)
	if err != nil {
		logger.Errorf("StrmController[GenerateFull] 获取115账号失败 ID %d: %v", strmConfig.Cloud115Id, err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Cloud115 account not found"})
		return
	}
	logger.Infof("StrmController[GenerateFull] 使用115账号 ID %d: %s", cloud115.ID, cloud115.Name)

	// 创建任务ID和任务名称
	taskID := uuid.New().String()
	taskName := fmt.Sprintf("STRM生成 - %s", strmConfig.NetDiskPath)
	err = c.createTask(taskID, "strm_generate", taskName)
	if err != nil {
		logger.Errorf("StrmController[GenerateFull] 创建任务失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	// 复制必要的配置数据，避免在goroutine中访问可能被修改的数据
	capturedConfigID := id
	capturedTaskID := taskID

	// 通过回调执行异步生成
	if c.runFullStrmGenerate != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("StrmController[GenerateFull] 异步生成panic: %v", r)
					c.setTaskError(capturedTaskID, fmt.Sprintf("Internal error: %v", r))
				}
			}()

			logger.Infof("StrmController[GenerateFull] 启动异步生成任务 %s, 配置 ID %d", capturedTaskID, capturedConfigID)
			c.updateTaskStatus(capturedTaskID, "running")

			_, err := c.runFullStrmGenerate(strmConfig, cloud115, capturedTaskID)
			if err != nil {
				logger.Errorf("StrmController[GenerateFull] 全量生成失败: %v", err)
				c.setTaskError(capturedTaskID, err.Error())
				return
			}

			c.updateTaskStatus(capturedTaskID, "completed")
			logger.Infof("StrmController[GenerateFull] 全量生成完成, 配置 ID %d, 任务 %s", capturedConfigID, capturedTaskID)
		}()
	}

	// 立即返回任务ID
	ctx.JSON(http.StatusOK, gin.H{
		"message":   "STRM generation task started",
		"task_id":   taskID,
		"config_id": capturedConfigID,
	})
}

// GenerateIncremental 增量生成STRM文件
// Route: POST /strm/config/:id/generate/incremental
func (c *StrmController) GenerateIncremental(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	logger.Debugf("StrmController[GenerateIncremental] 增量生成STRM, ID: %d", id)

	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error":   "增量 STRM 生成尚未接入真实执行链路",
		"message": "请使用全量生成，避免接口返回与实际执行结果不一致",
	})
}

// GetTaskStatus 查询STRM生成任务状态
// Route: GET /strm/task/:task_id
func (c *StrmController) GetTaskStatus(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	logger.Debugf("StrmController[GetTaskStatus] 查询任务状态, task_id: %s", taskID)

	if c.getTaskByID == nil {
		logger.Error("StrmController[GetTaskStatus] 回调依赖未注入")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	task, err := c.getTaskByID(taskID)
	if err != nil {
		logger.Errorf("StrmController[GetTaskStatus] 获取任务失败 %s: %v", taskID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get task status"})
		return
	}

	if task == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": task,
	})
}

// FindMatchingConfig 查找与媒体源匹配的 STRM 配置
func (c *StrmController) FindMatchingConfig(cloud115ID int, targetPath string) (int, error) {
	cfg, err := c.strmService.FindMatchingConfigByCloud115ID(cloud115ID, targetPath)
	if err != nil {
		return 0, err
	}
	if cfg == nil {
		return 0, nil
	}
	return cfg.ID, nil
}

// UpdateNetDiskPath 更新 STRM 配置的网盘路径
func (c *StrmController) UpdateNetDiskPath(configID int, netDiskPath string) error {
	return c.strmService.UpdateNetDiskPath(configID, netDiskPath)
}
