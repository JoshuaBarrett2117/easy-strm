package controller

import (
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

// PlaybackRecordController 提供需要登录的播放记录查询接口。
type PlaybackRecordController struct {
	service *service.PlaybackRecordService
}

// NewPlaybackRecordController 创建记录查询控制器。
func NewPlaybackRecordController(s *service.PlaybackRecordService) *PlaybackRecordController {
	return &PlaybackRecordController{service: s}
}

// List 分页读取调用记录，默认每页二十条，最多一百条。
func (c *PlaybackRecordController) List(ctx *gin.Context) {
	offset, e1 := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	limit, e2 := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	if e1 != nil || e2 != nil || offset < 0 || offset > 1000 || limit < 1 || limit > 100 {
		ErrorResp(ctx, 400, "分页参数无效")
		return
	}
	records, total, err := c.service.List(ctx.Request.Context(), offset, limit)
	if err != nil {
		ErrorResp(ctx, 500, "读取播放记录失败")
		return
	}
	SuccessResp(ctx, gin.H{"data": records, "total": total})
}
