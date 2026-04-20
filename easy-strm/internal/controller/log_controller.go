package controller

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"easy-strm/internal/dao"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

// LogController 日志管理控制器
// 负责日志文件列表查看、内容读取、配置管理
type LogController struct {
	systemConfigDAO *dao.SystemConfigDAO
	// logDir: 日志文件目录路径，由路由层注入
	logDir string
	// keepDaysUpdater: 更新日志保留天数的回调，由路由层注入
	// NOTE: logger 全局变量属于 main 包，无法在 controller 层直接引用
	keepDaysUpdater func(days int)
	// isAdminChecker: 验证当前用户是否为admin的回调，由路由层注入
	// 回调返回 (userName, isAdmin, error)，如果非admin则由回调内设置响应
	isAdminChecker func(ctx *gin.Context) (string, bool)
}

// NewLogController 创建日志管理控制器实例
func NewLogController(systemConfigDAO *dao.SystemConfigDAO) *LogController {
	return &LogController{
		systemConfigDAO: systemConfigDAO,
		logDir:          "logs", // 默认日志目录
	}
}

// SetLogDir 设置日志目录路径
func (lc *LogController) SetLogDir(logDir string) {
	if logDir != "" {
		lc.logDir = logDir
	}
}

// SetKeepDaysUpdater 设置日志保留天数更新回调
func (lc *LogController) SetKeepDaysUpdater(fn func(days int)) {
	lc.keepDaysUpdater = fn
}

// SetIsAdminChecker 设置 admin 权限校验回调
// 回调签名: 传入 gin.Context, 返回 (用户名, 是否admin)
func (lc *LogController) SetIsAdminChecker(fn func(ctx *gin.Context) (string, bool)) {
	lc.isAdminChecker = fn
}

// GetFileList 获取日志文件列表
// Route: GET /logs
// 仅 admin 用户可访问
func (lc *LogController) GetFileList(ctx *gin.Context) {
	logger.Debug("LogController[GetFileList] 获取日志文件列表")

	// 验证 admin 权限
	userName, isAdmin := lc.checkAdmin(ctx)
	if !isAdmin {
		return
	}

	logDir := lc.logDir

	// 读取日志文件列表（支持 info_*.log 和 debug_*.log 格式）
	infoFiles, err := filepath.Glob(filepath.Join(logDir, "info_*.log"))
	if err != nil {
		logger.Errorf("LogController[GetFileList] glob info日志失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log files"})
		return
	}

	debugFiles, err := filepath.Glob(filepath.Join(logDir, "debug_*.log"))
	if err != nil {
		logger.Errorf("LogController[GetFileList] glob debug日志失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log files"})
		return
	}

	// 合并文件列表
	files := append(infoFiles, debugFiles...)

	// 构建返回数据
	logFiles := make([]map[string]interface{}, 0)
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		fileName := filepath.Base(file)
		logType := "info"
		if strings.HasPrefix(fileName, "debug_") {
			logType = "debug"
		}
		logFiles = append(logFiles, map[string]interface{}{
			"name":     fileName,
			"size":     info.Size(),
			"mod_time": info.ModTime().Format("2006-01-02 15:04:05"),
			"type":     logType,
		})
	}

	// 按文件名降序排列（最新的在前面）
	for i, j := 0, len(logFiles)-1; i < j; i, j = i+1, j-1 {
		logFiles[i], logFiles[j] = logFiles[j], logFiles[i]
	}

	logger.Infof("LogController[GetFileList] admin用户 %s 访问日志文件列表", userName)
	ctx.JSON(http.StatusOK, gin.H{
		"data": logFiles,
	})
}

// GetFileContent 获取日志文件内容
// Route: GET /logs/:filename
// 仅 admin 用户可访问
func (lc *LogController) GetFileContent(ctx *gin.Context) {
	filename := ctx.Param("filename")
	logger.Debugf("LogController[GetFileContent] 获取日志内容, filename: %s", filename)

	// 验证 admin 权限
	userName, isAdmin := lc.checkAdmin(ctx)
	if !isAdmin {
		return
	}

	// 安全检查：确保文件名格式正确（支持 info_*.log 和 debug_*.log）
	if (!strings.HasPrefix(filename, "info_") && !strings.HasPrefix(filename, "debug_")) || !strings.HasSuffix(filename, ".log") {
		logger.Warnf("LogController[GetFileContent] 无效的日志文件名: %s", filename)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log filename"})
		return
	}

	logDir := lc.logDir
	logPath := filepath.Join(logDir, filename)

	// 安全检查：确保路径在日志目录内（防止路径遍历攻击）
	absLogDir, err := filepath.Abs(logDir)
	if err != nil {
		logger.Errorf("LogController[GetFileContent] 获取日志目录绝对路径失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log file"})
		return
	}

	absLogPath, err := filepath.Abs(logPath)
	if err != nil {
		logger.Errorf("LogController[GetFileContent] 获取日志文件绝对路径失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log file"})
		return
	}

	if !strings.HasPrefix(absLogPath, absLogDir) {
		logger.Warnf("LogController[GetFileContent] 路径遍历攻击: %s", filename)
		ctx.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// 获取请求参数
	lines := 500
	if linesStr := ctx.Query("lines"); linesStr != "" {
		if parsedLines, err := strconv.Atoi(linesStr); err == nil && parsedLines > 0 {
			lines = parsedLines
		}
	}

	// 读取日志文件
	content, err := ReadLastNLines(logPath, lines)
	if err != nil {
		if os.IsNotExist(err) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Log file not found"})
			return
		}
		logger.Errorf("LogController[GetFileContent] 读取日志文件失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read log file"})
		return
	}

	logger.Infof("LogController[GetFileContent] admin用户 %s 访问日志: %s", userName, filename)
	ctx.JSON(http.StatusOK, gin.H{
		"data": map[string]interface{}{
			"filename": filename,
			"content":  content,
			"lines":    lines,
		},
	})
}

