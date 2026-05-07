package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"

	"easy-strm/internal/controller"
	"easy-strm/internal/dao"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
)

// JWTClaims 定义JWT声明
type JWTClaims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT token
func GenerateToken(userID int, secret string) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// VerifyToken 验证JWT token
func VerifyToken(tokenString string, secret string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

// SetupAuthRoutes 设置认证相关路由（公开接口，不需要认证）
func SetupAuthRoutes(r *gin.Engine, config *Config, client *Client) {
	// --- 初始化 AuthController ---
	userDAO := dao.NewUserDAO()
	authService := service.NewAuthService(userDAO, config.JWTSecret)
	authController := controller.NewAuthController(authService)

	// 注入 AuthController 回调依赖
	authController.SetGetUserByName(func(name string) (*controller.UserInfo, error) {
		user, err := GetUserByName(name)
		if err != nil {
			return nil, err
		}
		return &controller.UserInfo{
			ID:         user.ID,
			Name:       user.Name,
			Password:   user.Password,
			CreateTime: user.CreateTime,
			UpdateTime: user.UpdateTime,
		}, nil
	})
	authController.SetGetUserByID(func(id int) (*controller.UserInfo, error) {
		user, err := GetUserByID(id)
		if err != nil {
			return nil, err
		}
		return &controller.UserInfo{
			ID:         user.ID,
			Name:       user.Name,
			Password:   user.Password,
			CreateTime: user.CreateTime,
			UpdateTime: user.UpdateTime,
		}, nil
	})
	authController.SetVerifyPassword(func(hashedPassword, password string) error {
		return VerifyPassword(hashedPassword, password)
	})
	authController.SetGenerateToken(func(userID int, secret string) (string, error) {
		return GenerateToken(userID, secret)
	})
	authController.SetSetToken(func(userID int, token string) error {
		return SetToken(userID, token)
	})
	authController.SetGetToken(func(userID int) (string, error) {
		return GetToken(userID)
	})
	authController.SetJWTSecret(config.JWTSecret)
	authController.SetVerifyTokenAndReturnUserID(func(tokenString string, secret string) (int, error) {
		claims, err := VerifyToken(tokenString, secret)
		if err != nil {
			return 0, err
		}
		return claims.UserID, nil
	})

	// --- 初始化 DirectLinkController ---
	directLinkController := controller.NewDirectLinkController()
	directLinkController.SetGetCloud115ByID(func(id int) (*controller.Cloud115AccountBrief, error) {
		cloud115, err := GetCloud115ByID(id)
		if err != nil {
			return nil, err
		}
		return convertCloud115ToBrief(cloud115), nil
	})
	directLinkController.SetGetAllCloud115(func(sortField, sortOrder string) ([]*controller.Cloud115AccountBrief, error) {
		list, err := GetAllCloud115(sortField, sortOrder)
		if err != nil {
			return nil, err
		}
		result := make([]*controller.Cloud115AccountBrief, len(list))
		for i, acc := range list {
			result[i] = convertCloud115ToBrief(acc)
		}
		return result, nil
	})
	directLinkController.SetGetPickCodeByPath(func(path string, cloud115ID int, cookie string) (string, error) {
		return client.GetPickCodeByPath(path, cloud115ID, cookie)
	})
	directLinkController.SetGetFileDirectLink(func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error) {
		directLink, err := client.GetFileDirectLink(uid, pickCode, cloud115ID, cookie, userAgent)
		if err != nil {
			return nil, err
		}
		// 返回 URL 字符串，由 controller 负责重定向
		directLinkURL := directLink.Url.Url
		if directLinkURL == "" || !strings.HasPrefix(directLinkURL, "http") {
			return nil, fmt.Errorf("invalid direct link URL")
		}
		return directLinkURL, nil
	})
	directLinkController.SetRapidTransferByMethod(func(pickCode, decodedPath string, sourceID int, sourceCookie string, targetDirCID string, targetID int, targetCookie string, targetDir string, transferMethod string, alistUrl string, alistToken string) (string, error) {
		return client.RapidTransferByMethod(pickCode, decodedPath, sourceID, sourceCookie, targetDirCID, targetID, targetCookie, targetDir, transferMethod, alistUrl, alistToken)
	})
	directLinkController.SetGetCIDByPath(func(path string, cloud115ID int, cookie string) (string, error) {
		return client.GetCIDByPath(path, cloud115ID, cookie)
	})
	directLinkController.SetRedisGet(func(key string) (string, error) {
		return redisClient.Get(ctx, key).Result()
	})
	directLinkController.SetRedisSet(func(key string, value string, expirationSec int) error {
		return redisClient.Set(ctx, key, value, time.Duration(expirationSec)*time.Second).Err()
	})
	directLinkController.SetGetDefaultUA(func() string {
		return driver.UA115Disk
	})

	// --- 注册公开路由 ---
	r.POST("/login", authController.Login)
	r.POST("/auth/login", authController.Login)
	r.GET("/direct-link", directLinkController.GetDirectLink)
}

// convertCloud115ToBrief 将 main.Cloud115 转换为 controller.Cloud115AccountBrief
// NOTE: 两个结构体字段一致，但属于不同包的类型定义，需要手动转换
func convertCloud115ToBrief(acc *Cloud115) *controller.Cloud115AccountBrief {
	if acc == nil {
		return nil
	}
	return &controller.Cloud115AccountBrief{
		ID:                acc.ID,
		Name:              acc.Name,
		Cookie:            acc.Cookie,
		RefreshToken:      acc.RefreshToken,
		AccessToken:       acc.AccessToken,
		ExpiresIn:         acc.ExpiresIn,
		TransferAccountID: acc.TransferAccountID,
		TransferDirectory: acc.TransferDirectory,
		AccountType:       acc.AccountType,
		Priority:          acc.Priority,
		Status:            acc.Status,
		TransferMethod:    acc.TransferMethod,
		AlistUrl:          acc.AlistUrl,
		AlistToken:        acc.AlistToken,
		CreateTime:        acc.CreateTime,
		UpdateTime:        acc.UpdateTime,
	}
}

