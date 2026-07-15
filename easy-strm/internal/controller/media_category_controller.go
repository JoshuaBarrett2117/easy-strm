package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"
)

type MediaCategoryController struct {
	mediaCategoryService *service.MediaCategoryService
}

func NewMediaCategoryController(mediaCategoryService *service.MediaCategoryService) *MediaCategoryController {
	return &MediaCategoryController{
		mediaCategoryService: mediaCategoryService,
	}
}

func (c *MediaCategoryController) GetAll(ctx *gin.Context) {
	categories, err := c.mediaCategoryService.GetAll()
	if err != nil {
		logger.Errorf("MediaCategoryController[GetAll] err: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取媒体分类失败")
		return
	}
	SuccessResp(ctx, gin.H{"data": categories, "total": len(categories)})
}

func (c *MediaCategoryController) Create(ctx *gin.Context) {
	var cat domain.MediaCategory
	if err := ctx.ShouldBindJSON(&cat); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "参数错误")
		return
	}
	if err := c.mediaCategoryService.Create(&cat); err != nil {
		logger.Errorf("MediaCategoryController[Create] err: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "创建媒体分类失败")
		return
	}
	SuccessResp(ctx, cat)
}

func (c *MediaCategoryController) Update(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	var cat domain.MediaCategory
	if err := ctx.ShouldBindJSON(&cat); err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "参数错误")
		return
	}
	cat.ID = id
	if err := c.mediaCategoryService.Update(&cat); err != nil {
		logger.Errorf("MediaCategoryController[Update] err: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "更新媒体分类失败")
		return
	}
	SuccessResp(ctx, cat)
}

func (c *MediaCategoryController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		ErrorResp(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	if err := c.mediaCategoryService.Delete(id); err != nil {
		logger.Errorf("MediaCategoryController[Delete] err: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "删除媒体分类失败")
		return
	}
	SuccessResp(ctx, nil)
}
