package controller

import (
	"fmt"
	"net/http"
	"time"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// CronController 定时任务控制器
// 负责定时任务的 CRUD、立即执行、状态查询等操作
type CronController struct {
	cronService     *service.CronService
	strmService     *service.StrmService
	cloud115Service *service.Cloud115Service
	// scheduler 接口：用于获取下次执行时间，与 service 层的 scheduler 接口解耦
	scheduler interface {
		GetNextRunTime(taskID int) *time.Time
	}
	// executeCronTaskFn: 执行定时任务的回调函数，由路由层注入
	// NOTE: 此函数引用了 main 包中的全局变量（client, config 等），无法直接迁移到 service 层，
	// 因此通过回调方式解耦
	executeCronTaskFn func(task *domain.CronTask)
}

// NewCronController 创建定时任务控制器实例
func NewCronController(cronService *service.CronService, strmService *service.StrmService, cloud115Service *service.Cloud115Service) *CronController {
	return &CronController{
		cronService:     cronService,
		strmService:     strmService,
		cloud115Service: cloud115Service,
	}
}

// SetScheduler 设置调度器实例
func (cc *CronController) SetScheduler(s interface {
	GetNextRunTime(taskID int) *time.Time
}) {
	cc.scheduler = s
}

// SetExecuteCronTaskFn 设置执行定时任务的回调
func (cc *CronController) SetExecuteCronTaskFn(fn func(task *domain.CronTask)) {
	cc.executeCronTaskFn = fn
}

// GetAll 获取所有 cron 定时任务
// Route: GET /cron/tasks
// 响应格式: {"data": [{task1}, {task2}, ...]}
func (cc *CronController) GetAll(ctx *gin.Context) {
	logger.Debug("CronController[GetAll] 获取所有定时任务")

	tasks, err := cc.cronService.GetAll()
	if err != nil {
		logger.Errorf("CronController[GetAll] 查询失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get cron tasks"})
		return
	}

	// 为每个任务添加下次执行时间
	result := make([]gin.H, 0, len(tasks))
	for _, task := range tasks {
		taskData := gin.H{
			"id":               task.ID,
			"task_name":        task.TaskName,
			"task_type":        task.TaskType,
			"cloud115_id":      task.Cloud115ID,
			"strm_config_id":   task.StrmConfigID,
			"cron_expr":        task.CronExpr,
			"status":           task.Status,
			"last_run_time":    task.LastRunTime,
			"next_run_time":    task.NextRunTime,
			"last_run_status":  task.LastRunStatus,
			"last_run_message": task.LastRunMessage,
			"create_time":      task.CreateTime,
			"update_time":      task.UpdateTime,
		}

		// 从调度器获取下次执行时间
		if cc.scheduler != nil {
			nextRun := cc.scheduler.GetNextRunTime(task.ID)
			if nextRun != nil {
				taskData["next_run_time"] = nextRun
			}
		}

		result = append(result, taskData)
	}

	ctx.JSON(http.StatusOK, gin.H{"data": result})
}

