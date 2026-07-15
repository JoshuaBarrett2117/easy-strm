package controller

import (
	"net/http"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// SettingsController 系统配置控制器
// 负责系统设置的 CRUD 操作，路由路径保持与 auth.go 内联版本完全一致
type SettingsController struct {
	systemConfigService *service.SystemConfigService
}

// NewSettingsController 创建系统配置控制器实例
func NewSettingsController(systemConfigService *service.SystemConfigService) *SettingsController {
	return &SettingsController{
		systemConfigService: systemConfigService,
	}
}

// GetAll 获取所有系统配置
// Route: GET /settings
// 响应格式: {"data": {"key1": "val1", "key2": "val2"}}
func (sc *SettingsController) GetAll(ctx *gin.Context) {
	logger.Debug("SettingsController[GetAll] 获取所有系统配置")

	configs, err := sc.systemConfigService.GetAll()
	if err != nil {
		logger.Errorf("SettingsController[GetAll] 查询失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 格式化为前端需要的 key->value 结构
	formattedConfigs := make(map[string]string)
	for _, config := range configs {
		formattedConfigs[config.ConfigKey] = config.ConfigVal
	}

	ctx.JSON(http.StatusOK, gin.H{"data": formattedConfigs})
}

// GetByKey 获取单个系统配置
// Route: GET /settings/:key
// 响应格式: {"data": {"key": "xxx", "value": "yyy"}}
func (sc *SettingsController) GetByKey(ctx *gin.Context) {
	key := ctx.Param("key")
	logger.Debugf("SettingsController[GetByKey] 获取配置, key: %s", key)

	config, err := sc.systemConfigService.GetByKey(key)
	if err != nil || config == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Setting not found: " + key})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"key":   config.ConfigKey,
			"value": config.ConfigVal,
		},
	})
}

// UpdateByKey 更新系统配置
// Route: PUT /settings/:key
// 响应格式: {"message": "Setting updated successfully", "data": {"key": "xxx", "value": "yyy"}}
func (sc *SettingsController) UpdateByKey(ctx *gin.Context) {
	key := ctx.Param("key")
	logger.Debugf("SettingsController[UpdateByKey] 更新配置, key: %s", key)

	var reqData struct {
		Value string `json:"value"`
	}
	if err := ctx.ShouldBindJSON(&reqData); err != nil {
		logger.Warnf("SettingsController[UpdateByKey] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := sc.systemConfigService.Upsert(key, reqData.Value); err != nil {
		logger.Errorf("SettingsController[UpdateByKey] 更新失败 %s: %v", key, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Infof("SettingsController[UpdateByKey] 更新成功: %s = %s", key, reqData.Value)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Setting updated successfully",
		"data": gin.H{
			"key":   key,
			"value": reqData.Value,
		},
	})
}

// BatchUpdate 批量更新系统配置
// Route: PUT /settings
// 响应格式: {"message": "Settings updated successfully", "data": {"key1": "val1", ...}}
func (sc *SettingsController) BatchUpdate(ctx *gin.Context) {
	logger.Debug("SettingsController[BatchUpdate] 批量更新系统配置")

	var reqData map[string]string
	if err := ctx.ShouldBindJSON(&reqData); err != nil {
		logger.Warnf("SettingsController[BatchUpdate] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	updated := sc.systemConfigService.BatchUpsert(reqData)

	logger.Infof("SettingsController[BatchUpdate] 批量更新完成: %d 项", len(updated))
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Settings updated successfully",
		"data":    updated,
	})
}

// GetLogSaveDayLimit 获取日志保留天数配置
// Route: GET /logs/config
// NOTE: 逻辑上属于日志模块，但底层仍是系统配置读取，放在此处复用 DAO
func (sc *SettingsController) GetLogSaveDayLimit(ctx *gin.Context) {
	logger.Debug("SettingsController[GetLogSaveDayLimit] 获取日志保留天数")

	key, days, _ := sc.systemConfigService.GetLogSaveDayLimit()

	ctx.JSON(http.StatusOK, gin.H{
		"data": map[string]interface{}{
			"key":   key,
			"value": days,
		},
	})
}

// UpdateLogSaveDayLimit 更新日志保留天数配置
// Route: PUT /logs/config
func (sc *SettingsController) UpdateLogSaveDayLimit(ctx *gin.Context) (int, string) {
	logger.Debug("SettingsController[UpdateLogSaveDayLimit] 更新日志保留天数")

	var configData struct {
		Value int `json:"value" binding:"required,min=1,max=365"`
	}
	if err := ctx.ShouldBindJSON(&configData); err != nil {
		logger.Warnf("SettingsController[UpdateLogSaveDayLimit] 请求体无效: %v", err)
		return http.StatusBadRequest, "Invalid request body, value must be between 1 and 365"
	}

	if err := sc.systemConfigService.UpdateLogSaveDayLimit(configData.Value); err != nil {
		logger.Errorf("SettingsController[UpdateLogSaveDayLimit] 更新失败: %v", err)
		return http.StatusInternalServerError, "Failed to update config"
	}

	logger.Infof("SettingsController[UpdateLogSaveDayLimit] 更新成功: %d 天", configData.Value)
	return http.StatusOK, ""
}
