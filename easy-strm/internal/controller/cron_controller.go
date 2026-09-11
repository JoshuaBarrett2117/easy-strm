package controller

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

// CronController 提供统一调度配置与执行历史接口。
type CronController struct {
	cronService *service.CronService
	scheduler   *service.CronService
	runTask     func(*domain.CronTask)
}

// NewCronController 创建统一调度控制器。
func NewCronController(s *service.CronService, _ *service.StrmService, _ *service.Cloud115Service) *CronController {
	return &CronController{cronService: s}
}

// SetScheduler 注入共享调度实例，供管理接口与后台任务保持同一调度状态。
func (c *CronController) SetScheduler(scheduler *service.CronService) {
	if scheduler == nil {
		return
	}
	c.scheduler = scheduler
	c.cronService = scheduler
}

// SetExecuteCronTaskFn 注入定时任务执行回调。
func (c *CronController) SetExecuteCronTaskFn(fn func(*domain.CronTask)) { c.runTask = fn }

// GetAll 返回分页配置；旧STRM调用不带page时仍返回完整数组。
func (c *CronController) GetAll(x *gin.Context) {
	list, e := c.cronService.GetAll()
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	filtered := []*domain.CronTask{}
	for _, t := range list {
		if x.Query("handler") != "" && t.Handler != x.Query("handler") {
			continue
		}
		if x.Query("status") != "" && t.Status != x.Query("status") {
			continue
		}
		if !strings.Contains(strings.ToLower(t.TaskName), strings.ToLower(x.Query("keyword"))) {
			continue
		}
		t.NextRunTime = c.cronService.GetNextRunTime(t.ID)
		filtered = append(filtered, t)
	}
	if x.Query("page") == "" {
		x.JSON(200, gin.H{"data": filtered})
		return
	}
	page, size, ok := cronPage(x)
	if !ok {
		return
	}
	total := len(filtered)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	SuccessResp(x, gin.H{"data": filtered[start:end], "total": total})
}
func cronPage(x *gin.Context) (int, int, bool) {
	p, e := strconv.Atoi(x.DefaultQuery("page", "1"))
	z, f := strconv.Atoi(x.DefaultQuery("page_size", "20"))
	if e != nil || f != nil || p < 1 || p > 1000000 || z < 1 || z > 100 {
		ErrorResp(x, 400, "分页参数无效")
		return 0, 0, false
	}
	return p, z, true
}
func cronID(x *gin.Context) (int, bool) {
	id, e := strconv.Atoi(x.Param("id"))
	if e != nil || id < 1 {
		ErrorResp(x, 400, "任务ID无效")
		return 0, false
	}
	return id, true
}

// Handlers 返回处理器与参数目录。
func (c *CronController) Handlers(x *gin.Context) { SuccessResp(x, c.cronService.Handlers()) }

// Create 创建通用任务。
func (c *CronController) Create(x *gin.Context) {
	var t domain.CronTask
	if x.ShouldBindJSON(&t) != nil {
		ErrorResp(x, 400, "请求格式无效")
		return
	}
	t.ID = 0
	t.TaskKey = ""
	t.Builtin = false
	if t.Handler == "" {
		t.Handler = "incremental_sync"
		t.TaskName = "STRM增量同步"
		t.Params = map[string]interface{}{"cloud115_id": t.Cloud115ID, "strm_config_id": t.StrmConfigID}
	}
	v, e := c.cronService.Save(&t)
	if e != nil {
		ErrorResp(x, 400, e.Error())
		return
	}
	SuccessResp(x, v)
}

// Update 更新通用配置，同时支持STRM页面的周期与启停编辑。
func (c *CronController) Update(x *gin.Context) {
	id, ok := cronID(x)
	if !ok {
		return
	}
	t, e := c.cronService.GetByID(id)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	if t == nil {
		ErrorResp(x, 404, "任务不存在")
		return
	}
	if x.ShouldBindJSON(t) != nil {
		ErrorResp(x, 400, "请求格式无效")
		return
	}
	t.ID = id
	v, e := c.cronService.Save(t)
	if e != nil {
		ErrorResp(x, 400, e.Error())
		return
	}
	SuccessResp(x, v)
}

// Delete 删除用户任务。
func (c *CronController) Delete(x *gin.Context) {
	id, ok := cronID(x)
	if !ok {
		return
	}
	if e := c.cronService.Delete(id); e != nil {
		ErrorResp(x, 400, e.Error())
		return
	}
	SuccessResp(x, nil)
}

// RunImmediately 返回实际执行ID。
func (c *CronController) RunImmediately(x *gin.Context) {
	id, ok := cronID(x)
	if !ok {
		return
	}
	taskID, e := c.cronService.Run(id, "manual")
	if e != nil {
		ErrorResp(x, 400, e.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": taskID})
}

// GetStatus 返回任务及下次执行时间。
func (c *CronController) GetStatus(x *gin.Context) {
	id, ok := cronID(x)
	if !ok {
		return
	}
	t, e := c.cronService.GetByID(id)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	if t == nil {
		ErrorResp(x, 404, "任务不存在")
		return
	}
	t.NextRunTime = c.cronService.GetNextRunTime(id)
	SuccessResp(x, t)
}

// Runs 返回持久化执行历史。
func (c *CronController) Runs(x *gin.Context) {
	id, ok := cronID(x)
	if !ok {
		return
	}
	p, z, ok := cronPage(x)
	if !ok {
		return
	}
	v, e := c.cronService.Runs(id, p, z)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, v)
}

// GetScheduledTasks 复用统一列表入口。
func (c *CronController) GetScheduledTasks(x *gin.Context) { c.GetAll(x) }
