package main

import (
	"easy-strm/internal/controller"
	"easy-strm/internal/dao"
	"easy-strm/internal/service"
	"fmt"
	driver "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/gin-gonic/gin"
	"strings"
	"time"
)

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
	playbackService := service.NewPlaybackRecordService(dao.NewPlaybackRecordDAO(redisClient))
	directLinkController.SetRecordPlayback(playbackService.Record)
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
	directLinkController.SetGetFileInfo(func(pickCode string, cloud115ID int, cookie string) (*driver.File, error) {
		return client.GetFileInfo(pickCode, cloud115ID, cookie)
	})
	directLinkController.SetGetDefaultUA(func() string {
		return driver.UA115Disk
	})

	// --- 注册公开路由 ---
	r.POST("/login", authController.Login)
	r.POST("/auth/login", authController.Login)
	r.GET("/direct-link", directLinkController.GetDirectLink)
	// Emby 会通过 HEAD 预解析 STRM 中的远程地址；返回最终 CDN Location 后，
	// Emby 可直接把 115 直链交给播放器，避免客户端访问仅在内网可达的中转地址。
	r.HEAD("/direct-link", directLinkController.GetDirectLink)
}
