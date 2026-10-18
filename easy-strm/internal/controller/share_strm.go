package controller

import (
	"context"
	"database/sql"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"encoding/json"
	"errors"
	"io"
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
	if modes, present := x.Request.URL.Query()["mode"]; present {
		if len(modes) != 1 || modes[0] != "work_reexport" || len(x.Request.URL.Query()) != 1 {
			ErrorResp(x, 400, "作品重导仅接受 mode=work_reexport，不接受混合筛选")
			return
		}
		c.workReexport(x)
		return
	}
	if x.Request.ContentLength != 0 {
		ErrorResp(x, 400, "JSON 作品重导必须明确指定 mode=work_reexport")
		return
	}
	var q domain.ShareLibraryQuery
	if x.ShouldBindQuery(&q) != nil || service.ValidateLibraryQuery(&q) != nil {
		ErrorResp(x, 400, "筛选参数无效")
		return
	}
	var id string
	var err error
	if contextual, ok := c.s.(interface {
		StartExportContext(context.Context, domain.ShareLibraryQuery) (string, error)
	}); ok {
		id, err = contextual.StartExportContext(x.Request.Context(), q)
	} else {
		id, err = c.s.StartExport(q)
	}
	if err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": id})
}

func (controller *ShareStrmController) workReexport(ctx *gin.Context) {
	var request domain.ShareWorkReexportRequest
	decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 32*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(interface{})) != io.EOF {
		ErrorResp(ctx, 400, "重导参数仅接受一个严格 JSON 对象")
		return
	}
	request.ActorUserID = ctx.GetInt("userID")
	if request.ActorUserID <= 0 {
		ErrorResp(ctx, 401, "作品重导必须由认证用户触发")
		return
	}
	if err := service.ValidateShareWorkReexport(request); err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	action, ok := controller.s.(interface {
		StartWorkReexportContext(context.Context, domain.ShareWorkReexportRequest) (string, error)
	})
	if !ok {
		ErrorResp(ctx, 500, "作品重导任务服务未初始化")
		return
	}
	id, err := action.StartWorkReexportContext(ctx.Request.Context(), request)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"task_id": id, "mode": "work_reexport", "status": "submitted"})
}

// AdoptBaseline 只接受认证用户的显式人工确认，返回可轮询任务，不提供自动采纳入口。
func (controller *ShareStrmController) AdoptBaseline(ctx *gin.Context) {
	var request domain.ShareBaselineAdoptionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		ErrorResp(ctx, 400, "采纳参数格式无效")
		return
	}
	if decoder.Decode(new(interface{})) != io.EOF {
		ErrorResp(ctx, 400, "采纳参数仅接受一个 JSON 对象")
		return
	}
	request.ActorUserID = ctx.GetInt("userID")
	if request.ActorUserID <= 0 {
		ErrorResp(ctx, 401, "采纳必须由已认证用户确认")
		return
	}
	if err := service.ValidateShareBaselineAdoption(request); err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	action, ok := controller.s.(interface {
		StartBaselineAdoptionContext(context.Context, domain.ShareBaselineAdoptionRequest) (string, error)
	})
	if !ok {
		ErrorResp(ctx, 500, "采纳任务服务未初始化")
		return
	}
	id, err := action.StartBaselineAdoptionContext(ctx.Request.Context(), request)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"task_id": id, "manual_confirmation": "人工确认采纳", "status": "submitted"})
}

// Playback 保持播放器UA并返回302；错误不生成重定向。
func (c *ShareStrmController) Playback(x *gin.Context) {
	defer beginDirectLinkRequest(x, "share_strm_playback")()
	id, err := uuid.Parse(x.Param("id"))
	if err != nil {
		writeSharePlaybackError(x, 400, errors.New("播放标识无效"))
		return
	}
	link, err := c.s.Playback(x.Request.Context(), id.String(), x.Request.UserAgent())
	if errors.Is(err, sql.ErrNoRows) {
		writeSharePlaybackError(x, 404, errors.New("播放映射不存在，请重新导出"))
		return
	}
	if err != nil {
		writeSharePlaybackError(x, 502, err)
		return
	}
	x.Header("Cache-Control", "no-store")
	x.Redirect(http.StatusFound, link)
	if c.recordPlayback != nil && x.Writer.Status() == http.StatusFound {
		c.recordPlayback(id.String(), link, x.ClientIP(), x.Request.Method)
	}
}
