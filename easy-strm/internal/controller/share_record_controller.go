package controller

import (
	"database/sql"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type ShareRecordController struct{ s *service.ShareRecordService }

// GetTaskSettings 返回分享任务的总时限配置。
func (c *ShareRecordController) GetTaskSettings(x *gin.Context) {
	value, err := c.s.GetTaskSettings()
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, value)
}

// SaveTaskSettings 保存总时限，使用指针区分无限制0与缺少字段。
func (c *ShareRecordController) SaveTaskSettings(x *gin.Context) {
	var input struct {
		TimeoutMinutes *int `json:"timeout_minutes"`
	}
	if x.ShouldBindJSON(&input) != nil || input.TimeoutMinutes == nil {
		ErrorResp(x, 400, "请输入总时限，0表示无限制")
		return
	}
	value := service.ShareTaskSettings{TimeoutMinutes: *input.TimeoutMinutes}
	if err := c.s.SaveTaskSettings(value); err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	SuccessResp(x, value)
}

// ClearMedia 清空指定分享的媒体内容，返回实际删除条数。
func (c *ShareRecordController) ClearMedia(x *gin.Context) {
	id, err := strconv.Atoi(x.Param("id"))
	if err != nil || id <= 0 {
		ErrorResp(x, 400, "分享ID无效")
		return
	}
	count, err := c.s.ClearMedia(x, id)
	if errors.Is(err, sql.ErrNoRows) {
		ErrorResp(x, 404, "分享不存在")
		return
	}
	if err != nil {
		ErrorResp(x, 409, err.Error())
		return
	}
	SuccessResp(x, gin.H{"deleted": count})
}

// ClearAllMedia 清空全部分享的媒体内容，返回实际删除条数。
func (c *ShareRecordController) ClearAllMedia(x *gin.Context) {
	count, err := c.s.ClearAllMedia(x)
	if err != nil {
		ErrorResp(x, 409, err.Error())
		return
	}
	SuccessResp(x, gin.H{"deleted": count})
}

// ParseImport 返回批量分享预览，不创建分享或执行网盘操作。
func (c *ShareRecordController) ParseImport(x *gin.Context) {
	var input struct {
		Text string `json:"text" binding:"required"`
	}
	if x.ShouldBindJSON(&input) != nil {
		ErrorResp(x, 400, "请输入有效的分享内容")
		return
	}
	result, err := c.s.ParseImport(input.Text)
	if err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	SuccessResp(x, result)
}

// ManualIdentify 保存用户选定的媒体信息。
func (c *ShareRecordController) ManualIdentify(x *gin.Context) {
	var m domain.ShareMedia
	id, err := strconv.Atoi(x.Param("mediaId"))
	if err != nil || id <= 0 || x.ShouldBindJSON(&m) != nil {
		ErrorResp(x, 400, "请求格式无效")
		return
	}
	m.ID = id
	if err := c.s.ManualIdentify(x, m); err != nil {
		ErrorResp(x, 409, err.Error())
		return
	}
	SuccessResp(x, nil)
}