// convertStrmConfigToDetail 将 main.StrmConfig 转换为 controller.StrmConfigDetail
func convertStrmConfigToDetail(cfg *StrmConfig) *controller.StrmConfigDetail {
	if cfg == nil {
		return nil
	}
	return &controller.StrmConfigDetail{
		ID:               cfg.ID,
		Cloud115Id:       cfg.Cloud115Id,
		NetDiskPath:      cfg.NetDiskPath,
		LocalPath:        cfg.LocalPath,
		Cron:             cfg.Cron,
		Extension:        cfg.Extension,
		SyncMode:         cfg.SyncMode,
		SourceAccount:    cfg.SourceAccount,
		TargetAccount:    cfg.TargetAccount,
		TargetDirectory:  cfg.TargetDirectory,
		AutoCleanup:      cfg.AutoCleanup,
		CleanupThreshold: cfg.CleanupThreshold,
		CleanupPolicy:    cfg.CleanupPolicy,
		MaxConcurrency:   cfg.MaxConcurrency,
		CreateTime:       cfg.CreateTime,
		UpdateTime:       cfg.UpdateTime,
	}
}

// SetupAuthProtectedRoutes 设置需要认证的路由组
func SetupAuthProtectedRoutes(r *gin.Engine, config *Config, client *Client) {
	// 初始化 DAO（DAO层已在main.go中初始化）

	mediaSourceDAO := dao.NewMediaSourceDAO()
	cloud115DAO := dao.NewCloud115DAO()
	notificationConfigDAO := dao.NewNotificationConfigDAO()
	tmdbCacheDAO := dao.NewTmdbCacheDAO()
	renamePresetDAO := dao.NewRenamePresetDAO()
	mediaCategoryDAO := dao.NewMediaCategoryDAO()
	systemConfigDAO := dao.NewSystemConfigDAO()
	mediaFileCacheDAO := dao.NewMediaFileCacheDAO()
	strmConfigDAO := dao.NewStrmConfigDAO()
	strmFileDAO := dao.NewStrmFileDAO()
	cronTaskDAO := dao.NewCronTaskDAO()
	mediaSyncIndexDAO := dao.NewMediaSyncIndexDAO()
	pendingMediaDAO := dao.NewPendingMediaDAO()
	if err := dao.EnsureMediaLibraryTables(); err != nil {
		Error("Failed to ensure media library tables: %v", err)
	}

	// 初始化 Service
	mediaSourceService := service.NewMediaSourceService(mediaSourceDAO, cloud115DAO)
	cloud115Service := service.NewCloud115Service(cloud115DAO, notificationConfigDAO)
	fileOperationService := service.NewFileOperationService(mediaSourceService, cloud115DAO)
	notificationService := service.NewNotificationService(notificationConfigDAO, NewProxyAwareHTTPClient(15*time.Second))
	tmdbAPIKey := ""
	if apiKeyConfig, err := GetSystemConfigByKey("tmdb_api_key"); err == nil {
		tmdbAPIKey = apiKeyConfig.ConfigVal
	}
	tmdbService := service.NewTmdbService(tmdbAPIKey, tmdbCacheDAO)
	renameService := service.NewRenameService(mediaSourceService, tmdbService, renamePresetDAO, systemConfigDAO)
	scrapeService := service.NewScrapeService(tmdbCacheDAO, mediaFileCacheDAO, mediaSourceDAO, systemConfigDAO, tmdbService)
	organizeService := service.NewOrganizeService(mediaSourceService, tmdbService, renameService, fileOperationService, mediaCategoryDAO, cloud115DAO, systemConfigDAO, scrapeService, client, dao.GetGlobalRedisClient())
	strmService := service.NewStrmService(strmConfigDAO, strmFileDAO, cronTaskDAO)
	cronService := service.NewCronService(cronTaskDAO)
	taskService := service.NewTaskService(dao.NewTaskRedisDAOWithGlobal())
	taskService.SetTaskStepDAO(dao.NewTaskStepDAO())
	dashboardService := service.NewDashboardService(cloud115DAO, mediaSourceDAO, strmFileDAO, dao.NewTaskRedisDAOWithGlobal())
	authService := service.NewAuthService(dao.NewUserDAO(), config.JWTSecret)
	watchService := service.NewWatchService(mediaSourceService, organizeService, cloud115DAO, client, taskService)
	embyService := service.NewEmbyService(systemConfigDAO, NewProxyAwareHTTPClient(15*time.Second))
	cacheAdminService := service.NewCacheAdminService(dao.DB, dao.GetGlobalRedisClient())
	mediaSyncService := service.NewMediaSyncService(mediaSourceDAO, cloud115DAO, mediaSyncIndexDAO, taskService, client)
	mediaSyncService.SetSystemConfigDAO(systemConfigDAO)
	mediaLibraryPipelineService := service.NewMediaLibraryPipelineService(mediaSourceDAO, mediaSyncIndexDAO, pendingMediaDAO, strmConfigDAO, strmFileDAO, systemConfigDAO, tmdbService, taskService, embyService)
	mediaSyncService.SetPipeline(mediaLibraryPipelineService)

	// --- 初始化 Controller ---
	mediaSourceController := controller.NewMediaSourceController(mediaSourceService, cloud115Service, watchService, client)
	fileOperationController := controller.NewFileOperationController(fileOperationService, mediaSourceService)
	organizeController := controller.NewOrganizeController(organizeService)
	organizeController.SetTaskService(taskService)
	tmdbController := controller.NewTmdbController(tmdbService)
	mediaCategoryController := controller.NewMediaCategoryController(mediaCategoryDAO)
	scrapeController := controller.NewScrapeController(scrapeService, organizeService)
	strmController := controller.NewStrmController(strmService)
	cloud115Controller := controller.NewCloud115Controller(cloud115Service, notificationService)
	settingsController := controller.NewSettingsController(systemConfigDAO)
	cronController := controller.NewCronController(cronService, strmService, cloud115Service)
	logController := controller.NewLogController(systemConfigDAO)
	taskController := controller.NewTaskController(taskService)
	networkController := controller.NewNetworkController()
	embyController := controller.NewEmbyController(embyService)
	dashboardController := controller.NewDashboardController(dashboardService)
	cacheAdminController := controller.NewCacheAdminController(cacheAdminService)
	mediaSyncController := controller.NewMediaSyncController(mediaSyncService, mediaLibraryPipelineService)
	pendingMediaController := controller.NewPendingMediaController(pendingMediaDAO, mediaLibraryPipelineService)
	mediaLibraryController := controller.NewMediaLibraryController(mediaSyncIndexDAO, mediaLibraryPipelineService)
	taskController.SetRetryAutoOrganizeTask(func(taskID string) error {
		return watchService.RetryAutoOrganizeTask(taskID)
	})

	// --- 注入 OrganizeController 的 Emby 刷新回调 ---
	organizeController.SetEmbyRefreshCallback(func(sourceID int) *service.EmbyLibraryRefreshResult {
		return embyService.RefreshLibraryBySourceID(mediaSourceDAO, sourceID)
	})

	// --- 注入 StrmController 回调依赖 ---
	strmController.SetGetAllStrmConfig(func(sortField, sortOrder string) ([]*controller.StrmConfigDetail, error) {
		list, err := GetAllStrmConfig(sortField, sortOrder)
		if err != nil {
			return nil, err
		}
		result := make([]*controller.StrmConfigDetail, len(list))
		for i, cfg := range list {
			result[i] = convertStrmConfigToDetail(cfg)
		}
		return result, nil
	})
	strmController.SetGetStrmConfigByID(func(id int) (*controller.StrmConfigDetail, error) {
		cfg, err := GetStrmConfigByID(id)
		if err != nil {
			return nil, err
		}
		return convertStrmConfigToDetail(cfg), nil
	})
	strmController.SetCreateStrmConfig(func(cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*controller.StrmConfigDetail, error) {
		cfg, err := CreateStrmConfig(cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency)
		if err != nil {
			return nil, err
		}
		return convertStrmConfigToDetail(cfg), nil
	})
	strmController.SetUpdateStrmConfig(func(id, cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*controller.StrmConfigDetail, error) {
		cfg, err := UpdateStrmConfig(id, cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency)
		if err != nil {
			return nil, err
		}
		return convertStrmConfigToDetail(cfg), nil
	})
	strmController.SetDeleteStrmConfig(func(id int) error {
		return DeleteStrmConfig(id)
	})
	strmController.SetGetCloud115ByID(func(id int) (*controller.Cloud115AccountBrief, error) {
		cloud115, err := GetCloud115ByID(id)
		if err != nil {
			return nil, err
		}
		return convertCloud115ToBrief(cloud115), nil
	})
	strmController.SetCreateTask(func(taskID, taskType, taskName string) error {
		_, err := CreateTask(taskID, TaskType(taskType), taskName)
		return err
	})
	strmController.SetUpdateTaskStatus(func(taskID, status string) error {
		UpdateTaskStatus(taskID, status)
		return nil
	})
	strmController.SetSetTaskError(func(taskID, errMsg string) error {
		SetTaskError(taskID, errMsg)
		return nil
	})
	strmController.SetRunFullStrmGenerate(func(strmConfig interface{}, cloud115 interface{}, taskID string) (interface{}, error) {
		// 类型断言：controller 类型 -> main 类型
		cfg := strmConfig.(*controller.StrmConfigDetail)
		acc := cloud115.(*controller.Cloud115AccountBrief)

		// 转换为 main 包类型以调用 RunFullStrmGenerate
		mainCfg := &StrmConfig{
			ID:               cfg.ID,
			Cloud115Id:       cfg.Cloud115Id,
			NetDiskPath:      cfg.NetDiskPath,
			LocalPath:        cfg.LocalPath,
			Cron:             cfg.Cron,
			Extension:        cfg.Extension,
			SyncMode:         cfg.SyncMode,
			SourceAccount:    cfg.SourceAccount,
			TargetAccount:    cfg.TargetAccount,
			TargetDirectory:  cfg.TargetDirectory,
			AutoCleanup:      cfg.AutoCleanup,
			CleanupThreshold: cfg.CleanupThreshold,
			CleanupPolicy:    cfg.CleanupPolicy,
			MaxConcurrency:   cfg.MaxConcurrency,
			CreateTime:       cfg.CreateTime,
			UpdateTime:       cfg.UpdateTime,
		}
		mainAcc := &Cloud115{
			ID:                acc.ID,
			Name:              acc.Name,
			Cookie:            acc.Cookie,
			RefreshToken:      acc.RefreshToken,
			AccessToken:       acc.AccessToken,
			ExpiresIn:         acc.ExpiresIn,
			TransferAccountID: acc.TransferAccountID,
			TransferDirectory: acc.TransferDirectory,
			AccountType:       acc.AccountType,
			Priority:          acc.Priority,
			Status:            acc.Status,
			TransferMethod:    acc.TransferMethod,
			AlistUrl:          acc.AlistUrl,
			AlistToken:        acc.AlistToken,
			CreateTime:        acc.CreateTime,
			UpdateTime:        acc.UpdateTime,
		}

		return RunFullStrmGenerate(mainCfg, mainAcc, taskID)
	})
	strmController.SetGetTaskByID(func(taskID string) (interface{}, error) {
		return GetTask(taskID)
	})
	strmController.SetServerURL(config.ServerURL)

	// --- 注入 Cloud115Controller 回调依赖 ---
	cloud115Controller.SetGetAllCloud115(func(sortField, sortOrder string) ([]*controller.Cloud115AccountBrief, error) {
		list, err := GetAllCloud115(sortField, sortOrder)
		if err != nil {
			return nil, err
		}
		result := make([]*controller.Cloud115AccountBrief, len(list))
		for i, acc := range list {
			result[i] = convertCloud115ToBrief(acc)
		}
		return result, nil
	})
	cloud115Controller.SetGetCloud115ByID(func(id int) (*controller.Cloud115AccountBrief, error) {
		cloud115, err := GetCloud115ByID(id)
		if err != nil {
			return nil, err
		}
		return convertCloud115ToBrief(cloud115), nil
	})
	cloud115Controller.SetCreateCloud115(func(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*controller.Cloud115AccountBrief, error) {
		acc, err := CreateCloud115(name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, transferMethod, alistUrl, alistToken)
		if err != nil {
			return nil, err
		}
		return convertCloud115ToBrief(acc), nil
	})
	cloud115Controller.SetUpdateCloud115(func(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*controller.Cloud115AccountBrief, error) {
		acc, err := UpdateCloud115(id, name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, status, transferMethod, alistUrl, alistToken)
		if err != nil {
			return nil, err
		}
		return convertCloud115ToBrief(acc), nil
	})
	cloud115Controller.SetDeleteCloud115(func(id int) error {
		return DeleteCloud115(id)
	})
	cloud115Controller.SetGetQRCode(func() (interface{}, error) {
		qrCodeResp, err := client.GetQRCode()
		if err != nil {
			return nil, err
		}
		// 返回与原 auth.go 内联处理器一致的格式
		return gin.H{
			"state":   true,
			"code":    0,
			"message": "success",
			"data": gin.H{
				"uid":    qrCodeResp.UID,
				"time":   qrCodeResp.Time,
				"qrcode": qrCodeResp.QrcodeContent,
				"sign":   qrCodeResp.Sign,
			},
			"error": "",
			"errno": 0,
		}, nil
	})
	cloud115Controller.SetCheckLoginStatus(func(session interface{}) (interface{}, error) {
		s := session.(map[string]string)
		timeVal := int64(0)
		fmt.Sscanf(s["time"], "%d", &timeVal)
		qrSession := &driver.QRCodeSession{
			UID:           s["uid"],
			Time:          timeVal,
			Sign:          s["sign"],
			QrcodeContent: "",
		}
		statusResp, err := client.CheckLoginStatus(qrSession)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"state":   true,
			"code":    0,
			"message": "success",
			"data": gin.H{
				"status":  statusResp.Status,
				"msg":     statusResp.Msg,
				"version": statusResp.Version,
			},
		}, nil
	})
	cloud115Controller.SetQRCodeLogin(func(session interface{}) (interface{}, error) {
		s := session.(map[string]interface{})
		qrSession := &driver.QRCodeSession{
			UID:           s["uid"].(string),
			Time:          s["time"].(int64),
			Sign:          s["sign"].(string),
			QrcodeContent: "",
		}
		cred, err := client.QRCodeLogin(qrSession)
		if err != nil {
			return nil, err
		}
		cookie := cred.Cookie()
		name := "115账号"
		// 创建新账号
		_, err = CreateCloud115(name, cookie, "", "", 0, 0, "", "resource", 5, "115driver", "", "")
		if err != nil {
			return nil, err
		}
		return gin.H{
			"message": "115 account created successfully",
			"cookie":  cookie,
		}, nil
	})
	cloud115Controller.SetQRCodeLoginWithApp(func(session interface{}, app string) (interface{}, error) {
		s := session.(map[string]interface{})
		qrSession := &driver.QRCodeSession{
			UID:           s["uid"].(string),
			Time:          s["time"].(int64),
			Sign:          s["sign"].(string),
			QrcodeContent: "",
		}
		cred, err := client.QRCodeLoginWithApp(qrSession, driver.LoginApp(app))
		if err != nil {
			return nil, err
		}
		cookie := cred.Cookie()
		name := "115账号"
		_, err = CreateCloud115(name, cookie, "", "", 0, 0, "", "resource", 5, "115driver", "", "")
		if err != nil {
			return nil, err
		}
		return gin.H{
			"message": "115 account created successfully",
			"cookie":  cookie,
		}, nil
	})
	cloud115Controller.SetGetOpenAPIQRCode(func() (interface{}, error) {
		session, err := client.GetOpenAPIQRCode()
		if err != nil {
			return nil, err
		}
		return gin.H{
			"state":   true,
			"code":    0,
			"message": "success",
			"data": gin.H{
				"qrcode_url": session.QRCodeUrl,
				"state":      session.State,
			},
		}, nil
	})
	cloud115Controller.SetCheckOpenAPILoginStatus(func(state string) (interface{}, error) {
		status, err := client.CheckOpenAPILoginStatus(state)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"state":   true,
			"code":    0,
			"message": "success",
			"data":    gin.H{"status": status},
		}, nil
	})
	cloud115Controller.SetConfirmOpenAPILogin(func(state string) (interface{}, error) {
		token, err := client.ConfirmOpenAPILogin(state)
		if err != nil {
			return nil, err
		}
		name := "115账号"
		_, err = CreateCloud115(name, "", token.RefreshToken, token.AccessToken, token.ExpiresIn, 0, "", "resource", 5, "115driver", "", "")
		if err != nil {
			return nil, err
		}
		return gin.H{
			"message":       "115 account created successfully",
			"access_token":  token.AccessToken,
			"refresh_token": token.RefreshToken,
		}, nil
	})
	cloud115Controller.SetGetFileList(func(cid, showDir, offset, limit int, cloud115ID int, cookie string) (interface{}, error) {
		return client.GetFileList(cid, showDir, offset, limit, cloud115ID, cookie)
	})
	cloud115Controller.SetGetFileDirectLink(func(uid int, pickCode string, cloud115ID int, cookie string, userAgent string) (interface{}, error) {
		directLink, err := client.GetFileDirectLink(uid, pickCode, cloud115ID, cookie, userAgent)
		if err != nil {
			return nil, err
		}
		headers := make(map[string]string)
		for key, values := range directLink.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}
		return gin.H{
			"state":   true,
			"code":    0,
			"message": "success",
			"data": gin.H{
				"url":      directLink.Url.Url,
				"size":     directLink.FileSize,
				"name":     directLink.FileName,
				"pickcode": directLink.PickCode,
				"headers":  headers,
			},
		}, nil
	})
	cloud115Controller.SetExportDirectoryTree(func(fileIds, target, cookie string) (interface{}, error) {
		return client.ExportDirectoryTree115(fileIds, target, cookie)
	})
	cloud115Controller.SetGetExportDirectoryTreeStatus(func(exportId, cookie string) (interface{}, error) {
		return client.GetExportDirectoryTreeStatus(exportId, cookie)
	})
	cloud115Controller.SetTestAccountCookie(func(cloud115ID int, cookie string) (interface{}, error) {
		// 获取账号信息
		account, err := cloud115DAO.GetByID(cloud115ID)
		if err != nil {
			return nil, fmt.Errorf("获取账号信息失败: %v", err)
		}
		fileList, err := client.GetFileList(0, 1, 0, 20, cloud115ID, cookie)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"state": true,
			"data": gin.H{
				"id":         cloud115ID,
				"name":       account.Name,
				"file_count": fileList.Count,
			},
			"message": "账号测试成功",
		}, nil
	})

	// --- 注入 TaskController 回调依赖 ---
	taskController.SetGetAllTasks(func() (interface{}, error) {
		return GetAllTasks()
	})

	// --- 注入 NetworkController 回调依赖 ---
	networkController.SetRunNetworkProbe(func(sites interface{}, timeout time.Duration) interface{} {
		probeSites := []NetworkProbeSite{
			{Name: "Telegram API", URL: "https://api.telegram.org"},
			{Name: "Telegram Web", URL: "https://t.me"},
			{Name: "GitHub", URL: "https://github.com"},
			{Name: "GitHub API", URL: "https://api.github.com"},
			{Name: "TMDB API", URL: "https://api.themoviedb.org/3/configuration"},
		}
		if sites != nil {
			raw, err := json.Marshal(sites)
			if err == nil {
				var customSites []NetworkProbeSite
				if unmarshalErr := json.Unmarshal(raw, &customSites); unmarshalErr == nil && len(customSites) > 0 {
					probeSites = customSites
				}
			}
		}
		return RunNetworkProbe(probeSites, timeout)
	})

	// --- 注入 CronController 回调依赖 ---
	if scheduler != nil {
		cronController.SetScheduler(scheduler)
	}
	cronController.SetExecuteCronTaskFn(func(task *domain.CronTask) {
		mainTask := &CronTask{
			ID:             task.ID,
			TaskName:       task.TaskName,
			TaskType:       task.TaskType,
			Cloud115ID:     task.Cloud115ID,
			StrmConfigID:   task.StrmConfigID,
			CronExpr:       task.CronExpr,
			Status:         task.Status,
			LastRunTime:    task.LastRunTime,
			NextRunTime:    task.NextRunTime,
			LastRunStatus:  task.LastRunStatus,
			LastRunMessage: task.LastRunMessage,
			CreateTime:     task.CreateTime,
			UpdateTime:     task.UpdateTime,
		}
		ExecuteCronTask(mainTask)
	})

	// --- 注入 LogController 回调依赖 ---
	if logger != nil {
		logController.SetLogDir(logger.logDir)
		logController.SetKeepDaysUpdater(func(days int) {
			if logger != nil {
				logger.keepDays = days
			}
		})
	}
	logController.SetIsAdminChecker(func(ctx *gin.Context) (string, bool) {
		userID, exists := ctx.Get("userID")
		if !exists {
			return "", false
		}
		user, err := GetUserByID(userID.(int))
		if err != nil {
			return "", false
		}
		return user.Name, user.Name == "admin"
	})

	// --- 初始化 AuthController（用于 JWT 中间件和用户信息）---
	authController := controller.NewAuthController(authService)
	authController.SetJWTSecret(config.JWTSecret)
	authController.SetVerifyTokenAndReturnUserID(func(tokenString string, secret string) (int, error) {
		claims, err := VerifyToken(tokenString, secret)
		if err != nil {
			return 0, err
		}
		return claims.UserID, nil
	})
	authController.SetGetToken(func(userID int) (string, error) {
		return GetToken(userID)
	})
	authController.SetGetUserByID(func(id int) (*controller.UserInfo, error) {
		user, err := GetUserByID(id)
		if err != nil {
			return nil, err
		}
		return &controller.UserInfo{
			ID:         user.ID,
			Name:       user.Name,
			Password:   user.Password,
			CreateTime: user.CreateTime,
			UpdateTime: user.UpdateTime,
		}, nil
	})

	// 需要验证token的路由组
	auth := r.Group("/")
	auth.Use(authController.JWTMiddleware())
	{
		// ========== Dashboard 数据概览 ==========
		auth.GET("/dashboard/stats", dashboardController.GetStats)
		auth.GET("/dashboard/overview", dashboardController.GetOverview)
		auth.GET("/dashboard/resource-monitor", dashboardController.GetResourceMonitor)
		auth.GET("/dashboard/trends/:kind", dashboardController.GetTrend)
		auth.GET("/cache/overview", cacheAdminController.GetOverview)
		auth.POST("/cache/clear", cacheAdminController.Clear)

		// ========== 用户信息 ==========
		auth.GET("/user/info", authController.GetUserInfo)

		// ========== 媒体分类管理 ==========
		auth.GET("/media/categories", mediaCategoryController.GetAll)
		auth.POST("/media/categories", mediaCategoryController.Create)
		auth.PUT("/media/categories/:id", mediaCategoryController.Update)
		auth.DELETE("/media/categories/:id", mediaCategoryController.Delete)

		// ========== 115 扫码登录 ==========
		auth.GET("/115/login/channels", cloud115Controller.GetLoginChannels)
		auth.GET("/115/qrcode", cloud115Controller.GetQRCode)
		auth.GET("/115/login/status", cloud115Controller.CheckLoginStatus)
		auth.POST("/115/login/confirm", cloud115Controller.ConfirmLogin)

		// ========== 115 Open API 扫码登录 ==========
		auth.GET("/115/open/qrcode", cloud115Controller.GetOpenAPIQRCode)
		auth.GET("/115/open/login/status", cloud115Controller.CheckOpenAPILoginStatus)
		auth.POST("/115/open/login/confirm", cloud115Controller.ConfirmOpenAPILogin)

		// ========== 115 云账号管理 ==========
		auth.GET("/cloud115", cloud115Controller.GetList)
		auth.GET("/cloud115/:id", cloud115Controller.GetByID)
		auth.POST("/cloud115", cloud115Controller.Create)
		auth.PUT("/cloud115/:id", cloud115Controller.Update)
		auth.DELETE("/cloud115/:id", cloud115Controller.Delete)
		auth.GET("/auth/cloud115/:id", cloud115Controller.TestAccountCookie)

		// ========== 115 文件操作 ==========
		auth.GET("/115/files", cloud115Controller.GetFileList)
		auth.GET("/115/direct-link", cloud115Controller.GetDirectLink)
		auth.POST("/115/export-dir", cloud115Controller.ExportDirectoryTree)
		auth.GET("/115/export-dir/status", cloud115Controller.GetExportDirectoryTreeStatus)

		// ========== 通知配置 ==========
		auth.GET("/notify/config", cloud115Controller.GetNotificationConfig)
		auth.PUT("/notify/config", cloud115Controller.UpdateNotificationConfig)
		auth.DELETE("/notify/config/:channel", cloud115Controller.DeleteNotificationConfig)
		auth.POST("/notify/test", cloud115Controller.TestNotification)

		// ========== 系统配置 ==========
		auth.GET("/settings", settingsController.GetAll)
		auth.GET("/settings/:key", settingsController.GetByKey)
		auth.PUT("/settings/:key", settingsController.UpdateByKey)
		auth.PUT("/settings", settingsController.BatchUpdate)

		// ========== 网络测试 ==========
		auth.GET("/network/test", networkController.NetworkTest)

		// ========== STRM 配置管理 ==========
		auth.GET("/strm/config", strmController.GetConfigList)
		auth.GET("/strm/config/:id", strmController.GetConfigByID)
		auth.POST("/strm/config", strmController.CreateConfig)
		auth.PUT("/strm/config/:id", strmController.UpdateConfig)
		auth.DELETE("/strm/config/:id", strmController.DeleteConfig)
		auth.POST("/strm/config/:id/generate/full", strmController.GenerateFull)
		auth.POST("/strm/config/:id/generate/incremental", strmController.GenerateIncremental)
		auth.GET("/strm/task/:task_id", strmController.GetTaskStatus)

		// ========== 任务管理 ==========
		auth.GET("/tasks", taskController.GetAll)
		auth.GET("/tasks/:task_id", taskController.Get)
		auth.GET("/tasks/unified", taskController.GetUnified)
		auth.POST("/tasks/:task_id/cancel", taskController.Cancel)
		auth.POST("/tasks/:task_id/resume", taskController.Resume)
		auth.GET("/scheduled-tasks", cronController.GetScheduledTasks)

		// ========== Cron 任务管理 ==========
		auth.GET("/cron/tasks", cronController.GetAll)
		auth.POST("/cron/task", cronController.Create)
		auth.PUT("/cron/task/:id", cronController.Update)
		auth.DELETE("/cron/task/:id", cronController.Delete)
		auth.POST("/cron/task/:id/run", cronController.RunImmediately)
		auth.GET("/cron/task/:id/status", cronController.GetStatus)

		// ========== 媒体源管理 ==========
		auth.GET("/media/sources", mediaSourceController.GetList)
		auth.GET("/media/sources/:id", mediaSourceController.GetByID)
		auth.POST("/media/sources", mediaSourceController.Create)
		auth.PUT("/media/sources/:id", mediaSourceController.Update)
		auth.DELETE("/media/sources/:id", mediaSourceController.Delete)
		auth.POST("/media/sources/:id/sync/full", mediaSyncController.RunFullSync)
		auth.POST("/media/sources/:id/sync/incremental", mediaSyncController.RunIncrementalSync)
		auth.GET("/media/sources/:id/sync/index", mediaSyncController.GetIndex)
		auth.POST("/media/sources/:id/pipeline", mediaSyncController.RunPipeline)
		auth.GET("/media/files", mediaSourceController.GetFiles)
		auth.GET("/media/files/search", mediaSourceController.SearchFiles)

		// ========== 媒体库与待处理 ==========
		auth.GET("/media/library/items", mediaLibraryController.ListItems)
		auth.GET("/media/library/items/:id", mediaLibraryController.GetItem)
		auth.POST("/media/library/items/:id/pipeline", mediaLibraryController.RunPipeline)
		auth.POST("/media/library/items/:id/strm", mediaLibraryController.GenerateStrm)
		auth.POST("/media/library/items/:id/refresh-server", mediaLibraryController.RefreshServer)
		auth.GET("/media/pending", pendingMediaController.List)
		auth.POST("/media/pending", pendingMediaController.Create)
		auth.POST("/media/pending/:id/identify", pendingMediaController.Identify)
		auth.POST("/media/pending/:id/run", pendingMediaController.Run)
		auth.POST("/media/pending/:id/ignore", pendingMediaController.Ignore)

		// ========== 文件操作 ==========
		auth.POST("/media/files/move", fileOperationController.MoveFile)
		auth.POST("/media/files/copy", fileOperationController.CopyFile)
		auth.POST("/media/files/delete", fileOperationController.DeleteFile)
		auth.POST("/media/files/rename", fileOperationController.RenameFile)
		auth.POST("/media/files/batch", fileOperationController.BatchOperation)
		auth.GET("/media/files/preview", fileOperationController.GetFilePreview)

		// ========== 自动整理 ==========
		auth.POST("/media/organize/candidates", organizeController.ListOrganizeCandidates)
		auth.POST("/media/organize/candidates/async", organizeController.StartCandidateTaskAsync)
		auth.GET("/media/organize/candidates/status", organizeController.GetCandidateTaskStatus)
		auth.POST("/media/organize/preview", organizeController.PreviewOrganize)
		auth.POST("/media/organize/preview/async", organizeController.StartPreviewTaskAsync)
		auth.GET("/media/organize/preview/status", organizeController.GetPreviewTaskStatus)
		auth.POST("/media/organize/preview/check", organizeController.CheckRestorableTask)
		auth.POST("/media/organize/execute", organizeController.ExecuteOrganize)
		auth.POST("/media/organize/execute/async", organizeController.ExecuteOrganizeAsync)
		auth.POST("/media/organize/batch-identify", organizeController.BatchIdentify)
		auth.POST("/media/organize/batch-identify-directory", organizeController.BatchIdentifyDirectory)
		auth.POST("/media/organize/batch-rename-preview", organizeController.BatchRenamePreview)
		auth.POST("/media/organize/batch-rename-execute", organizeController.BatchRenameExecute)
		auth.POST("/media/organize/identify", organizeController.IdentifyFile)
		auth.POST("/media/organize/rename-preview", organizeController.PreviewRename)
		auth.POST("/media/organize/rename-execute", organizeController.ExecuteRename)
		auth.GET("/media/organize/presets", organizeController.GetRenamePresets)

		// ========== STRM 联动整理 ==========
		auth.POST("/strm/config/generate/from-organize", func(c *gin.Context) {
			Debug("Generate STRM from organize API called from %s", c.ClientIP())

			var req struct {
				SourceID     int      `json:"source_id" binding:"required"`
				TargetPaths  []string `json:"target_paths" binding:"required"`
				StrmConfigID int      `json:"strm_config_id"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				Warn("Invalid generate STRM from organize request body from %s: %v", c.ClientIP(), err)
				JSON(c, http.StatusBadRequest, gin.H{"error": "无效的请求体"})
				return
			}

			if len(req.TargetPaths) == 0 {
				JSON(c, http.StatusBadRequest, gin.H{"error": "目标路径列表不能为空"})
				return
			}

			source, err := mediaSourceService.GetByID(req.SourceID)
			if err != nil {
				Error("Failed to get media source by ID %d: %v", req.SourceID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "媒体源不存在"})
				return
			}

			if source.SourceType != "cloud115" {
				JSON(c, http.StatusBadRequest, gin.H{"error": "仅115云盘类型的媒体源支持STRM生成"})
				return
			}

			if source.Cloud115ID == nil {
				JSON(c, http.StatusBadRequest, gin.H{"error": "媒体源未关联115账号"})
				return
			}

			cloud115ID := *source.Cloud115ID
			configID := req.StrmConfigID
			if configID <= 0 {
				matchPath := req.TargetPaths[0]
				matchedID, err := strmController.FindMatchingConfig(cloud115ID, matchPath)
				if err != nil {
					Error("Failed to find matching STRM config: %v", err)
					JSON(c, http.StatusInternalServerError, gin.H{"error": "查找匹配STRM配置失败"})
					return
				}
				if matchedID <= 0 {
					JSON(c, http.StatusNotFound, gin.H{"error": "未找到匹配的STRM配置，请先创建STRM配置"})
					return
				}
				configID = matchedID
				Info("Auto-matched STRM config ID %d for source %d", configID, req.SourceID)
			}

			strmConfig, err := GetStrmConfigByID(configID)
			if err != nil || strmConfig == nil {
				Error("STRM config not found: ID %d, err: %v", configID, err)
				JSON(c, http.StatusNotFound, gin.H{"error": "STRM配置不存在"})
				return
			}

			organizeTargetPath := source.OrganizeTargetPath
			if organizeTargetPath != "" {
				if err := strmController.UpdateNetDiskPath(configID, organizeTargetPath); err != nil {
					Warn("Failed to update STRM config net_disk_path: %v", err)
				}
			}

			cloud115, err := GetCloud115ByID(strmConfig.Cloud115Id)
			if err != nil || cloud115 == nil {
				Error("Cloud115 account not found: ID %d", strmConfig.Cloud115Id)
				JSON(c, http.StatusNotFound, gin.H{"error": "115账号不存在"})
				return
			}

			taskID := uuid.New().String()
			taskName := fmt.Sprintf("STRM生成(整理后) - %s", strmConfig.NetDiskPath)
			_, err = CreateTask(taskID, TaskTypeStrmGenerate, taskName)
			if err != nil {
				Error("Failed to create STRM generation task: %v", err)
				JSON(c, http.StatusInternalServerError, gin.H{"error": "创建任务失败"})
				return
			}

			capturedConfigID := configID
			capturedTaskID := taskID

			go func() {
				defer func() {
					if r := recover(); r != nil {
						Error("Panic in STRM generation from organize task: %v", r)
						SetTaskError(capturedTaskID, fmt.Sprintf("Internal error: %v", r))
					}
				}()

				Info("Starting STRM generation from organize task %s for config ID %d", capturedTaskID, capturedConfigID)
				UpdateTaskStatus(capturedTaskID, TaskStatusRunning)

				result, err := RunFullStrmGenerate(strmConfig, cloud115, capturedTaskID)
				if err != nil {
					Error("STRM generation from organize failed: %v", err)
					SetTaskError(capturedTaskID, err.Error())
					return
				}

				UpdateTaskStatus(capturedTaskID, TaskStatusCompleted)
				Info("STRM generation from organize completed: task %s, %d files", capturedTaskID, result.Total)
			}()

			JSON(c, 200, gin.H{
				"message":        "STRM生成任务已创建",
				"task_id":        taskID,
				"strm_config_id": configID,
				"target_paths":   req.TargetPaths,
			})
		})

		// ========== TMDB API ==========
		auth.GET("/media/tmdb/search", tmdbController.Search)
		auth.POST("/media/tmdb/identify", tmdbController.Identify)
		auth.POST("/media/tmdb/auto-identify", tmdbController.AutoIdentify)
		auth.POST("/media/tmdb/batch-identify", tmdbController.BatchIdentify)
		auth.GET("/media/tmdb/movie/:id", tmdbController.GetMovieDetail)
		auth.GET("/media/tmdb/tv/:id", tmdbController.GetTVDetail)
		auth.GET("/media/tmdb/config", tmdbController.GetConfig)
		auth.POST("/media/tmdb/config", func(c *gin.Context) {
			var req struct {
				APIKey   string `json:"api_key"`
				Language string `json:"language"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				JSON(c, 400, gin.H{"error": "无效的请求体"})
				return
			}

			req.APIKey = strings.TrimSpace(req.APIKey)
			if strings.Contains(req.APIKey, "****") {
				JSON(c, 400, gin.H{"error": "请填写真实的 TMDB API Key，不能提交脱敏后的显示值"})
				return
			}

			if req.APIKey != "" {
				_, err := UpsertSystemConfig("tmdb_api_key", req.APIKey)
				if err != nil {
					Error("Failed to save tmdb_api_key to database: %v", err)
					JSON(c, 500, gin.H{"error": "保存配置失败"})
					return
				}

				tmdbService.SetAPIKey(req.APIKey)
			} else if !tmdbService.HasUsableAPIKey() {
				JSON(c, 400, gin.H{"error": "当前未配置可用的 TMDB API Key，请输入真实的 API Key"})
				return
			}
			if req.Language != "" {
				tmdbService.SetLanguage(req.Language)
			}

			Info("TMDB API Key updated successfully")
			JSON(c, 200, gin.H{"message": "API Key 更新成功"})
		})

		// ========== NFO 刮削 ==========
		auth.POST("/media/scrape/file", scrapeController.ScrapeFile)
		auth.POST("/media/scrape/files", scrapeController.ScrapeFiles)
		auth.POST("/media/scrape/directory", scrapeController.ScrapeDirectory)

		// ========== Emby 集成 ==========
		auth.GET("/emby/status", embyController.GetStatus)
		auth.GET("/emby/libraries", embyController.GetLibraries)
		auth.POST("/emby/refresh", embyController.Refresh)

		// ========== 日志查看 ==========
		auth.GET("/logs", logController.GetFileList)
		auth.GET("/logs/:filename", logController.GetFileContent)
		auth.GET("/logs/config", logController.GetConfig)
		auth.PUT("/logs/config", logController.UpdateConfig)
	}

	// 启动文件监控服务（异步，不阻塞主流程）
	go func() {
		Info("Starting WatchService...")
		watchService.StartAll()
	}()

	// 启动识别缓存清理任务（每天凌晨3点执行）
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if nextRun.Before(now) {
				nextRun = nextRun.Add(24 * time.Hour)
			}
			select {
			case <-ticker.C:
				// 执行清理
				deleted, err := dao.NewIdentifyCacheDAO().DeleteOlderThan(30)
				if err != nil {
					Error("IdentifyCache cleanup failed: %v", err)
				} else {
					Info("IdentifyCache cleanup: deleted %d old records", deleted)
				}
			}
		}
	}()
}

