package controller

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type ShareRecordController struct{ s *service.ShareRecordService }

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
	v, e := c.s.List(x, domain.ShareRecordQuery{Keyword: x.Query("keyword"), Page: p, PageSize: z})
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
	if e := c.s.Identify(x, m, false); e != nil {
		ErrorResp(x, http.StatusBadRequest, e.Error())
		return
	}
	SuccessResp(x, nil)
}
func (c *ShareRecordController) Batch(x *gin.Context) {
	var r struct {
		IDs   []int `json:"ids"`
		Retry bool  `json:"retry_failed"`
	}
	_ = x.ShouldBindJSON(&r)
	taskID, e := c.s.StartBatchIdentify(x, r.IDs, r.Retry)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": taskID, "message": "批量识别任务已创建"})
}

// IdentifyRecord 为单条分享创建后台识别任务。
func (c *ShareRecordController) IdentifyRecord(x *gin.Context) {
	id, _ := strconv.Atoi(x.Param("id"))
	taskID, e := c.s.StartRecordIdentify(x, id)
	if e != nil {
		ErrorResp(x, 500, e.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": taskID, "message": "单条分享识别任务已创建"})
}
