package controller

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// UpdateScheduledTaskTriggers 修改或删除 Emby 自动触发规则，不立即执行任务。
func (c *EmbyManagementController) UpdateScheduledTaskTriggers(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	var req domain.EmbyTaskTriggersRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "触发规则格式无效")
		return
	}
	if err := service.ValidateEmbyTaskTriggers(req.Triggers); err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	if err := c.service.UpdateScheduledTaskTriggers(id, ctx.Param("task_id"), req); err != nil {
		ErrorResp(ctx, 502, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"message": "触发规则已保存"})
}

// ListScheduledTasks 查询实例定时任务及最近执行结果。
func (c *EmbyManagementController) ListScheduledTasks(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	tasks, err := c.service.ListScheduledTasks(id)
	if err != nil {
		ErrorResp(ctx, 502, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": tasks, "total": len(tasks)})
}

// StartScheduledTask 立即触发指定 Emby 任务并返回触发记录 ID。
func (c *EmbyManagementController) StartScheduledTask(ctx *gin.Context) {
	id, ok := c.serverID(ctx)
	if !ok {
		return
	}
	taskID, err := c.service.StartScheduledTask(id, ctx.Param("task_id"))
	if err != nil {
		respondTaskError(ctx, taskID, err)
		return
	}
	SuccessResp(ctx, gin.H{"task_id": taskID, "message": "触发请求已提交，执行结果请查看定时任务列表"})
}