// buildDirTreeFromEntries 从目录树文件条目构建目录树结构
func buildDirTreeFromEntries(entries []DirTreeEntry, rootName string) *DirectoryNode {
	Debug("Building directory tree from %d entries, root name: %s", len(entries), rootName)

	root := &DirectoryNode{
		CID:      "0",
		Name:     rootName,
		Type:     "dir",
		Files:    []driver.FileInfo{},
		Children: []*DirectoryNode{},
	}

	pathToNode := make(map[string]*DirectoryNode)
	pathToNode[""] = root

	for _, entry := range entries {
		if entry.IsDir {
			node := &DirectoryNode{
				CID:      "0",
				Name:     entry.Name,
				Type:     "dir",
				Files:    []driver.FileInfo{},
				Children: []*DirectoryNode{},
			}
			fullPath := entry.Path
			if fullPath != "" {
				fullPath = filepath.Join(fullPath, entry.Name)
			} else {
				fullPath = entry.Name
			}
			pathToNode[fullPath] = node
			Debug("Added directory node: %s at path: %s", entry.Name, fullPath)
		}
	}

	for _, entry := range entries {
		if !entry.IsDir {
			fileInfo := driver.FileInfo{
				Name:     entry.Name,
				Size:     driver.StringInt64(entry.Size),
				PickCode: entry.Pc,
				FileID:   entry.Fid,
				Sha1:     entry.Sha1,
			}
			parentPath := entry.Path
			parentNode, exists := pathToNode[parentPath]
			if !exists {
				parentNode = root
			}
			parentNode.Files = append(parentNode.Files, fileInfo)
			Debug("Added file: %s to directory at path: %s", entry.Name, parentPath)
		}
	}

	for fullPath, node := range pathToNode {
		if fullPath == "" {
			continue
		}
		parentPath := filepath.Dir(fullPath)
		if parentPath == "." {
			parentPath = ""
		}
		parentNode, exists := pathToNode[parentPath]
		if exists && parentNode != node {
			parentNode.Children = append(parentNode.Children, node)
			Debug("Added child: %s to parent at path: %s", node.Name, parentPath)
		}
	}

	return root
}

