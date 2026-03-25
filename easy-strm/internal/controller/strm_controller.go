package controller

import (
	"fmt"
	"net/http"

	"easy-strm/internal/service"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

type StrmController struct {
	strmService *service.StrmService
}

func NewStrmController(strmService *service.StrmService) *StrmController {
	return &StrmController{
		strmService: strmService,
	}
}

// GetConfigList 获取所有STRM配置
func (c *StrmController) GetConfigList(ctx *gin.Context) {
	sortField := ctx.DefaultQuery("sort_field", "id")
	sortOrder := ctx.DefaultQuery("sort_order", "asc")

	list, err := c.strmService.GetAllConfig(sortField, sortOrder)
	if err != nil {
		logger.Errorf("StrmController[GetConfigList] 获取配置列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取配置列表失败")
		return
	}

	formattedList := make([]map[string]interface{}, len(list))
	for i, cfg := range list {
		formattedList[i] = map[string]interface{}{
			"id":            cfg.ID,
			"cloud115_id":   cfg.Cloud115Id,
			"net_disk_path": cfg.NetDiskPath,
			"local_path":    cfg.LocalPath,
			"cron":          cfg.Cron,
			"extension":     cfg.Extension,
			"create_time":   cfg.CreateTime.Format("2006-01-02 15:04:05"),
			"update_time":   cfg.UpdateTime.Format("2006-01-02 15:04:05"),
		}
	}

	SuccessResp(ctx, gin.H{
		"data":  formattedList,
		"total": len(formattedList),
	})
}

// GetConfigByID 根据ID获取STRM配置
func (c *StrmController) GetConfigByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	cfg, err := c.strmService.GetConfigByID(id)
	if err != nil || cfg == nil {
		logger.Errorf("StrmController[GetConfigByID] 获取配置详情失败: %v", err)
		ErrorResp(ctx, http.StatusNotFound, "配置不存在")
		return
	}

	formatted := map[string]interface{}{
		"id":            cfg.ID,
		"cloud115_id":   cfg.Cloud115Id,
		"net_disk_path": cfg.NetDiskPath,
		"local_path":    cfg.LocalPath,
		"cron":          cfg.Cron,
		"extension":     cfg.Extension,
		"create_time":   cfg.CreateTime.Format("2006-01-02 15:04:05"),
		"update_time":   cfg.UpdateTime.Format("2006-01-02 15:04:05"),
	}

	SuccessResp(ctx, formatted)
}

// CreateConfig 创建STRM配置
func (c *StrmController) CreateConfig(ctx *gin.Context) {
	var cfgData struct {
		Cloud115Id  int    `json:"cloud115_id" binding:"required"`
		NetDiskPath string `json:"net_disk_path" binding:"required"`
		LocalPath   string `json:"local_path" binding:"required"`
		Cron        string `json:"cron"`
		Extension   string `json:"extension"`
	}

	if err := ctx.ShouldBindJSON(&cfgData); err != nil {
		logger.Warnf("StrmController[CreateConfig] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	cfg, err := c.strmService.CreateConfig(cfgData.Cloud115Id, cfgData.NetDiskPath, cfgData.LocalPath, cfgData.Cron, cfgData.Extension)
	if err != nil {
		logger.Errorf("StrmController[CreateConfig] 创建配置失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("StrmController[CreateConfig] 创建配置成功: ID %d", cfg.ID)
	SuccessResp(ctx, gin.H{
		"message": "创建成功",
		"data": map[string]interface{}{
			"id":            cfg.ID,
			"cloud115_id":   cfg.Cloud115Id,
			"net_disk_path": cfg.NetDiskPath,
			"local_path":    cfg.LocalPath,
			"cron":          cfg.Cron,
			"extension":     cfg.Extension,
			"create_time":   cfg.CreateTime.Format("2006-01-02 15:04:05"),
			"update_time":   cfg.UpdateTime.Format("2006-01-02 15:04:05"),
		},
	})
}

// UpdateConfig 更新STRM配置
func (c *StrmController) UpdateConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	var cfgData struct {
		Cloud115Id  int    `json:"cloud115_id" binding:"required"`
		NetDiskPath string `json:"net_disk_path" binding:"required"`
		LocalPath   string `json:"local_path" binding:"required"`
		Cron        string `json:"cron"`
		Extension   string `json:"extension"`
	}

	if err := ctx.ShouldBindJSON(&cfgData); err != nil {
		logger.Warnf("StrmController[UpdateConfig] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	cfg, err := c.strmService.UpdateConfig(id, cfgData.Cloud115Id, cfgData.NetDiskPath, cfgData.LocalPath, cfgData.Cron, cfgData.Extension)
	if err != nil {
		logger.Errorf("StrmController[UpdateConfig] 更新配置失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("StrmController[UpdateConfig] 更新配置成功: ID %d", id)
	SuccessResp(ctx, gin.H{
		"message": "更新成功",
		"data": map[string]interface{}{
			"id":            cfg.ID,
			"cloud115_id":   cfg.Cloud115Id,
			"net_disk_path": cfg.NetDiskPath,
			"local_path":    cfg.LocalPath,
			"cron":          cfg.Cron,
			"extension":     cfg.Extension,
			"create_time":   cfg.CreateTime.Format("2006-01-02 15:04:05"),
			"update_time":   cfg.UpdateTime.Format("2006-01-02 15:04:05"),
		},
	})
}

// DeleteConfig 删除STRM配置
func (c *StrmController) DeleteConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	if err := c.strmService.DeleteConfig(id); err != nil {
		logger.Errorf("StrmController[DeleteConfig] 删除配置失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	logger.Infof("StrmController[DeleteConfig] 删除配置成功: ID %d", id)
	SuccessResp(ctx, gin.H{
		"message": "删除成功",
	})
}

// GetFilesByConfigID 根据配置ID获取STRM文件记录
func (c *StrmController) GetFilesByConfigID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	files, err := c.strmService.GetFilesByConfigID(id)
	if err != nil {
		logger.Errorf("StrmController[GetFilesByConfigID] 获取文件列表失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, "获取文件列表失败")
		return
	}

	SuccessResp(ctx, gin.H{
		"data":  files,
		"total": len(files),
	})
}