// GetConfig 获取日志保留天数配置
// Route: GET /logs/config
// 仅 admin 用户可访问
func (lc *LogController) GetConfig(ctx *gin.Context) {
	logger.Debug("LogController[GetConfig] 获取日志配置")

	// 验证 admin 权限
	if _, isAdmin := lc.checkAdmin(ctx); !isAdmin {
		return
	}

	config, err := lc.systemConfigDAO.GetByKey("log_save_day_limit")
	if err != nil || config == nil {
		ctx.JSON(http.StatusOK, gin.H{
			"data": map[string]interface{}{
				"key":   "log_save_day_limit",
				"value": 1,
			},
		})
		return
	}

	var days int
	fmt.Sscanf(config.ConfigVal, "%d", &days)

	ctx.JSON(http.StatusOK, gin.H{
		"data": map[string]interface{}{
			"key":   config.ConfigKey,
			"value": days,
		},
	})
}

// UpdateConfig 更新日志保留天数配置
// Route: PUT /logs/config
// 仅 admin 用户可访问
func (lc *LogController) UpdateConfig(ctx *gin.Context) {
	logger.Debug("LogController[UpdateConfig] 更新日志配置")

	// 验证 admin 权限
	userName, isAdmin := lc.checkAdmin(ctx)
	if !isAdmin {
		return
	}

	var configData struct {
		Value int `json:"value" binding:"required,min=1,max=365"`
	}
	if err := ctx.ShouldBindJSON(&configData); err != nil {
		logger.Warnf("LogController[UpdateConfig] 请求体无效: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body, value must be between 1 and 365"})
		return
	}

	// 更新数据库配置
	if err := lc.systemConfigDAO.Upsert("log_save_day_limit", fmt.Sprintf("%d", configData.Value)); err != nil {
		logger.Errorf("LogController[UpdateConfig] 更新失败: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update config"})
		return
	}

	// 更新日志记录器的保留天数
	if lc.keepDaysUpdater != nil {
		lc.keepDaysUpdater(configData.Value)
	}

	logger.Infof("LogController[UpdateConfig] admin用户 %s 更新日志保留天数为 %d", userName, configData.Value)
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Log config updated successfully",
		"data": map[string]interface{}{
			"key":   "log_save_day_limit",
			"value": configData.Value,
		},
	})
}

// --- 私有辅助方法 ---

// checkAdmin 验证当前用户是否为 admin
// 返回 (用户名, 是否admin)。如果不是admin，自动设置403响应
func (lc *LogController) checkAdmin(ctx *gin.Context) (string, bool) {
	if lc.isAdminChecker != nil {
		userName, isAdmin := lc.isAdminChecker(ctx)
		if !isAdmin {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
			return userName, false
		}
		return userName, true
	}
	// 未注入校验函数时，默认拒绝访问
	logger.Warn("LogController[checkAdmin] 未配置admin校验函数，拒绝访问")
	ctx.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
	return "", false
}

// ReadLastNLines 读取文件的最后N行，返回倒序结果（最新的日志在最上面）
// 导出函数，供其他包复用
func ReadLastNLines(filePath string, n int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	fileSize := stat.Size()

	var lines []string
	var lineBuffer []byte
	var offset int64 = fileSize - 1
	newlineCount := 0

	for offset >= 0 && newlineCount < n {
		b := make([]byte, 1)
		_, err := file.ReadAt(b, offset)
		if err != nil {
			break
		}

		if b[0] == '\n' {
			if len(lineBuffer) > 0 {
				line := reverseBytes(lineBuffer)
				lines = append(lines, string(line))
				lineBuffer = lineBuffer[:0]
				newlineCount++
			}
		} else {
			lineBuffer = append(lineBuffer, b[0])
		}
		offset--
	}

	if len(lineBuffer) > 0 && newlineCount < n {
		line := reverseBytes(lineBuffer)
		lines = append(lines, string(line))
	}

	return strings.Join(lines, "\n"), nil
}

// reverseBytes 反转字节切片
func reverseBytes(b []byte) []byte {
	result := make([]byte, len(b))
	for i := range b {
		result[len(b)-1-i] = b[i]
	}
	return result
}
