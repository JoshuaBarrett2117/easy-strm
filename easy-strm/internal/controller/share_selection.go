package controller

import (
	"context"
	"database/sql"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"errors"

	"github.com/gin-gonic/gin"
)

type shareSelectionActions interface {
	List(context.Context, domain.ShareSelectionQuery) ([]domain.ShareSelection, int, error)
	Detail(context.Context, string) (domain.ShareSelectionDetail, error)
	Change(context.Context, domain.ShareSelectionChange) error
	Refresh(context.Context) error
}

// ShareSelectionController 提供鉴权组内集级来源选择管理。
type ShareSelectionController struct{ service shareSelectionActions }

// NewShareSelectionController 注入来源管理用例。
func NewShareSelectionController(service *service.ShareSelectionService) *ShareSelectionController {
	return &ShareSelectionController{service: service}
}

// List 返回稳定 data/total 分页结构。
func (c *ShareSelectionController) List(ctx *gin.Context) {
	var query domain.ShareSelectionQuery
	if ctx.ShouldBindQuery(&query) != nil {
		ErrorResp(ctx, 400, "分页参数无效")
		return
	}
	values, total, err := c.service.List(ctx.Request.Context(), query)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"data": values, "total": total})
}

// Detail 获取一条媒体季集候选状态。
func (c *ShareSelectionController) Detail(ctx *gin.Context) {
	value, err := c.service.Detail(ctx.Request.Context(), ctx.Query("media_item_key"))
	if err != nil {
		selectionError(ctx, err)
		return
	}
	SuccessResp(ctx, value)
}

// Change 以 CAS 改选，下次导出生效，不立即触碰输出。
func (c *ShareSelectionController) Change(ctx *gin.Context) {
	var change domain.ShareSelectionChange
	if ctx.ShouldBindJSON(&change) != nil {
		ErrorResp(ctx, 400, "候选或版本参数无效")
		return
	}
	if err := c.service.Change(ctx.Request.Context(), change); err != nil {
		selectionError(ctx, err)
		return
	}
	SuccessResp(ctx, gin.H{"next_export": true})
}

// Refresh 显式刷新候选，列表读取没有写副作用。
func (c *ShareSelectionController) Refresh(ctx *gin.Context) {
	if err := c.service.Refresh(ctx.Request.Context()); err != nil {
		selectionError(ctx, err)
		return
	}
	SuccessResp(ctx, nil)
}

func selectionError(ctx *gin.Context, err error) {
	status := 400
	if errors.Is(err, sql.ErrNoRows) {
		status = 404
	}
	if errors.Is(err, dao.ErrShareSelectionConflict) {
		status = 409
	}
	ErrorResp(ctx, status, err.Error())
}
