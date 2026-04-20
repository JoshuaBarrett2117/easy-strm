package controller

import (
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

type TaskController struct {
	taskService           *service.TaskService
	getAllTasks           func() (interface{}, error)
	retryAutoOrganizeTask func(taskID string) error
}

func NewTaskController(taskService *service.TaskService) *TaskController {
	return &TaskController{
		taskService: taskService,
	}
}

func (c *TaskController) SetGetAllTasks(fn func() (interface{}, error)) {
	c.getAllTasks = fn
}

func (c *TaskController) SetRetryAutoOrganizeTask(fn func(taskID string) error) {
	c.retryAutoOrganizeTask = fn
}

func (c *TaskController) GetAll(ctx *gin.Context) {
	logger.Debugf("TaskController[GetAll] get all tasks from %s", ctx.ClientIP())

	if c.getAllTasks != nil {
		tasks, err := c.getAllTasks()
		if err != nil {
			logger.Errorf("TaskController[GetAll] failed to get tasks: %v", err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get tasks"})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"data": tasks})
		return
	}

	tasks, err := c.taskService.GetAll()
	if err != nil {
		logger.Errorf("TaskController[GetAll] failed to get tasks: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取任务列表失败")
		return
	}

	taskList := make([]map[string]interface{}, 0, len(tasks))
	for _, task := range tasks {
		taskList = append(taskList, task)
	}

	SuccessResp(ctx, gin.H{
		"data":  taskList,
		"total": len(taskList),
	})
}

func (c *TaskController) GetUnified(ctx *gin.Context) {
	logger.Debugf("TaskController[GetUnified] get unified tasks from %s", ctx.ClientIP())

	tasks, err := c.taskService.GetUnified()
	if err != nil {
		logger.Errorf("TaskController[GetUnified] failed to get unified tasks: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取任务列表失败")
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  tasks,
		"total": len(tasks),
	})
}

func (c *TaskController) Get(ctx *gin.Context) {
	taskID := ctx.Query("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "task_id is required")
		return
	}

	task, err := c.taskService.Get(taskID)
	if err != nil {
		logger.Errorf("TaskController[Get] failed to get task: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取任务失败")
		return
	}

	if task == nil {
		ErrorResp(ctx, http.StatusNotFound, "任务不存在")
		return
	}

	SuccessResp(ctx, task)
}

func (c *TaskController) Cancel(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "task_id is required")
		return
	}

	logger.Infof("TaskController[Cancel] cancel task: %s from %s", taskID, ctx.ClientIP())

	if err := c.taskService.Cancel(taskID); err != nil {
		logger.Errorf("TaskController[Cancel] failed to cancel task: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"message": "任务已取消",
		"task_id": taskID,
	})
}

func (c *TaskController) Resume(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "task_id is required")
		return
	}

	logger.Infof("TaskController[Resume] resume task: %s from %s", taskID, ctx.ClientIP())

	task, err := c.taskService.Get(taskID)
	if err != nil {
		logger.Errorf("TaskController[Resume] failed to load task: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取任务失败")
		return
	}
	if task == nil {
		ErrorResp(ctx, http.StatusNotFound, "任务不存在")
		return
	}

	taskType, _ := task["task_type"].(string)
	if taskType == "watch_auto_organize" && c.retryAutoOrganizeTask != nil {
		if err := c.retryAutoOrganizeTask(taskID); err != nil {
			logger.Errorf("TaskController[Resume] failed to retry auto organize task: %v", err)
			ErrorResp(ctx, http.StatusBadRequest, err.Error())
			return
		}

		SuccessResp(ctx, gin.H{
			"message": "任务已重试执行",
			"task_id": taskID,
		})
		return
	}

	if err := c.taskService.Resume(taskID); err != nil {
		logger.Errorf("TaskController[Resume] failed to resume task: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"message": "任务已恢复",
		"task_id": taskID,
	})
}

func (c *TaskController) Delete(ctx *gin.Context) {
	taskID := ctx.Query("task_id")
	if taskID == "" {
		ErrorResp(ctx, http.StatusBadRequest, "task_id is required")
		return
	}

	if err := c.taskService.Delete(taskID); err != nil {
		logger.Errorf("TaskController[Delete] failed to delete task: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "删除任务失败")
		return
	}

	logger.Infof("TaskController[Delete] delete task success: %s", taskID)
	SuccessResp(ctx, gin.H{
		"message": "删除成功",
	})
}
