package controller

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type shareStrmActions interface {
	Settings() (domain.ShareStrmSettings, error)
	SaveSettings(domain.ShareStrmSettings) error
	StartExport(domain.ShareLibraryQuery) (string, error)
	Playback(context.Context, string, string) (string, error)
}

// ShareStrmController 提供配置、导出任务及媒体服务器播放入口。
type ShareStrmController struct {
	s              shareStrmActions
	recordPlayback func(string, string, string, string)
}

// NewShareStrmController 注入资料库STRM服务。
func NewShareStrmController(s *service.ShareStrmService) *ShareStrmController {
	return &ShareStrmController{s: s}
}

// SetRecordPlayback 注入分享 STRM 成功解析后的播放记录能力。
func (c *ShareStrmController) SetRecordPlayback(fn func(string, string, string, string)) {
	c.recordPlayback = fn
}

// Settings 返回导出配置。
func (c *ShareStrmController) Settings(x *gin.Context) {
	v, err := c.s.Settings()
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// SaveSettings 验证并保存导出和转存目标。
func (c *ShareStrmController) SaveSettings(x *gin.Context) {
	var v domain.ShareStrmSettings
	if x.ShouldBindJSON(&v) != nil {
		ErrorResp(x, 400, "配置格式无效")
		return
	}
	if err := c.s.SaveSettings(v); err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	SuccessResp(x, nil)
}

// Export 提交筛选范围内全部作品的异步导出任务。
func (c *ShareStrmController) Export(x *gin.Context) {
	var q domain.ShareLibraryQuery
	if x.ShouldBindQuery(&q) != nil || service.ValidateLibraryQuery(&q) != nil {
		ErrorResp(x, 400, "筛选参数无效")
		return
	}
	id, err := c.s.StartExport(q)
	if err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": id})
}

// Playback 保持播放器UA并返回302；错误不生成重定向。
func (c *ShareStrmController) Playback(x *gin.Context) {
	defer beginDirectLinkRequest(x, "share_strm_playback")()
	id, err := uuid.Parse(x.Param("id"))
	if err != nil {
		ErrorResp(x, 400, "播放标识无效")
		return
	}
	link, err := c.s.Playback(x.Request.Context(), id.String(), x.Request.UserAgent())
	if errors.Is(err, sql.ErrNoRows) {
		ErrorResp(x, 404, "播放映射不存在，请重新导出")
		return
	}
	if err != nil {
		ErrorResp(x, 502, err.Error())
		return
	}
	x.Header("Cache-Control", "no-store")
	x.Redirect(http.StatusFound, link)
	if c.recordPlayback != nil && x.Writer.Status() == http.StatusFound {
		c.recordPlayback(id.String(), link, x.ClientIP(), x.Request.Method)
	}
}
