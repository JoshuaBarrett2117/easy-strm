package controller

import (
	"net/http"

	"easy-strm/internal/service"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

type TaskController struct {
	taskService *service.TaskService
}

func NewTaskController(taskService *service.TaskService) *TaskController {
	return &TaskController{
		taskService: taskService,
	}
}

// GetAll 获取所有任务
func (c *TaskController) GetAll(ctx *gin.Context) {
	tasks, err := c.taskService.GetAll()
	if err != nil {
		logger.Errorf("TaskController[GetAll] 获取任务列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取任务列表失败")
		return
	}

	var taskList []map[string]interface{}
	for _, task := range tasks {
		taskList = append(taskList, task)
	}

	SuccessResp(ctx, gin.H{
		"data":  taskList,
		"total": len(taskList),
	})
}

// Get 获取单个任务
func (c *TaskController) Get(ctx *gin.Context) {
	taskID := ctx.Query("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "task_id is required")
		return
	}

	task, err := c.taskService.Get(taskID)
	if err != nil {
		logger.Errorf("TaskController[Get] 获取任务失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取任务失败")
		return
	}

	if task == nil {
		ErrorResp(ctx, http.StatusNotFound, "任务不存在")
		return
	}

	SuccessResp(ctx, task)
}

// Delete 删除任务
func (c *TaskController) Delete(ctx *gin.Context) {
	taskID := ctx.Query("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := c.taskService.Delete(taskID); err != nil {
		logger.Errorf("TaskController[Delete] 删除任务失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "删除任务失败")
		return
	}

	logger.Infof("TaskController[Delete] 删除任务成功: %s", taskID)
	SuccessResp(ctx, gin.H{
		"message": "删除成功",
	})
}