// extractVideoFiles 从目录树中递归提取视频文件
func extractVideoFiles(node *DirectoryNode, currentPath string, netDiskPath string, cid string, targetExts []string, collection *VideoCollection, cloud115Id int, isRoot bool) {
	Debug("Extracting video files from directory: %s, currentPath: %s, isRoot: %v", node.Name, currentPath, isRoot)

	for _, file := range node.Files {
		fileExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.Name), "."))

		matched := false
		for _, targetExt := range targetExts {
			if fileExt == targetExt {
				matched = true
				break
			}
		}

		if matched {
			netDiskFullPath := filepath.Join(netDiskPath, currentPath, file.Name)
			filePickCode := file.PickCode
			if filePickCode == "" {
				filePickCode = file.FileID
			}

			relativePath := currentPath
			if relativePath == "" {
				relativePath = file.Name
			} else {
				relativePath = filepath.Join(currentPath, file.Name)
			}

			videoFile := VideoFile{
				Path:         currentPath,
				Filename:     file.Name,
				CID:          cid,
				FID:          filePickCode,
				Size:         int(file.Size),
				Extension:    filepath.Ext(file.Name),
				Sha1:         netDiskFullPath,
				Cloud115ID:   cloud115Id,
				RelativePath: relativePath,
				PickCode:     filePickCode,
				Name:         file.Name,
			}

			collection.Videos = append(collection.Videos, videoFile)
			Debug("Added video file: %s (PickCode: %s, Size: %d, cloud115_id: %d)", netDiskFullPath, filePickCode, file.Size, cloud115Id)
		}
	}

	for _, child := range node.Children {
		childPath := child.Name
		if !isRoot && currentPath != "" {
			childPath = filepath.Join(currentPath, child.Name)
		}
		extractVideoFiles(child, childPath, netDiskPath, cid, targetExts, collection, cloud115Id, false)
	}
}

// readLastNLines 读取文件的最后N行，返回倒序结果
func readLastNLines(filePath string, n int) (string, error) {
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

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