// Create 创建 cron 定时任务
// Route: POST /cron/task
func (cc *CronController) Create(ctx *gin.Context) {
	logger.Debug("CronController[Create] 创建定时任务")

	var req struct {
		Cloud115ID   int    `json:"cloud115_id" binding:"required"`
		StrmConfigID int    `json:"strm_config_id" binding:"required"`
		CronExpr     string `json:"cron_expr" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("CronController[Create] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// 验证 STRM 配置存在
	strmConfig, err := cc.strmService.GetConfigByID(req.StrmConfigID)
	if err != nil || strmConfig == nil {
		logger.Errorf("CronController[Create] STRM配置不存在: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "STRM config not found"})
		return
	}

	// 验证 STRM 配置属于该 115 账号
	if strmConfig.Cloud115Id != req.Cloud115ID {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "STRM config does not belong to this 115 account"})
		return
	}

	// 生成任务名称
	taskName := fmt.Sprintf("%d增量更新任务", req.Cloud115ID)

	// 检查任务是否已存在
	existingTask, _ := cc.cronService.GetByName(taskName)
	if existingTask != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Cron task already exists for this account"})
		return
	}

	// 创建定时任务
	task, err := cc.cronService.Create(taskName, "incremental_sync", req.Cloud115ID, req.StrmConfigID, req.CronExpr)
	if err != nil {
		logger.Errorf("CronController[Create] 创建失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create cron task"})
		return
	}

	logger.Infof("CronController[Create] 创建成功: %s (ID: %d)", taskName, task.ID)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Cron task created successfully",
		"data":    task,
	})
}

// Update 更新 cron 定时任务
// Route: PUT /cron/task/:id
func (cc *CronController) Update(ctx *gin.Context) {
	logger.Debug("CronController[Update] 更新定时任务")

	taskID := 0
	fmt.Sscanf(ctx.Param("id"), "%d", &taskID)
	if taskID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var req struct {
		CronExpr string `json:"cron_expr" binding:"required"`
		Status   string `json:"status"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Warnf("CronController[Update] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// 获取现有任务
	task, err := cc.cronService.GetByID(taskID)
	if err != nil || task == nil {
		logger.Errorf("CronController[Update] 任务不存在: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Cron task not found"})
		return
	}

	// 更新状态
	status := task.Status
	if req.Status != "" {
		status = req.Status
	}

	// 更新数据库和调度器
	task, err = cc.cronService.Update(taskID, task.TaskName, task.TaskType, req.CronExpr, status)
	if err != nil {
		logger.Errorf("CronController[Update] 更新失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update cron task"})
		return
	}

	logger.Infof("CronController[Update] 更新成功: ID %d", taskID)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Cron task updated successfully",
		"data":    task,
	})
}

// Delete 删除 cron 定时任务
// Route: DELETE /cron/task/:id
func (cc *CronController) Delete(ctx *gin.Context) {
	logger.Debug("CronController[Delete] 删除定时任务")

	taskID := 0
	fmt.Sscanf(ctx.Param("id"), "%d", &taskID)
	if taskID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	if err := cc.cronService.Delete(taskID); err != nil {
		logger.Errorf("CronController[Delete] 删除失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete cron task"})
		return
	}

	logger.Infof("CronController[Delete] 删除成功: ID %d", taskID)
	ctx.JSON(http.StatusOK, gin.H{"message": "Cron task deleted successfully"})
}

// RunImmediately 立即执行 cron 任务
// Route: POST /cron/task/:id/run
func (cc *CronController) RunImmediately(ctx *gin.Context) {
	logger.Debug("CronController[RunImmediately] 立即执行定时任务")

	taskID := 0
	fmt.Sscanf(ctx.Param("id"), "%d", &taskID)
	if taskID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取任务
	task, err := cc.cronService.GetByID(taskID)
	if err != nil || task == nil {
		logger.Errorf("CronController[RunImmediately] 任务不存在: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Cron task not found"})
		return
	}

	// 异步执行任务
	if cc.executeCronTaskFn != nil {
		go cc.executeCronTaskFn(task)
	}

	logger.Infof("CronController[RunImmediately] 已触发执行: ID %d", taskID)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Cron task triggered successfully",
		"task_id": taskID,
	})
}

// GetStatus 获取 cron 任务执行状态
// Route: GET /cron/task/:id/status
func (cc *CronController) GetStatus(ctx *gin.Context) {
	logger.Debug("CronController[GetStatus] 获取定时任务状态")

	taskID := 0
	fmt.Sscanf(ctx.Param("id"), "%d", &taskID)
	if taskID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	task, err := cc.cronService.GetByID(taskID)
	if err != nil || task == nil {
		logger.Errorf("CronController[GetStatus] 任务不存在: %v", err)
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Cron task not found"})
		return
	}

	// 从调度器获取下次执行时间
	nextRun := task.NextRunTime
	if cc.scheduler != nil {
		if nr := cc.scheduler.GetNextRunTime(taskID); nr != nil {
			nextRun = nr
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":               task.ID,
			"task_name":        task.TaskName,
			"status":           task.Status,
			"last_run_time":    task.LastRunTime,
			"next_run_time":    nextRun,
			"last_run_status":  task.LastRunStatus,
			"last_run_message": task.LastRunMessage,
		},
	})
}

// GetScheduledTasks 获取所有定时任务（兼容旧接口）
// Route: GET /scheduled-tasks
func (cc *CronController) GetScheduledTasks(ctx *gin.Context) {
	logger.Debug("CronController[GetScheduledTasks] 获取所有定时任务(兼容接口)")

	tasks, err := cc.cronService.GetAll()
	if err != nil {
		ctx.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": tasks})
}