func NewShareRecordController(s *service.ShareRecordService) *ShareRecordController {
	return &ShareRecordController{s}
}
func (c *ShareRecordController) List(x *gin.Context) {
	p, _ := strconv.Atoi(x.DefaultQuery("page", "1"))
	z, _ := strconv.Atoi(x.DefaultQuery("page_size", "20"))
	shareID, _ := strconv.Atoi(x.Query("share_id"))
	v, e := c.s.List(x, domain.ShareRecordQuery{ShareID: shareID, Summary: true, Keyword: x.Query("keyword"), Page: p, PageSize: z})
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, v)
}
func (c *ShareRecordController) Create(x *gin.Context) {
	var r domain.ShareRecord
	if e := x.ShouldBindJSON(&r); e != nil {
		ErrorResp(x, 400, "请求格式无效")
		return
	}
	if e := c.s.Create(x, &r); e != nil {
		ErrorResp(x, 400, e.Error())
		return
	}
	SuccessResp(x, r)
}
func (c *ShareRecordController) Update(x *gin.Context) {
	var r domain.ShareRecord
	if x.ShouldBindJSON(&r) != nil {
		ErrorResp(x, 400, "请求无效")
		return
	}
	r.ID, _ = strconv.Atoi(x.Param("id"))
	if e := c.s.Update(x, &r); e != nil {
		ErrorResp(x, 409, e.Error())
		return
	}
	SuccessResp(x, r)
}
func (c *ShareRecordController) Delete(x *gin.Context) {
	id, _ := strconv.Atoi(x.Param("id"))
	if e := c.s.Delete(x, id); e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, nil)
}
func (c *ShareRecordController) AddMedia(x *gin.Context) {
	var m domain.ShareMedia
	_ = x.ShouldBindJSON(&m)
	m.ShareID, _ = strconv.Atoi(x.Param("id"))
	if e := c.s.AddMedia(x, &m); e != nil {
		ErrorResp(x, 400, e.Error())
		return
	}
	SuccessResp(x, m)
}
func (c *ShareRecordController) DeleteMedia(x *gin.Context) {
	id, _ := strconv.Atoi(x.Param("mediaId"))
	if e := c.s.DeleteMedia(x, id); e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, nil)
}
func (c *ShareRecordController) Identify(x *gin.Context) {
	id, _ := strconv.Atoi(x.Param("mediaId"))
	var m domain.ShareMedia
	_ = x.ShouldBindJSON(&m)
	m.ID = id
	if e := c.s.Identify(x, m, true); e != nil {
		ErrorResp(x, http.StatusBadRequest, e.Error())
		return
	}
	SuccessResp(x, nil)
}
func (c *ShareRecordController) Batch(x *gin.Context) {
	var r struct {
		IDs         []int `json:"ids"`
		Retry       bool  `json:"retry_failed"`
		PendingOnly bool  `json:"pending_only"`
	}
	if x.ShouldBindJSON(&r) != nil || (r.Retry && r.PendingOnly) {
		ErrorResp(x, 400, "识别任务参数无效")
		return
	}
	taskID, e := c.s.StartBatchIdentify(x, r.IDs, r.Retry, r.PendingOnly)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": taskID, "message": "批量识别任务已创建"})
}

// IdentifyRecord 为单条分享创建后台识别任务。
func (c *ShareRecordController) IdentifyRecord(x *gin.Context) {
	id, err := strconv.Atoi(x.Param("id"))
	pendingOnly, parseErr := strconv.ParseBool(x.DefaultQuery("pending_only", "false"))
	failedOnly, failedErr := strconv.ParseBool(x.DefaultQuery("failed_only", "false"))
	if err != nil || id < 1 || parseErr != nil || failedErr != nil || (pendingOnly && failedOnly) {
		ErrorResp(x, 400, "分享ID或识别参数无效")
		return
	}
	taskID, e := c.s.StartRecordIdentify(x, id, pendingOnly, failedOnly)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": taskID, "message": "单条分享识别任务已创建"})
}

// SyncRecord 为单条分享创建文件同步任务。
func (c *ShareRecordController) SyncRecord(x *gin.Context) {
	id, err := strconv.Atoi(x.Param("id"))
	if err != nil || id < 1 {
		ErrorResp(x, 400, "分享ID无效")
		return
	}
	taskID, err := c.s.StartRecordSync(x, id)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": taskID, "message": "分享文件同步任务已创建"})
}

// ListFiles 按页读取分享中的真实文件及识别状态。
func (c *ShareRecordController) ListFiles(x *gin.Context) {
	id, e1 := strconv.Atoi(x.Param("id"))
	page, e2 := strconv.Atoi(x.DefaultQuery("page", "1"))
	size, e3 := strconv.Atoi(x.DefaultQuery("page_size", "20"))
	if e1 != nil || e2 != nil || e3 != nil || id < 1 || page < 1 || size < 1 || size > 200 {
		ErrorResp(x, 400, "分页参数无效")
		return
	}
	result, err := c.s.ListFiles(x, id, page, size)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, result)
}

// ListMedia 按页读取已识别海报，默认每页十条。
func (c *ShareRecordController) ListMedia(x *gin.Context) {
	id, e1 := strconv.Atoi(x.Param("id"))
	page, e2 := strconv.Atoi(x.DefaultQuery("page", "1"))
	size, e3 := strconv.Atoi(x.DefaultQuery("page_size", "10"))
	duplicates, e4 := strconv.ParseBool(x.DefaultQuery("show_duplicates", "false"))
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || id < 1 || page < 1 || size < 1 || size > 100 || page > 10000000 {
		ErrorResp(x, 400, "分页参数无效")
		return
	}
	result, err := c.s.ListMedia(x, id, page, size, duplicates)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, result)
}
