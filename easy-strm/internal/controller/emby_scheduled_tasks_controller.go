package controller

import "github.com/gin-gonic/gin"

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
