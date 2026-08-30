package controller

import (
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

// GlobalAPIController 提供全局第三方 API 配置接口。
type GlobalAPIController struct{ service *service.GlobalAPIService }

func NewGlobalAPIController(s *service.GlobalAPIService) *GlobalAPIController {
	return &GlobalAPIController{service: s}
}
func (c *GlobalAPIController) GetConfig(ctx *gin.Context) {
	v, err := c.service.GetConfig()
	if err != nil {
		ErrorResp(ctx, 500, err.Error())
		return
	}
	SuccessResp(ctx, v)
}
func (c *GlobalAPIController) Update(ctx *gin.Context) {
	var req service.GlobalAPIConfigUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ErrorResp(ctx, 400, "参数错误")
		return
	}
	v, err := c.service.Update(req)
	if err != nil {
		ErrorResp(ctx, 400, err.Error())
		return
	}
	SuccessResp(ctx, v)
}
func (c *GlobalAPIController) Regenerate(ctx *gin.Context) {
	v, key, err := c.service.Regenerate()
	if err != nil {
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	SuccessResp(ctx, gin.H{"config": v, "api_key": key})
}
