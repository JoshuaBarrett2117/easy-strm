package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"easy-strm/internal/pkg/logger"
	"easy-strm/internal/service"

	"github.com/gin-gonic/gin"
)

// TestMDCConnection 检测 MDC-NG 地址是否可达，不创建任务也不修改 MDC 配置。
func (sc *SettingsController) TestMDCConnection(ctx *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		ErrorResp(ctx, http.StatusBadRequest, "MDC-NG 地址不能为空")
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		ErrorResp(ctx, http.StatusBadRequest, "MDC-NG 地址格式无效")
		return
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(strings.TrimRight(req.URL, "/"))
	if err != nil {
		ErrorResp(ctx, http.StatusBadGateway, fmt.Sprintf("MDC-NG 连接失败: %v", err))
		return
	}
	defer resp.Body.Close()
	ctx.JSON(http.StatusOK, gin.H{"data": gin.H{"reachable": true, "status": resp.StatusCode, "authenticated": resp.StatusCode != http.StatusTemporaryRedirect && resp.StatusCode != http.StatusFound}})
}

// SettingsController 系统配置控制器
// 负责系统设置的 CRUD 操作，路由路径保持与 auth.go 内联版本完全一致
type SettingsController struct {
	systemConfigService *service.SystemConfigService
	tmdbService         *service.TmdbService
}

// SetTmdbService 注入识别服务，使 MetaTube 配置保存后立即生效。
func (sc *SettingsController) SetTmdbService(svc *service.TmdbService) { sc.tmdbService = svc }

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
	if key == "metatube_enabled" || key == "metatube_url" || key == "metatube_token" {
		adultEnabled := false
		if current, err := sc.systemConfigService.GetByKey("adult_content_enabled"); err == nil && current != nil {
			adultEnabled = current.ConfigVal == "true" || current.ConfigVal == "1"
		}
		if !adultEnabled && ((key == "metatube_enabled" && (reqData.Value == "true" || reqData.Value == "1")) || (key != "metatube_enabled" && strings.TrimSpace(reqData.Value) != "")) {
			ErrorResp(ctx, http.StatusBadRequest, "关闭成人内容识别后不能配置 MetaTube")
			return
		}
	}
	if key == "mdc_enabled" || key == "mdc_url" {
		adultEnabled := false
		if current, err := sc.systemConfigService.GetByKey("adult_content_enabled"); err == nil && current != nil {
			adultEnabled = current.ConfigVal == "true" || current.ConfigVal == "1"
		}
		if !adultEnabled && ((key == "mdc_enabled" && (reqData.Value == "true" || reqData.Value == "1")) || (key == "mdc_url" && strings.TrimSpace(reqData.Value) != "")) {
			ErrorResp(ctx, http.StatusBadRequest, "关闭成人内容识别后不能配置 MDC-NG")
			return
		}
	}

	if err := sc.systemConfigService.Upsert(key, reqData.Value); err != nil {
		logger.Errorf("SettingsController[UpdateByKey] 更新失败 %s: %v", key, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseValue := reqData.Value
	if key == "metatube_token" {
		responseValue = ""
	}
	logger.Infof("SettingsController[UpdateByKey] 更新成功: %s", key)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Setting updated successfully",
		"data": gin.H{
			"key":   key,
			"value": responseValue,
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
	adultEnabled := false
	if current, err := sc.systemConfigService.GetByKey("adult_content_enabled"); err == nil && current != nil {
		adultEnabled = current.ConfigVal == "true" || current.ConfigVal == "1"
	}
	if value, ok := reqData["adult_content_enabled"]; ok {
		adultEnabled = value == "true" || value == "1"
	}
	if !adultEnabled {
		if value := reqData["metatube_enabled"]; value == "true" || value == "1" {
			ErrorResp(ctx, http.StatusBadRequest, "关闭成人内容识别后不能启用 MetaTube")
			return
		}
		if strings.TrimSpace(reqData["metatube_url"]) != "" || strings.TrimSpace(reqData["metatube_token"]) != "" {
			ErrorResp(ctx, http.StatusBadRequest, "关闭成人内容识别后不能配置 MetaTube")
			return
		}
		if value := reqData["mdc_enabled"]; value == "true" || value == "1" || strings.TrimSpace(reqData["mdc_url"]) != "" {
			ErrorResp(ctx, http.StatusBadRequest, "关闭成人内容识别后不能配置 MDC-NG")
			return
		}
		reqData["metatube_enabled"] = "false"
		reqData["mdc_enabled"] = "false"
	}

	updated := sc.systemConfigService.BatchUpsert(reqData)
	if sc.tmdbService != nil {
		sc.tmdbService.SetAdultContentEnabled(adultEnabled)
		currentURL, currentToken := sc.tmdbService.GetMetaTubeConfig()
		if !adultEnabled {
			currentURL, currentToken = "", ""
		}
		if value, ok := updated["metatube_url"]; ok {
			currentURL = value
		}
		if value, ok := reqData["metatube_token"]; ok && value != "" {
			currentToken = value
		}
		sc.tmdbService.SetMetaTubeConfig(currentURL, currentToken)
		if enabled, ok := updated["metatube_enabled"]; ok {
			sc.tmdbService.SetMetaTubeDefaultEnabled(enabled == "true" || enabled == "1")
		}
	}

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
