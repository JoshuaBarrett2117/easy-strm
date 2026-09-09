package controller

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
)

// Library 返回作品级海报墙。
func (c *ShareRecordController) Library(x *gin.Context) {
	var q domain.ShareLibraryQuery
	if err := x.ShouldBindQuery(&q); err != nil {
		ErrorResp(x, 400, "查询参数格式无效")
		return
	}
	if err := service.ValidateLibraryQuery(&q); err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	v, err := c.s.Library(x, q)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// LibrarySources 返回作品来源分页。
func (c *ShareRecordController) LibrarySources(x *gin.Context) {
	var q domain.ShareLibraryQuery
	if x.ShouldBindQuery(&q) != nil || service.ValidateLibraryQuery(&q) != nil || x.Query("work_key") == "" {
		ErrorResp(x, 400, "作品或分页参数无效")
		return
	}
	v, err := c.s.LibrarySources(x, x.Query("work_key"), q.Page, q.PageSize)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// LibraryOptions 返回筛选选项。
func (c *ShareRecordController) LibraryOptions(x *gin.Context) {
	v, err := c.s.LibraryOptions(x)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// EnrichLibrary 创建可取消的历史补全任务。
func (c *ShareRecordController) EnrichLibrary(x *gin.Context) {
	id, err := c.s.StartLibraryEnrichment()
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": id})
}
